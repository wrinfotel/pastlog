package codex

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/wrinfotel/pastlog/internal/agentlog"
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
	Type      string          `json:"type"` // message | function_call | custom_tool_call | …
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`   // message: string or [{type, text}]
	Name      string          `json:"name"`      // function_call / custom_tool_call
	Arguments string          `json:"arguments"` // function_call (JSON)
	Input     string          `json:"input"`     // custom_tool_call (raw text)
	CallID    string          `json:"call_id"`   // call/output correlation
	Output    json.RawMessage `json:"output"`    // function_call_output / custom_tool_call_output
	Summary   json.RawMessage `json:"summary"`   // reasoning: [{type:"summary_text", text}]
}

// textItem is one typed content/summary item.
type textItem struct {
	Type string `json:"type"` // input_text | output_text | summary_text | …
	Text string `json:"text"`
}

// tokenUsage carries one usage observation's totals for the stats flow (M7).
// The values are cumulative over the rollout, so the LAST observation wins.
type tokenUsage struct {
	input, cached, output, reasoning int64
}

// usageFields mirrors one codex usage object — every field optional — shared
// by the token_count info totals/deltas and the token_usage_record columns.
type usageFields struct {
	InputTokens           *int64 `json:"input_tokens"`
	CachedInputTokens     *int64 `json:"cached_input_tokens"`
	OutputTokens          *int64 `json:"output_tokens"`
	ReasoningOutputTokens *int64 `json:"reasoning_output_tokens"`
}

// present distinguishes "no usage object" from "an all-zero one".
func (u usageFields) present() bool {
	return u.InputTokens != nil || u.CachedInputTokens != nil ||
		u.OutputTokens != nil || u.ReasoningOutputTokens != nil
}

// usage materializes the fields; missing ones contribute zero.
func (u usageFields) usage() tokenUsage {
	return tokenUsage{
		input:     ptrVal(u.InputTokens),
		cached:    ptrVal(u.CachedInputTokens),
		output:    ptrVal(u.OutputTokens),
		reasoning: ptrVal(u.ReasoningOutputTokens),
	}
}

// tokenInfo is the info object of a token_count record: the cumulative
// totals (total_token_usage) alongside the per-turn delta (last_token_usage
// — the per-model split's building block, TASK.md backlog; the session
// totals still come from the last cumulative record, SCHEMA.md).
type tokenInfo struct {
	TotalTokenUsage usageFields `json:"total_token_usage"`
	LastTokenUsage  usageFields `json:"last_token_usage"`
}

// tokenCountPayload mirrors the top-level token_count payload of pre-0.5
// rollouts; current ones wrap the same info in an event_msg envelope.
type tokenCountPayload struct {
	Info tokenInfo `json:"info"`
}

// eventMsgPayload mirrors one event_msg envelope; payload.type selects the
// event. Only token_count carries anything pastlog consumes — every other
// event (item_completed, task_started, user_message, …) is recognized-silent.
type eventMsgPayload struct {
	Type string    `json:"type"`
	Info tokenInfo `json:"info"`
}

// tokenUsageRecordPayload mirrors the token_usage_record payload of current
// rollouts: thread_token_usage is cumulative over the rollout (a totals
// source alongside the token_count envelope), turn_token_usage is the
// turn's delta.
type tokenUsageRecordPayload struct {
	ThreadTokenUsage usageFields `json:"thread_token_usage"`
	TurnTokenUsage   usageFields `json:"turn_token_usage"`
}

// turnContextPayload mirrors the turn_context payload; only the model field
// is consumed (last non-empty wins).
type turnContextPayload struct {
	Model string `json:"model"`
}

// lineInfo carries the per-line facts the session scanner aggregates.
type lineInfo struct {
	sessionID string
	cwd       string
	ts        time.Time
	model     string      // turn_context model, "" when the record had none
	tokens    *tokenUsage // cumulative usage totals, nil when the record had none
	delta     *tokenUsage // per-turn usage delta, nil when absent (TASK.md backlog)
}

// observeTotals folds one cumulative-totals observation: values are
// cumulative in both usage series (token_count envelopes and
// token_usage_records), so the scanner keeps the LAST one.
func (li *lineInfo) observeTotals(total usageFields) {
	t := total.usage()
	li.tokens = &t
}

// observeDelta folds the token_count family's per-turn delta. Only that
// family's last_token_usage is a true delta: its values sum to the
// cumulative totals. A token_usage_record's turn_token_usage is per-response
// gross usage — the same window is counted once per API call inside a turn,
// so summing it overshoots the totals ~15× on real rollouts — and never
// feeds the split (SCHEMA.md).
func (li *lineInfo) observeDelta(last usageFields) {
	if last.present() {
		d := last.usage()
		li.delta = &d
	}
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
	case "token_count":
		// M7: recognized-silent — no entries, not counted. Pre-0.5 rollouts
		// carry usage here; current ones wrap the same info in event_msg.
		trimmed := bytes.TrimSpace(rec.Payload)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			return info, nil, false // known type, missing payload
		}
		var tp tokenCountPayload
		if err := json.Unmarshal(trimmed, &tp); err != nil {
			return info, nil, false // known type, unusable payload
		}
		info.observeTotals(tp.Info.TotalTokenUsage)
		info.observeDelta(tp.Info.LastTokenUsage)
		return info, nil, true
	case "event_msg":
		// Recognized-silent envelope since the 0.2.5 format fix: current
		// rollouts deliver token counts (and echo items, task lifecycle, …)
		// as events. token_count feeds the usage views; every other payload
		// type yields nothing and is not counted (SCHEMA.md).
		trimmed := bytes.TrimSpace(rec.Payload)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			return info, nil, false // known type, missing payload
		}
		var ep eventMsgPayload
		if err := json.Unmarshal(trimmed, &ep); err != nil {
			return info, nil, false // known type, unusable payload
		}
		if ep.Type == "token_count" {
			info.observeTotals(ep.Info.TotalTokenUsage)
			info.observeDelta(ep.Info.LastTokenUsage)
		}
		return info, nil, true
	case "token_usage_record":
		// Current rollouts' per-response usage record: thread_token_usage is
		// a second cumulative totals source (observed to track the
		// token_count envelope's total within ~2%). turn_token_usage is NOT
		// a summable delta — see observeDelta — so it is ignored.
		// Recognized-silent — no entries, not counted.
		trimmed := bytes.TrimSpace(rec.Payload)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			return info, nil, false // known type, missing payload
		}
		var rp tokenUsageRecordPayload
		if err := json.Unmarshal(trimmed, &rp); err != nil {
			return info, nil, false // known type, unusable payload
		}
		info.observeTotals(rp.ThreadTokenUsage)
		return info, nil, true
	case "turn_context":
		// M7: recognized-silent — no entries, not counted. The model is
		// last-wins in the scanner.
		trimmed := bytes.TrimSpace(rec.Payload)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			return info, nil, false // known type, missing payload
		}
		var tp turnContextPayload
		if err := json.Unmarshal(trimmed, &tp); err != nil {
			return info, nil, false // known type, unusable payload
		}
		info.model = tp.Model
		return info, nil, true
	case "world_state", "inter_agent_communication_metadata":
		// Pure metadata records interleaved by current rollouts; nothing is
		// consumed and nothing validated — recognized-silent (SCHEMA.md).
		return info, nil, true
	default:
		// compacted, … — unknown top-level types are skipped and counted
		// (SCHEMA.md, Defensive behavior).
		return info, nil, false
	}
}

// ptrVal dereferences an optional token field: nil (missing) contributes
// zero.
func ptrVal(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// responseItemEntries maps one response_item payload to entries. ok=false
// marks the whole line skipped (unusable or unknown payload shape).
func responseItemEntries(ip itemPayload, ts time.Time) (entries []agentlog.Entry, ok bool) {
	switch ip.Type {
	case "message":
		return messageEntries(ip, ts)
	case "function_call", "custom_tool_call":
		// function_call logs JSON arguments; custom_tool_call (current
		// rollouts) logs a raw input string — both fall back to the name
		args := strings.TrimSpace(ip.Arguments)
		if args == "" {
			args = strings.TrimSpace(ip.Input)
		}
		if args == "" {
			args = strings.TrimSpace(ip.Name)
		}
		if args == "" {
			return nil, true // readable line, nothing to record
		}
		role := ip.Role
		if role == "" {
			role = "assistant" // tool calls are model-issued
		}
		return append(entries, agentlog.Entry{Kind: agentlog.ToolCall, Role: role, Text: args, Timestamp: ts}), true
	case "function_call_output", "custom_tool_call_output":
		text, ok := rawText(ip.Output)
		if !ok {
			return nil, false // malformed output array poisons the line
		}
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
	case "agent_message", "additional_tools", "compaction":
		// current rollouts interleave inter-agent traffic (routing headers +
		// encrypted content), tool definitions and compaction payloads
		// (encrypted) as response items — nothing transcript-worthy.
		// Recognized-silent; compaction still marks a Compact event in the
		// context flow (ctxextract.go).
		return nil, true
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

// rawText flattens a tool output field to text: a JSON string unquotes to
// its value; an array of typed text items (custom_tool_call_output chunks,
// current rollouts) joins the item texts with newlines; any other JSON value
// is kept as its raw encoding (best effort). ok=false marks a malformed
// array item — the caller counts the line skipped.
func rawText(raw json.RawMessage) (string, bool) {
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
		var raws []json.RawMessage
		if err := json.Unmarshal(trimmed, &raws); err != nil {
			return "", false
		}
		var parts []string
		for _, raw := range raws {
			b := bytes.TrimSpace(raw)
			if len(b) == 0 {
				continue
			}
			if b[0] == '"' { // bare string inside the array
				var s string
				if err := json.Unmarshal(b, &s); err != nil {
					return "", false
				}
				parts = append(parts, s)
				continue
			}
			var item textItem
			if err := json.Unmarshal(b, &item); err != nil {
				return "", false // malformed item poisons the line
			}
			// any item type may carry text; the observed chunks are input_text
			if item.Text != "" {
				parts = append(parts, item.Text)
			}
		}
		return strings.Join(parts, "\n"), true
	default:
		return string(trimmed), true
	}
}
