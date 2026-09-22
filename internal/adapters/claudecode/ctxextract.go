package claudecode

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// This file implements agentlog.CtxSource for Claude Code (SPEC
// context-analysis §3): one streaming pass over the session's JSONL mapped
// to the normalized IR. Same rules run on this stream for every agent; the
// adapter's own scans (listing, entries, search prefilter) are untouched and
// the shared skipped counter is not used here — the context flow is
// best-effort by nature and skips corrupt lines silently.

// claudeUsage mirrors message.usage for the context stream.
type claudeUsage struct {
	InputTokens              *int64 `json:"input_tokens"`
	OutputTokens             *int64 `json:"output_tokens"`
	CacheCreationInputTokens *int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *int64 `json:"cache_read_input_tokens"`
}

// ctxRecord mirrors one JSONL line for context extraction: only IR-relevant
// fields are declared; everything is optional (shapes are undocumented).
type ctxRecord struct {
	Type      string        `json:"type"`
	Timestamp string        `json:"timestamp"`
	Message   *ctxMsgRecord `json:"message"`
	IsCompact bool          `json:"isCompact"`
}

type ctxMsgRecord struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string or array of blocks
	Usage   json.RawMessage `json:"usage"`
}

// ctxBlock mirrors one typed content block plus its raw encoding (the
// tool_use/tool_result id is only needed for call↔result correlation).
type ctxBlock struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Name    string          `json:"name"`
	IsError bool            `json:"is_error"`
	Raw     json.RawMessage `json:"-"`
}

// ctxCall captures one in-flight tool_use waiting for its result.
type ctxCall struct {
	tool string
}

// ContextEvents implements agentlog.CtxSource.
func (a *Adapter) ContextEvents(s agentlog.Session) ([]agentlog.CtxEvent, error) {
	path, found := a.fileForSession(s.ID)
	if !found {
		return nil, fmt.Errorf("session %s not found in claude-code storage", s.ID)
	}
	events := []agentlog.CtxEvent{}
	seq := 0
	next := func() int { seq++; return seq }
	pending := map[string]ctxCall{} // tool_use id -> call facts

	err := scanCtxLines(path, func(line []byte) {
		var rec ctxRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return // corrupt line: skip silently (best-effort flow)
		}
		ts := parseCtxTimestamp(rec.Timestamp)

		// compact markers (SPEC §3): explicit flag or a /compact user text.
		// Compact events carry no usage; the analysis interpolates (§2 R4).
		if rec.IsCompact {
			events = append(events, agentlog.CtxEvent{Seq: next(), Kind: agentlog.CtxCompact, Role: "system"})
		}
		if rec.Message == nil {
			return
		}
		msg := rec.Message
		if rec.Type == "user" {
			if txt, ok := plainString(msg.Content); ok && strings.HasPrefix(strings.TrimSpace(txt), "/compact") {
				events = append(events, agentlog.CtxEvent{Seq: next(), At: ts, Kind: agentlog.CtxCompact, Role: "user"})
				return
			}
		}

		// assistant turn marker with the record's usage (exact tokens).
		if rec.Type == "assistant" {
			usage, kind := agentlog.CtxTokens{}, agentlog.CtxTokensNone
			if u := parseCtxUsage(msg.Usage); u != nil {
				usage = agentlog.CtxTokens{
					Input:      derefCtx(u.InputTokens),
					Output:     derefCtx(u.OutputTokens),
					CacheRead:  derefCtx(u.CacheReadInputTokens),
					CacheWrite: derefCtx(u.CacheCreationInputTokens),
				}
				kind = agentlog.CtxTokensExact
			}
			events = append(events, agentlog.CtxEvent{
				Seq: next(), At: ts, Kind: agentlog.CtxTurnStart, Role: "assistant",
				Tokens: usage, TokensKind: kind,
			})
		}

		blocks, ok := contentCtxBlocks(msg.Content)
		if !ok {
			return
		}
		role := msg.Role
		if role == "" {
			role = rec.Type
		}
		for _, b := range blocks {
			switch b.Type {
			case "text":
				if b.Text != "" {
					events = append(events, agentlog.CtxEvent{
						Seq: next(), At: ts, Kind: agentlog.CtxMessage, Role: role,
						ResBytes: len(b.Text),
					})
				}
			case "tool_use":
				id := rawString(b.Raw, "id")
				if id != "" {
					pending[id] = ctxCall{tool: b.Name}
				}
				events = append(events, agentlog.CtxEvent{
					Seq: next(), At: ts, Kind: agentlog.CtxToolCall, Role: role,
					Tool: b.Name, ArgsKey: ctxArgsKey(b.Name, rawField(b.Raw, "input")),
					Label: ctxCallLabel(rawField(b.Raw, "input"), b.Name),
				})
			case "tool_result":
				// result blocks reference their call via tool_use_id
				// (tool_use blocks carry "id"; results carry "tool_use_id")
				id := rawString(b.Raw, "tool_use_id")
				call, known := pending[id]
				text := ctxResultText(b.Raw)
				ev := agentlog.CtxEvent{
					Seq: next(), At: ts, Kind: agentlog.CtxToolResult, Role: "tool",
					Err:      b.IsError,
					ResBytes: len(text),
					Head:     firstLine(text),
				}
				if known {
					ev.Tool = call.tool
					delete(pending, id)
				}
				events = append(events, ev)
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return events, nil
}

// scanCtxLines streams one JSONL file line by line; fn gets each non-empty
// line. Unreadable files are silent (best-effort flow, mirrors scanFile).
func scanCtxLines(path string, fn func([]byte)) error {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, initialBuf), maxLineSize)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		fn(line)
	}
	return nil
}

