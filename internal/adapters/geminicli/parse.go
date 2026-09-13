package geminicli

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/pastlog/pastlog/internal/agentlog"
)

// metadata is the parsed JSONL metadata record (line 1); see SCHEMA.md.
type metadata struct {
	sessionID string
	project   string
	title     string
	startedAt time.Time
	endedAt   time.Time
}

// metaRaw mirrors the on-disk metadata record fields.
type metaRaw struct {
	SessionID   string   `json:"sessionId"`
	ProjectHash string   `json:"projectHash"`
	StartTime   string   `json:"startTime"`
	LastUpdated string   `json:"lastUpdated"`
	Kind        string   `json:"kind"` // main | subagent — not needed by the v0.1 model
	Directories []string `json:"directories"`
	Summary     string   `json:"summary"`
}

// parseMeta reports whether a line is a session metadata record: a JSON
// object without a message `type` that carries at least one metadata field.
func parseMeta(line []byte) (metadata, bool) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return metadata{}, false
	}
	var probe struct {
		Type        json.RawMessage `json:"type"` // message records always carry a type
		SessionID   json.RawMessage `json:"sessionId"`
		StartTime   json.RawMessage `json:"startTime"`
		LastUpdated json.RawMessage `json:"lastUpdated"`
	}
	if err := json.Unmarshal(trimmed, &probe); err != nil {
		return metadata{}, false
	}
	if len(bytes.TrimSpace(probe.Type)) > 0 {
		return metadata{}, false // a message record, never metadata
	}
	if len(bytes.TrimSpace(probe.SessionID)) == 0 &&
		len(bytes.TrimSpace(probe.StartTime)) == 0 &&
		len(bytes.TrimSpace(probe.LastUpdated)) == 0 {
		return metadata{}, false // no metadata field at all
	}
	var raw metaRaw
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return metadata{}, false
	}
	return metadata{
		sessionID: raw.SessionID,
		project:   firstString(raw.Directories),
		title:     raw.Summary,
		startedAt: parseTS(raw.StartTime),
		endedAt:   parseTS(raw.LastUpdated),
	}, true
}

// recordRaw mirrors one on-disk record; only fields pastlog consumes are
// declared. MessageRecord and the compaction checkpoint share this shape.
type recordRaw struct {
	Type      string          `json:"type"` // user | info | error | warning | gemini
	Timestamp string          `json:"timestamp"`
	Content   json.RawMessage `json:"content"` // PartListUnion: string or []part
	ToolCalls json.RawMessage `json:"toolCalls"`
	Thoughts  json.RawMessage `json:"thoughts"`
	Set       *setRaw         `json:"$set"` // compaction checkpoint
}

// setRaw is the checkpoint payload: a snapshot of the conversation so far.
type setRaw struct {
	Messages []json.RawMessage `json:"messages"`
}

// toolCallRaw mirrors one gemini record's toolCalls entry.
type toolCallRaw struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Args      json.RawMessage `json:"args"`
	Result    json.RawMessage `json:"result"`
	Status    string          `json:"status"`
	Timestamp string          `json:"timestamp"`
}

// thoughtRaw mirrors one gemini record's thoughts entry.
type thoughtRaw struct {
	Subject     string `json:"subject"`
	Description string `json:"description"`
	Timestamp   string `json:"timestamp"`
}

// roleFor maps a record type to a best-effort role (SCHEMA.md): user stays
// user, the machine-generated info/error/warning records become system, the
// model's records become assistant. Unknown types have no mapping.
func roleFor(recordType string) (string, bool) {
	switch recordType {
	case "user":
		return "user", true
	case "info", "error", "warning":
		return "system", true
	case "gemini":
		return "assistant", true
	default:
		return "", false
	}
}

