package opencode

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/pastlog/pastlog/internal/agentlog"
)

// partRaw mirrors the part.data JSON payload; only fields pastlog consumes
// are declared (see SCHEMA.md). Everything is optional — shapes are
// version-dependent.
type partRaw struct {
	Type  string        `json:"type"` // text | reasoning | tool | file | patch | step-start | step-finish | compaction | …
	Text  string        `json:"text"` // text and reasoning parts
	Tool  string        `json:"tool"` // tool parts: the tool name
	State *toolStateRaw `json:"state"`
}

// toolStateRaw is the tool part's state payload. input is a JSON object,
// output a plain (possibly large, observed up to ~300 KB) string.
type toolStateRaw struct {
	Status string          `json:"status"`
	Input  json.RawMessage `json:"input"`
	Output json.RawMessage `json:"output"`
}

// partToEntries maps one part.data row to entries. skip=true counts the row
// among the unreadable/unknown records (spec §8): malformed JSON, unknown
// part types, and the deliberately unmapped patch/step-*/compaction records
// (M3 brief). file parts are skipped uncounted — a recognized type pastlog
// deliberately does not map (its url is a data: URL, not searchable text;
// documented choice per the brief). Entries may be empty for readable parts
// (e.g. an empty text part has nothing to record).
func partToEntries(role, data string, ts time.Time) (entries []agentlog.Entry, skip bool) {
	var p partRaw
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return nil, true // malformed part.data
	}
	switch p.Type {
	case "text":
		if p.Text == "" {
			return nil, false // readable, nothing to record
		}
		return append(entries, agentlog.Entry{Kind: agentlog.Message, Role: role, Text: p.Text, Timestamp: ts}), false
	case "reasoning":
		if p.Text == "" {
			return nil, false
		}
		return append(entries, agentlog.Entry{Kind: agentlog.Summary, Role: "assistant", Text: p.Text, Timestamp: ts}), false
	case "tool":
		return toolEntries(p, ts), false
	case "file":
		return nil, false // recognized, deliberately unmapped (SCHEMA.md)
	default:
		// patch, step-start, step-finish, compaction, unknown types —
		// skipped and counted (M3 brief)
		return nil, true
	}
}

// toolEntries maps a tool part to a ToolCall entry (compact JSON of
// state.input, falling back to the tool name when input is missing) plus,
// when state.output is non-empty, a ToolResult entry with the output text.
func toolEntries(p partRaw, ts time.Time) []agentlog.Entry {
	var entries []agentlog.Entry
	input := ""
	name := p.Tool
	if p.State != nil {
		input = compactJSON(p.State.Input)
	}
	if input == "" {
		input = name
	}
	if input != "" {
		entries = append(entries, agentlog.Entry{Kind: agentlog.ToolCall, Role: "tool", Text: input, Timestamp: ts})
	}
	if p.State != nil {
		if out := outputText(p.State.Output); out != "" {
			entries = append(entries, agentlog.Entry{Kind: agentlog.ToolResult, Role: "tool", Text: out, Timestamp: ts})
		}
	}
	return entries
}

// compactJSON re-encodes raw without insignificant whitespace; "" when raw is
// empty, null, or not valid JSON.
func compactJSON(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, trimmed); err != nil {
		return ""
	}
	return buf.String()
}

// outputText flattens a tool state.output: a JSON string unquotes to its
// value; any other JSON value keeps its compact encoding (defensive: the
// observed format is a plain string).
func outputText(raw json.RawMessage) string {
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
	if compact := compactJSON(trimmed); compact != "" {
		return compact
	}
	return string(trimmed)
}