func derefCtx(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// parseCtxTimestamp parses RFC 3339 best effort; zero on failure.
func parseCtxTimestamp(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if ts, err := time.Parse(time.RFC3339, s); err == nil {
		return ts
	}
	return time.Time{}
}

// parseCtxUsage defensively extracts message.usage.
func parseCtxUsage(raw json.RawMessage) *claudeUsage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil
	}
	var u claudeUsage
	if err := json.Unmarshal(trimmed, &u); err != nil {
		return nil
	}
	return &u
}

// plainString returns the content as a plain string when it is one.
func plainString(raw json.RawMessage) (string, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			return s, true
		}
	}
	return "", false
}

// contentCtxBlocks flattens message.content into blocks when it is an array.
// Each block keeps its raw encoding: the id/input fields are read lazily
// (only tool blocks need them). A malformed element is dropped, the rest
// keeps parsing (hybrid semantics, mirroring the adapter).
func contentCtxBlocks(raw json.RawMessage) ([]ctxBlock, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, false
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(trimmed, &raws); err != nil {
		return nil, false
	}
	blocks := make([]ctxBlock, 0, len(raws))
	for _, r := range raws {
		var b ctxBlock
		if err := json.Unmarshal(r, &b); err != nil {
			continue
		}
		b.Raw = r
		blocks = append(blocks, b)
	}
	return blocks, true
}

// rawField reads one top-level field of the raw block JSON (id, input).
func rawField(raw json.RawMessage, name string) json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(raw), &m); err != nil {
		return nil
	}
	return m[name]
}

// rawString is rawField as a plain string.
func rawString(raw json.RawMessage, name string) string {
	v := rawField(raw, name)
	if len(v) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return ""
	}
	return s
}

// ctxResultText flattens a tool_result's payload to text: string content
// → the string; array content → its text parts joined with newlines (the
// text the model actually re-reads). Missing/unshaped content is "".
func ctxResultText(raw json.RawMessage) string {
	content := rawField(raw, "content")
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return ""
	}
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			return s
		}
	case '[':
		var parts []struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(trimmed, &parts); err == nil {
			texts := make([]string, 0, len(parts))
			for _, p := range parts {
				if p.Text != "" {
					texts = append(texts, p.Text)
				}
			}
			return strings.Join(texts, "\n")
		}
	}
	return ""
}

// firstLine returns the first non-empty line of s, capped at 120 runes.
func firstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		if len(ln) > 120 {
			return ln[:119] + "…"
		}
		return ln
	}
	return ""
}

// ctxArgsKey normalizes a call's identity for repeat detection (SPEC §1.1):
// tool name + canonical JSON of the input (keys sorted, noise keys dropped).
func ctxArgsKey(tool string, rawInput json.RawMessage) string {
	return tool + "\x00" + ctxCanonical(rawInput)
}

// ctxCallLabel picks a short human label for a call: the natural field when
// the tool has one (path, command, pattern, …), else compact canonical JSON,
// else the tool name.
func ctxCallLabel(rawInput json.RawMessage, tool string) string {
	for _, name := range []string{"file_path", "path", "notebook_path", "command", "pattern", "url", "query"} {
		if v := rawString(rawInput, name); v != "" {
			return v
		}
	}
	if c := ctxCanonical(rawInput); c != "{}" {
		return c
	}
	return tool
}

// ctxCanonical sorts object keys recursively, drops known-noise keys, and
// renders compactly. Strings render bare (no quotes) to keep labels short.
func ctxCanonical(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "{}"
	}
	var v any
	if err := json.Unmarshal(trimmed, &v); err != nil {
		return string(trimmed)
	}
	return ctxCanonValue(v)
}

func ctxCanonValue(v any) string {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		for i := 1; i < len(keys); i++ { // insertion sort; arg objects are tiny
			for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
				keys[j], keys[j-1] = keys[j-1], keys[j]
			}
		}
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			if ctxNoiseKey(k) {
				continue
			}
			parts = append(parts, k+"="+ctxCanonValue(t[k]))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, ctxCanonValue(e))
		}
		return "[" + strings.Join(parts, ",") + "]"
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

// ctxNoiseKey reports keys that never define what a call asked for.
func ctxNoiseKey(k string) bool {
	switch k {
	case "session_id", "sessionId", "call_id", "callId", "id", "uuid", "timestamp":
		return true
	}
	return false
}
