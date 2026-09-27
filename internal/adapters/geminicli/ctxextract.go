package geminicli

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

// This file implements agentlog.CtxSource for Gemini CLI (SPEC
// context-analysis §3): one streaming pass over the session's records —
// JSONL or legacy monolithic chats.json — mapped to the normalized IR. The
// adapter's own scans (listing, entries, search prefilter) are untouched and
// the shared skipped counter is not used here — the context flow is
// best-effort by nature and skips unusable records silently.
//
// Tokens: a gemini record's tokens summary is the per-request usage, so
// Input+CacheRead is that turn's window proxy (the format has no cache-write
// field; the repo maps cached to CacheRead, SCHEMA.md). Compaction: a `$set`
// checkpoint emits one Compact event — its message snapshot is NOT replayed
// into the IR, because the snapshot is pre-compaction history that no longer
// sits in the window and replaying it would double-count result bytes (the
// entries flow replays it; the context flow must not). Error shape: a
// toolCalls status that names an error is exact; records without a status
// fall back to the uniform regex (SPEC §2.2) — observed success-ish statuses
// ("confirmed", "executed") pass through untouched.

// ctxRecord mirrors one record for context extraction: only IR-relevant
// fields are declared; everything is optional (shapes are undocumented).
type ctxRecord struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Content   json.RawMessage `json:"content"` // PartListUnion: string or []part
	ToolCalls json.RawMessage `json:"toolCalls"`
	Tokens    *ctxTokensRaw   `json:"tokens"`
	Set       json.RawMessage `json:"$set"`
}

// ctxTokensRaw mirrors a record's tokens summary (subset of tokensRaw).
type ctxTokensRaw struct {
	Input    *int64 `json:"input"`
	Output   *int64 `json:"output"`
	Cached   *int64 `json:"cached"`
	Thoughts *int64 `json:"thoughts"`
}

// ctxMapper accumulates one session's IR events.
type ctxMapper struct {
	events []agentlog.CtxEvent
	seq    int
}

func (c *ctxMapper) next() int { c.seq++; return c.seq }

func (c *ctxMapper) emit(ev agentlog.CtxEvent) {
	ev.Seq = c.next()
	c.events = append(c.events, ev)
}

// line maps one JSONL record line.
func (c *ctxMapper) line(line []byte) {
	var rec ctxRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return // corrupt line: skip silently (best-effort flow)
	}
	c.record(rec)
}

// record maps one parsed record to IR events (JSONL and legacy alike).
func (c *ctxMapper) record(rec ctxRecord) {
	ts := parseTS(rec.Timestamp)

	// compaction checkpoint: one Compact event, snapshot content not replayed
	if len(bytes.TrimSpace(rec.Set)) > 0 && !bytes.Equal(bytes.TrimSpace(rec.Set), []byte("null")) {
		c.emit(agentlog.CtxEvent{At: ts, Kind: agentlog.CtxCompact, Role: "system"})
		return
	}

	role, known := roleFor(rec.Type)
	if !known {
		return // unknown record shape (deletion records, …)
	}

	// turn marker with the record's usage (exact tokens)
	if rec.Tokens != nil {
		c.emit(agentlog.CtxEvent{
			At: ts, Kind: agentlog.CtxTurnStart, Role: "assistant",
			Tokens: agentlog.CtxTokens{
				Input:     ctxDeref(rec.Tokens.Input),
				Output:    ctxDeref(rec.Tokens.Output),
				CacheRead: ctxDeref(rec.Tokens.Cached),
				Reasoning: ctxDeref(rec.Tokens.Thoughts),
			},
			TokensKind: agentlog.CtxTokensExact,
		})
	}

	c.content(rec.Content, role, ts)
	c.toolCalls(rec.ToolCalls, ts)
}

