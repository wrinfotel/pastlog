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

// processLine classifies one non-empty JSONL line into at most a handful of
// entries (a record is one line, so this stays small and streaming holds at
// the file level). ok=false means the line was unreadable or an unknown shape
// and must be counted as skipped. A nil timestamp means the record had none.
func processLine(line []byte) (info lineInfo, entries []agentlog.Entry, ok bool) {
	var rec record
	if err := json.Unmarshal(line, &rec); err != nil {
		return info, nil, false
	}
	if rec.Type == "" && rec.SessionID == "" && rec.Cwd == "" && rec.Summary == "" && rec.Message == nil {
		return info, nil, false // null or an object with none of the known fields
	}

	info.sessionID = rec.SessionID
	info.cwd = rec.Cwd
	info.summary = rec.Summary
	if rec.Timestamp != "" {
		if ts, err := time.Parse(time.RFC3339, rec.Timestamp); err == nil {
			info.ts = ts
		}
	}

	switch rec.Type {
	case "summary":
		if rec.Summary != "" {
			entries = append(entries, agentlog.Entry{Kind: agentlog.Summary, Text: rec.Summary})
		}
		return info, entries, true
	case "user", "assistant", "system":
		if rec.Message == nil {
			return info, nil, false // known type, unusable shape
		}
		role := rec.Message.Role
		if role == "" {
			role = rec.Type
		}
		entries = yieldContent(rec.Message.Content, role, info.ts, entries)
		return info, entries, true
	default:
		return info, nil, false // unknown record type: skip and count
	}
}

// yieldContent walks message content (string or block array), appending
// entries. An unusable content shape returns nil, marking the line skipped.
func yieldContent(content json.RawMessage, role string, ts time.Time, acc []agentlog.Entry) []agentlog.Entry {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return nil
	}
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil
		}
		return append(acc, agentlog.Entry{Kind: agentlog.Message, Role: role, Text: s, Timestamp: ts})
	case '[':
		var raws []json.RawMessage
		if err := json.Unmarshal(trimmed, &raws); err != nil {
			return nil
		}
		for _, raw := range raws {
			b := bytes.TrimSpace(raw)
			if len(b) == 0 {
				continue
			}
			if b[0] == '"' { // bare string inside the array: treat as text
				var s string
				if err := json.Unmarshal(b, &s); err != nil {
					return nil
				}
				acc = append(acc, agentlog.Entry{Kind: agentlog.Message, Role: role, Text: s, Timestamp: ts})
				continue
			}
			var blk block
			if err := json.Unmarshal(b, &blk); err != nil {
				return nil // malformed block poisons the line
			}
			acc = yieldBlock(blk, role, ts, acc)
		}
		return acc
	default:
		return nil // neither string nor array
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
