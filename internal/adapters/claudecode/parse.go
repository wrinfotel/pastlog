package claudecode

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
	Type      string         `json:"type"`
	Timestamp string         `json:"timestamp"`
	SessionID string         `json:"sessionId"`
	Cwd       string         `json:"cwd"`
	Summary   string         `json:"summary"`
	Message   *messageRecord `json:"message"`
}

type messageRecord struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string or array of blocks
}

// block mirrors one typed content block.
type block struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Name    string          `json:"name"`
	Input   json.RawMessage `json:"input"`   // tool_use
	Content json.RawMessage `json:"content"` // tool_result: string or [blocks]
}

// lineInfo carries the per-line facts the session scanner aggregates.
type lineInfo struct {
	sessionID string
	cwd       string
	summary   string
	ts        time.Time
}

// lineResult is the outcome of classifying one non-empty JSONL line.
type lineResult struct {
	info    lineInfo
	entries []agentlog.Entry
	ok      bool // false: the line was unreadable or an unknown shape — skipped and counted, no entries
	skip    bool // true (with ok): a readable line that also counts as skipped — hybrid semantics, see yieldContent
}

// processLine classifies one non-empty JSONL line into at most a handful of
// entries (a record is one line, so this stays small and streaming holds at
// the file level). ok=false means the line was unreadable or an unknown shape
// and must be counted as skipped. skip=true (with ok=true) marks a readable
// line that still counts as skipped: a shape-mismatched element inside an
// otherwise readable content array truncates that array, but entries parsed
// before it are kept (SCHEMA.md "hybrid semantics").
func processLine(line []byte) lineResult {
	var rec record
	if err := json.Unmarshal(line, &rec); err != nil {
		return lineResult{} // corrupt line: skipped and counted
	}
	if rec.Type == "" && rec.SessionID == "" && rec.Cwd == "" && rec.Summary == "" && rec.Message == nil {
		return lineResult{} // null or an object with none of the known fields
	}

	info := lineInfo{
		sessionID: rec.SessionID,
		cwd:       rec.Cwd,
		summary:   rec.Summary,
	}
	if rec.Timestamp != "" {
		if ts, err := time.Parse(time.RFC3339, rec.Timestamp); err == nil {
			info.ts = ts
		}
	}

	switch rec.Type {
	case "summary":
		var entries []agentlog.Entry
		if rec.Summary != "" {
			entries = append(entries, agentlog.Entry{Kind: agentlog.Summary, Text: rec.Summary})
		}
		return lineResult{info: info, entries: entries, ok: true}
	case "user", "assistant", "system":
		if rec.Message == nil {
			return lineResult{info: info} // known type, no message: skipped and counted
		}
		role := rec.Message.Role
		if role == "" {
			role = rec.Type
		}
		entries, usable, malformed := yieldContent(rec.Message.Content, role, info.ts, nil)
		if !usable {
			// known type, but content is neither a string nor an array (e.g.
			// a number): no usable message — skipped and counted (SCHEMA.md)
			return lineResult{info: info}
		}
		return lineResult{info: info, entries: entries, ok: true, skip: malformed}
	default:
		return lineResult{info: info} // unknown record type: skip and count
	}
}

// yieldContent walks message content (string or block array), appending
// entries. usable=false marks content of an unusable shape — neither a string
// nor an array — making the whole line skipped and counted (M4). malformed=
// true marks a shape-mismatched element inside an otherwise readable array:
// parsing of the array stops there, the entries parsed before it are still
// returned, and the line is additionally counted as skipped (hybrid
// semantics, SCHEMA.md).
func yieldContent(content json.RawMessage, role string, ts time.Time, acc []agentlog.Entry) (entries []agentlog.Entry, usable bool, malformed bool) {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return acc, true, false
	}
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, false, false // defensive: content comes from a parsed line
		}
		return append(acc, agentlog.Entry{Kind: agentlog.Message, Role: role, Text: s, Timestamp: ts}), true, false
	case '[':
		var raws []json.RawMessage
		if err := json.Unmarshal(trimmed, &raws); err != nil {
			return nil, false, false // defensive
		}
		for _, raw := range raws {
			b := bytes.TrimSpace(raw)
			if len(b) == 0 {
				continue
			}
			if b[0] == '"' { // bare string inside the array: treat as text
				var s string
				if err := json.Unmarshal(b, &s); err != nil {
					return acc, true, true // malformed element: keep acc, count the line
				}
				acc = append(acc, agentlog.Entry{Kind: agentlog.Message, Role: role, Text: s, Timestamp: ts})
				continue
			}
			var blk block
			if err := json.Unmarshal(b, &blk); err != nil {
				return acc, true, true // malformed block mid-array: keep acc, count the line
			}
			acc = yieldBlock(blk, role, ts, acc)
		}
		return acc, true, false
	default:
		return nil, false, false // number/bool/object content: unusable shape
	}
}

func yieldBlock(blk block, role string, ts time.Time, acc []agentlog.Entry) []agentlog.Entry {
	switch blk.Type {
	case "text":
		if blk.Text != "" {
			acc = append(acc, agentlog.Entry{Kind: agentlog.Message, Role: role, Text: blk.Text, Timestamp: ts})
		}
	case "tool_use":
		text := blk.Name
		if input := strings.TrimSpace(string(blk.Input)); input != "" && input != "null" {
			text = blk.Name + " " + input
		}
		acc = append(acc, agentlog.Entry{Kind: agentlog.ToolCall, Role: role, Text: text, Timestamp: ts})
	case "tool_result":
		acc = append(acc, agentlog.Entry{Kind: agentlog.ToolResult, Role: "tool", Text: resultText(blk), Timestamp: ts})
	default:
		// unknown block inside a readable line: ignore silently
	}
	return acc
}

// resultText flattens a tool_result content (string or block array) to text.
func resultText(blk block) string {
	trimmed := bytes.TrimSpace(blk.Content)
	if len(trimmed) == 0 {
		return ""
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			return s
		}
	}
	if trimmed[0] == '[' {
		var parts []block
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