// processRecord classifies one non-empty record into entries. ok=false means
// the record was unreadable or an unknown shape and must be counted as
// skipped by the caller. Checkpoint records ({$set:…}, recognized protocol
// shapes) are applied by mapping their message array and never count
// themselves; message elements inside them that fail to parse are genuinely
// unknown shapes and are counted via skipped. Within one record the entry
// order is: content texts, content part entries, tool calls/results,
// thoughts.
func processRecord(line []byte, skipped *int) (entries []agentlog.Entry, ok bool) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, false
	}
	var rec recordRaw
	if err := json.Unmarshal(trimmed, &rec); err != nil {
		return nil, false
	}
	if rec.Set != nil {
		// compaction checkpoint: apply the snapshot's messages in place
		for _, raw := range rec.Set.Messages {
			if sub, ok := processRecord(raw, skipped); ok {
				entries = append(entries, sub...)
			} else {
				*skipped++ // unknown shape inside a recognized checkpoint
			}
		}
		return entries, true
	}
	role, known := roleFor(rec.Type)
	if !known {
		return nil, false // unknown record shape (deletion records, …)
	}
	ts := parseTS(rec.Timestamp)
	texts, extras, ok := contentEntries(rec.Content, ts)
	if !ok {
		return nil, false // content present but unusable
	}
	for _, text := range texts {
		entries = append(entries, agentlog.Entry{Kind: agentlog.Message, Role: role, Text: text, Timestamp: ts})
	}
	entries = append(entries, extras...)
	calls, ok := toolCallEntries(rec.ToolCalls, ts)
	if !ok {
		return nil, false // toolCalls present but unusable
	}
	entries = append(entries, calls...)
	thoughts, ok := thoughtEntries(rec.Thoughts, ts)
	if !ok {
		return nil, false // thoughts present but unusable
	}
	return append(entries, thoughts...), true
}

// contentEntries flattens a PartListUnion content (string or part array).
// Text parts become message texts; functionCall/functionResponse parts become
// extra entries. ok=false marks the whole record unusable (content of the
// wrong shape). The record's role is applied by the caller to the texts.
func contentEntries(raw json.RawMessage, ts time.Time) (texts []string, extras []agentlog.Entry, ok bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil, true // readable record, nothing to record
	}
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, nil, false
		}
		if s != "" {
			texts = append(texts, s)
		}
		return texts, extras, true
	case '[':
		var parts []json.RawMessage
		if err := json.Unmarshal(trimmed, &parts); err != nil {
			return nil, nil, false
		}
		for _, part := range parts {
			// unknown part shapes are skipped; the rest keeps parsing
			if text, entry, isPart := partEntries(bytes.TrimSpace(part), ts); isPart {
				if text != "" {
					texts = append(texts, text)
				}
				if entry != nil {
					extras = append(extras, *entry)
				}
			}
		}
		return texts, extras, true
	default:
		return nil, nil, false // neither string nor array
	}
}

// partEntries maps one content part: {text:...} → text; {functionCall:...} →
// tool call entry; {functionResponse:...} → tool result entry; anything else
// → isPart=false (skip that part, keep parsing the rest).
func partEntries(part []byte, ts time.Time) (text string, entry *agentlog.Entry, isPart bool) {
	if len(part) == 0 || part[0] != '{' {
		return "", nil, false
	}
	var probe struct {
		Text             json.RawMessage `json:"text"`
		FunctionCall     json.RawMessage `json:"functionCall"`
		FunctionResponse json.RawMessage `json:"functionResponse"`
	}
	if err := json.Unmarshal(part, &probe); err != nil {
		return "", nil, false
	}
	switch {
	case len(bytes.TrimSpace(probe.FunctionCall)) > 0:
		if e, ok := functionCallEntry(part, ts); ok {
			return "", &e, true
		}
		return "", nil, false
	case len(bytes.TrimSpace(probe.FunctionResponse)) > 0:
		if e, ok := functionResponseEntry(part, ts); ok {
			return "", &e, true
		}
		return "", nil, false
	case len(bytes.TrimSpace(probe.Text)) > 0:
		var s string
		if err := json.Unmarshal(probe.Text, &s); err != nil {
			return "", nil, false
		}
		return s, nil, true
	default:
		return "", nil, false
	}
}

// functionCallEntry maps a {functionCall:{name,args}} part to a ToolCall
// entry with the compact JSON of args as its text (falls back to the call
// name when args are missing, mirroring the codex adapter).
func functionCallEntry(part []byte, ts time.Time) (agentlog.Entry, bool) {
	var fc struct {
		FunctionCall struct {
			Name string          `json:"name"`
			Args json.RawMessage `json:"args"`
		} `json:"functionCall"`
	}
	if err := json.Unmarshal(part, &fc); err != nil {
		return agentlog.Entry{}, false
	}
	text := agentlog.CompactJSON(fc.FunctionCall.Args)
	if text == "" {
		text = fc.FunctionCall.Name
	}
	if text == "" {
		return agentlog.Entry{}, false
	}
	return agentlog.Entry{Kind: agentlog.ToolCall, Role: "tool", Text: text, Timestamp: ts}, true
}