// content maps a PartListUnion: text parts → Message events;
// functionCall parts → ToolCall; functionResponse parts → ToolResult.
// Unknown part shapes are skipped (hybrid semantics, mirroring the adapter).
func (c *ctxMapper) content(raw json.RawMessage, role string, ts time.Time) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return
	}
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil && s != "" {
			c.emit(agentlog.CtxEvent{At: ts, Kind: agentlog.CtxMessage, Role: role, ResBytes: len(s)})
		}
		return
	case '[':
	default:
		return // wrong shape: skip silently
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(trimmed, &parts); err != nil {
		return
	}
	for _, part := range parts {
		var probe struct {
			Text         json.RawMessage `json:"text"`
			FunctionCall *struct {
				Name string          `json:"name"`
				Args json.RawMessage `json:"args"`
			} `json:"functionCall"`
			FunctionResponse *struct {
				Name     string          `json:"name"`
				Response json.RawMessage `json:"response"`
			} `json:"functionResponse"`
		}
		if err := json.Unmarshal(bytes.TrimSpace(part), &probe); err != nil {
			continue // malformed part: skip, keep parsing the rest
		}
		switch {
		case probe.FunctionCall != nil:
			c.toolCall(probe.FunctionCall.Name, probe.FunctionCall.Args, nil, "", ts)
		case probe.FunctionResponse != nil:
			text := partListText(probe.FunctionResponse.Response)
			c.toolResult(probe.FunctionResponse.Name, "", text, ts)
		case len(bytes.TrimSpace(probe.Text)) > 0:
			var s string
			if err := json.Unmarshal(probe.Text, &s); err == nil && s != "" {
				c.emit(agentlog.CtxEvent{At: ts, Kind: agentlog.CtxMessage, Role: role, ResBytes: len(s)})
			}
		}
	}
}

// toolCalls maps one record's toolCalls array: each entry yields a ToolCall
// and, when a result is present, a ToolResult — in array order.
func (c *ctxMapper) toolCalls(raw json.RawMessage, ts time.Time) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return
	}
	if trimmed[0] != '[' {
		return // wrong shape: skip silently
	}
	var calls []toolCallRaw
	if err := json.Unmarshal(trimmed, &calls); err != nil {
		return
	}
	for _, tc := range calls {
		callTS := ts
		if t := parseTS(tc.Timestamp); !t.IsZero() {
			callTS = t
		}
		c.toolCall(tc.Name, tc.Args, tc.Result, tc.Status, callTS)
	}
}

// toolCall maps one call (with its inline result, when present) to IR events.
func (c *ctxMapper) toolCall(name string, args, result json.RawMessage, status string, ts time.Time) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	c.emit(agentlog.CtxEvent{
		At: ts, Kind: agentlog.CtxToolCall, Role: "assistant",
		Tool: name, ArgsKey: agentlog.CanonArgsKey(name, args),
		Label: agentlog.ToolLabel(name, args),
	})
	if text := partListText(result); text != "" {
		c.toolResult(name, status, text, ts)
	}
}

// toolResult maps one result: the tool name comes from the call it answers
// (gemini carries it inline), so no id correlation is needed.
func (c *ctxMapper) toolResult(name, status, text string, ts time.Time) {
	c.emit(agentlog.CtxEvent{
		At: ts, Kind: agentlog.CtxToolResult, Role: "tool",
		Tool:     strings.TrimSpace(name),
		Err:      agentlog.ErrFromStatus(status, text),
		ResBytes: len(text),
		Head:     agentlog.FirstLine(text),
	})
}

// ContextEvents implements agentlog.CtxSource.
func (a *Adapter) ContextEvents(s agentlog.Session) ([]agentlog.CtxEvent, error) {
	ref, found := a.sessionFor(s.ID)
	if !found {
		return nil, fmt.Errorf("session %s not found in gemini-cli storage", s.ID)
	}
	c := &ctxMapper{events: []agentlog.CtxEvent{}}
	if ref.legacy {
		a.streamLegacyCtx(ref.path, s.ID, c)
	} else {
		scanCtxLinesJSONL(ref.path, c.line)
	}
	return c.events, nil
}

// scanCtxLinesJSONL streams one JSONL file line by line; fn gets each
// non-empty line. Unreadable files are silent (best-effort flow, mirrors
// scanFile).
func scanCtxLinesJSONL(path string, fn func([]byte)) {
	f, err := os.Open(path)
	if err != nil {
		return
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
}

// streamLegacyCtx maps one session's messages inside a monolithic chats.json
// (same streaming shape as walkLegacy, minus the entry mapping). Unreadable
// stores yield no events; the context flow is best-effort.
func (a *Adapter) streamLegacyCtx(path, wantID string, c *ctxMapper) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	dec := json.NewDecoder(bufio.NewReaderSize(f, initialBuf))
	if !delim(dec, '{') {
		return
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return // truncated wrapper
		}
		key, _ := keyTok.(string)
		if key != "sessions" {
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return
			}
			continue
		}
		if !delim(dec, '[') {
			return
		}
		for dec.More() {
			var raw legacyRaw
			if err := dec.Decode(&raw); err != nil {
				return // truncated element: the rest is unreadable
			}
			if raw.SessionID != wantID {
				continue
			}
			for _, msg := range raw.Messages {
				c.line(bytes.TrimSpace(msg))
			}
		}
		if !delim(dec, ']') {
			return
		}
	}
}

func ctxDeref(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
