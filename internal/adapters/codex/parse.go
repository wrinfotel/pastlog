package codex

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/pastlog/pastlog/internal/agentlog"
)

// record mirrors one JSONL line; only fields pastlog consumes are declared
// (see SCHEMA.md). Everything is optional — shapes are undocumented.
type record struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// metaPayload is the session_meta payload.
type metaPayload struct {
	ID         string `json:"id"`
	Cwd        string `json:"cwd"`
	CLIVersion string `json:"cli_version"`
}

// itemPayload is one response_item payload; the fields used depend on the
// payload type.
type itemPayload struct {
	Type      string          `json:"type"` // message | function_call | function_call_output | reasoning | …
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`   // message: string or [{type, text}]
	Name      string          `json:"name"`      // function_call
	Arguments string          `json:"arguments"` // function_call
	CallID    string          `json:"call_id"`   // function_call_output
	Output    json.RawMessage `json:"output"`    // function_call_output
	Summary   json.RawMessage `json:"summary"`   // reasoning: [{type:"summary_text", text}]
}

// textItem is one typed content/summary item.
type textItem struct {
	Type string `json:"type"` // input_text | output_text | summary_text | …
	Text string `json:"text"`
}

// lineInfo carries the per-line facts the session scanner aggregates.
type lineInfo struct {
	sessionID string
	cwd       string
	ts        time.Time
}

// processLine classifies one non-empty JSONL line into at most a handful of
// entries (a record is one line, so this stays small and streaming holds at
// the file level). ok=false means the line was unreadable or an unknown shape
// and must be counted as skipped. A nil timestamp means the record had none.
func processLine(line []byte) (info lineInfo, entries []agentlog.Entry, ok bool) {
	var rec record
	if err := json.Unmarshal(line, &rec); err != nil {
		return info, nil, false
	}
	if rec.Timestamp != "" {
		if ts, err := time.Parse(time.RFC3339, rec.Timestamp); err == nil {
			info.ts = ts
		}
	}

	switch rec.Type {
	case "session_meta":
		trimmed := bytes.TrimSpace(rec.Payload)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			return info, nil, false // known type, missing payload
		}
		var mp metaPayload
		if err := json.Unmarshal(trimmed, &mp); err != nil {
			return info, nil, false // known type, unusable payload
		}
		info.sessionID = mp.ID
		info.cwd = mp.Cwd
		return info, nil, true
	case "response_item":
		trimmed := bytes.TrimSpace(rec.Payload)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			return info, nil, false // known type, missing payload
		}
		var ip itemPayload
		if err := json.Unmarshal(trimmed, &ip); err != nil {
			return info, nil, false
		}
		entries, ok = responseItemEntries(ip, info.ts)
		return info, entries, ok
	default:
		// event_msg, turn_context, compacted, … — unknown top-level types are
		// skipped and counted (SCHEMA.md, Defensive behavior).
		return info, nil, false
	}
}

// responseItemEntries maps one response_item payload to entries. ok=false
// marks the whole line skipped (unusable or unknown payload shape).
func responseItemEntries(ip itemPayload, ts time.Time) (entries []agentlog.Entry, ok bool) {
	switch ip.Type {
	case "message":
		return messageEntries(ip, ts)
	case "function_call":
		args := strings.TrimSpace(ip.Arguments)
		if args == "" {
			args = strings.TrimSpace(ip.Name)
		}
		if args == "" {
			return nil, true // readable line, nothing to record
		}
		role := ip.Role
		if role == "" {
			role = "assistant" // function calls are model-issued
		}
		return append(entries, agentlog.Entry{Kind: agentlog.ToolCall, Role: role, Text: args, Timestamp: ts}), true
	case "function_call_output":
		text := rawText(ip.Output)
		if strings.TrimSpace(text) == "" {
			return nil, true // readable line, nothing to record
		}
		return append(entries, agentlog.Entry{Kind: agentlog.ToolResult, Role: "tool", Text: text, Timestamp: ts}), true
	case "reasoning":
		text, ok := summaryText(ip.Summary)
		if !ok {
			return nil, false // malformed summary array poisons the line
		}
		if text == "" {
			return nil, true
		}
		return append(entries, agentlog.Entry{Kind: agentlog.Summary, Text: text, Timestamp: ts}), true
	default:
		return nil, false // unknown payload type: skip and count
	}
}

// messageEntries flattens a message payload's content (string or typed-item
// array) into Message entries, one per text part.
func messageEntries(ip itemPayload, ts time.Time) ([]agentlog.Entry, bool) {
	trimmed := bytes.TrimSpace(ip.Content)
	if len(trimmed) == 0 {
		return nil, true // readable line, empty content
	}
	role := ip.Role
	var entries []agentlog.Entry
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, false
		}
		if s != "" {
			entries = append(entries, agentlog.Entry{Kind: agentlog.Message, Role: role, Text: s, Timestamp: ts})
		}
		return entries, true
	case '[':
		var raws []json.RawMessage
		if err := json.Unmarshal(trimmed, &raws); err != nil {
			return nil, false
		}
		for _, raw := range raws {
			b := bytes.TrimSpace(raw)
			if len(b) == 0 {
				continue
			}
			if b[0] == '"' { // bare string inside the array: treat as text
				var s string
				if err := json.Unmarshal(b, &s); err != nil {
					return nil, false
				}
				if s != "" {
					entries = append(entries, agentlog.Entry{Kind: agentlog.Message, Role: role, Text: s, Timestamp: ts})
				}
				continue
			}
			var item textItem
			if err := json.Unmarshal(b, &item); err != nil {
				return nil, false // malformed item poisons the line
			}
			// input_text (user) / output_text (assistant); unknown item types
			// are ignored silently — the line itself was readable.
			if item.Text != "" && (item.Type == "input_text" || item.Type == "output_text") {
				entries = append(entries, agentlog.Entry{Kind: agentlog.Message, Role: role, Text: item.Text, Timestamp: ts})
			}
		}
		return entries, true
	default:
		return nil, false // neither string nor array
	}
}

// summaryText flattens a reasoning payload's summary (string or
// summary-item array) into one text. Malformed shapes return ok=false.
func summaryText(raw json.RawMessage) (string, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", true
	}
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return "", false
		}
		return s, true
	case '[':
		var items []json.RawMessage
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return "", false
		}
		var parts []string
		for _, r := range items {
			b := bytes.TrimSpace(r)
			if len(b) == 0 {
				continue
			}
			if b[0] == '"' {
				var s string
				if err := json.Unmarshal(b, &s); err != nil {
					return "", false
				}
				parts = append(parts, s)
				continue
			}
			var item textItem
			if err := json.Unmarshal(b, &item); err != nil {
				return "", false
			}
			// summary_text is the known type; unknown item types are ignored
			if item.Text != "" && item.Type == "summary_text" {
				parts = append(parts, item.Text)
			}
		}
		return strings.Join(parts, "\n"), true
	default:
		return "", false
	}
}

// rawText flattens a function_call_output's output field to text: a JSON
// string unquotes to its value; any other JSON value is kept as its raw
// encoding (best effort).
func rawText(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			return s
		}
	}
	return string(trimmed)
}