// functionResponseEntry maps a {functionResponse:{name,response}} part to a
// ToolResult entry with the flattened response parts as its text.
func functionResponseEntry(part []byte, ts time.Time) (agentlog.Entry, bool) {
	var fr struct {
		FunctionResponse struct {
			Name     string          `json:"name"`
			Response json.RawMessage `json:"response"`
		} `json:"functionResponse"`
	}
	if err := json.Unmarshal(part, &fr); err != nil {
		return agentlog.Entry{}, false
	}
	text := partListText(fr.FunctionResponse.Response)
	if text == "" {
		return agentlog.Entry{}, false
	}
	return agentlog.Entry{Kind: agentlog.ToolResult, Role: "tool", Text: text, Timestamp: ts}, true
}

// toolCallEntries maps one gemini record's toolCalls array to ToolCall plus
// (when a result is present) ToolResult entries, in array order.
func toolCallEntries(raw json.RawMessage, ts time.Time) ([]agentlog.Entry, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, true
	}
	if trimmed[0] != '[' {
		return nil, false // wrong shape
	}
	var calls []toolCallRaw
	if err := json.Unmarshal(trimmed, &calls); err != nil {
		return nil, false
	}
	var entries []agentlog.Entry
	for _, tc := range calls {
		text := agentlog.CompactJSON(tc.Args)
		if text == "" {
			text = tc.Name
		}
		callTS := ts
		if t := parseTS(tc.Timestamp); !t.IsZero() {
			callTS = t
		}
		if text != "" {
			entries = append(entries, agentlog.Entry{Kind: agentlog.ToolCall, Role: "tool", Text: text, Timestamp: callTS})
		}
		if text := partListText(tc.Result); text != "" {
			entries = append(entries, agentlog.Entry{Kind: agentlog.ToolResult, Role: "tool", Text: text, Timestamp: callTS})
		}
	}
	return entries, true
}

// thoughtEntries maps one gemini record's thoughts array to Summary entries.
func thoughtEntries(raw json.RawMessage, ts time.Time) ([]agentlog.Entry, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, true
	}
	if trimmed[0] != '[' {
		return nil, false // wrong shape
	}
	var thoughts []thoughtRaw
	if err := json.Unmarshal(trimmed, &thoughts); err != nil {
		return nil, false
	}
	var entries []agentlog.Entry
	for _, th := range thoughts {
		text := thoughtText(th)
		if text == "" {
			continue
		}
		entryTS := ts
		if t := parseTS(th.Timestamp); !t.IsZero() {
			entryTS = t
		}
		entries = append(entries, agentlog.Entry{Kind: agentlog.Summary, Role: "assistant", Text: text, Timestamp: entryTS})
	}
	return entries, true
}

// thoughtText joins a thought's subject and description ("subject:
// description"), keeping whichever is present.
func thoughtText(th thoughtRaw) string {
	subject, description := strings.TrimSpace(th.Subject), strings.TrimSpace(th.Description)
	switch {
	case subject != "" && description != "":
		return subject + ": " + description
	case subject != "":
		return subject
	default:
		return description
	}
}

// partListText flattens a PartListUnion (string or part array) to text: text
// parts are joined with newlines; other part shapes are skipped.
func partListText(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			return s
		}
	case '[':
		var parts []json.RawMessage
		if err := json.Unmarshal(trimmed, &parts); err == nil {
			var texts []string
			for _, part := range parts {
				var probe struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(bytes.TrimSpace(part), &probe); err == nil && probe.Text != "" {
					texts = append(texts, probe.Text)
				}
			}
			return strings.Join(texts, "\n")
		}
	}
	return ""
}

// firstString returns the first element of a string list, "" when empty.
func firstString(list []string) string {
	for _, s := range list {
		if s != "" {
			return s
		}
	}
	return ""
}

// parseTS parses an ISO 8601 timestamp best effort: RFC 3339 first, then the
// tolerant layouts observed in gemini-cli artifacts. Zero on failure —
// timestamps are optional everywhere.
func parseTS(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15-04-05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if ts, err := time.Parse(layout, s); err == nil {
			return ts
		}
	}
	return time.Time{}
}
