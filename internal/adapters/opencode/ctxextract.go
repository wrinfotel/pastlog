package opencode

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// This file implements agentlog.CtxSource for opencode (SPEC
// context-analysis §3): one ordered cursor over the session's parts — the
// same query Entries uses — mapped to the normalized IR. The adapter's own
// scans (listing, entries) are untouched and the shared skipped counter is
// not used here — the context flow is best-effort by nature and skips
// unusable rows silently.
//
// Tokens: a step-finish part carries the per-step usage, so
// Input+CacheRead+CacheWrite is that turn's window proxy (SCHEMA.md: the
// payload's `tokens` field; the real-data shape is an object
// {input, output, cache:{read, write}, reasoning}, while fixture-era shapes
// carried a scalar — both parse, the scalar mapping to Input). Compaction:
// a `compaction` part emits a Compact event, falling back to the session's
// time_compacting when the session has none. Error shape: a state.status
// naming an error is exact; an absent status falls back to the uniform
// regex (SPEC §2.2) — observed statuses are "completed"/"pending".

// ctxPart mirrors part.data for context extraction: the fields partRaw
// carries plus step-finish's tokens payload (the listing flow drops
// step-finish, so partRaw itself stays untouched).
type ctxPart struct {
	Type   string        `json:"type"`
	Text   string        `json:"text"`
	Tool   string        `json:"tool"`
	State  *toolStateRaw `json:"state"`
	Tokens json.RawMessage
}

// ContextEvents implements agentlog.CtxSource.
func (a *Adapter) ContextEvents(s agentlog.Session) ([]agentlog.CtxEvent, error) {
	if a.dir == "" {
		return nil, fmt.Errorf("session %s not found in opencode storage", s.ID)
	}
	db, err := a.open()
	if err != nil {
		if a.markLocked(err) {
			return nil, fmt.Errorf("cannot analyze session %s: opencode database is locked", s.ID)
		}
		return nil, fmt.Errorf("cannot open opencode database: %v", err)
	}
	defer db.Close()

	var exists int
	if err := db.QueryRow(`SELECT 1 FROM session WHERE id = ?`, s.ID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("session %s not found in opencode storage", s.ID)
		}
		if a.markLocked(err) {
			return nil, fmt.Errorf("cannot analyze session %s: opencode database is locked", s.ID)
		}
		return nil, fmt.Errorf("cannot read opencode session %s: %v", s.ID, err)
	}

	// fallback compaction marker: sessions compacted by an opencode build
	// that records the timestamp without a compaction part
	var compacting sql.NullInt64
	_ = db.QueryRow(`SELECT time_compacting FROM session WHERE id = ?`, s.ID).Scan(&compacting) //nolint:errcheck // best-effort fallback: older schemas have no time_compacting column, the marker stays zero

	rows, err := db.Query(`SELECT m.id, m.data, p.data, p.time_created
		FROM message m LEFT JOIN part p ON p.message_id = m.id
		WHERE m.session_id = ?
		ORDER BY m.time_created, m.id, p.time_created, p.id`, s.ID)
	if err != nil {
		if a.markLocked(err) {
			return nil, fmt.Errorf("cannot analyze session %s: opencode database is locked", s.ID)
		}
		return nil, fmt.Errorf("cannot read opencode session %s: %v", s.ID, err)
	}
	defer rows.Close()

	c := &ctxMapper{events: []agentlog.CtxEvent{}}
	var lastMsgID string
	var role string
	seen := false
	for rows.Next() {
		var msgID string
		var msgData, partData sql.NullString
		var partCreated sql.NullInt64
		if err := rows.Scan(&msgID, &msgData, &partData, &partCreated); err != nil {
			continue // unreadable row: skip
		}
		if !seen || msgID != lastMsgID {
			if r, ok := messageRole(msgData.String); ok {
				role = r
			}
			lastMsgID = msgID
			seen = true
		}
		if !partData.Valid {
			continue // message without parts (LEFT JOIN null row)
		}
		var ts time.Time
		if partCreated.Valid {
			ts = time.UnixMilli(partCreated.Int64)
		}
		c.parts(role, partData.String, ts)
	}
	if err := rows.Err(); err != nil {
		if a.markLocked(err) {
			return nil, fmt.Errorf("cannot analyze session %s: opencode database is locked", s.ID)
		}
		return nil, fmt.Errorf("cannot read opencode session %s: %v", s.ID, err)
	}

	if !c.hasCompact && compacting.Valid && compacting.Int64 > 0 {
		c.insertCompact(time.UnixMilli(compacting.Int64))
	}
	return c.events, nil
}

// ctxMapper accumulates one session's IR events.
type ctxMapper struct {
	events     []agentlog.CtxEvent
	seq        int
	hasCompact bool
}

func (c *ctxMapper) next() int { c.seq++; return c.seq }

func (c *ctxMapper) emit(ev agentlog.CtxEvent) {
	ev.Seq = c.next()
	c.events = append(c.events, ev)
}

// insertCompact adds the session-level compaction marker at its timestamp:
// before the first event recorded after it, else at the end.
func (c *ctxMapper) insertCompact(at time.Time) {
	pos := len(c.events)
	for i, ev := range c.events {
		if !ev.At.IsZero() && ev.At.After(at) {
			pos = i
			break
		}
	}
	c.events = append(c.events, agentlog.CtxEvent{})
	copy(c.events[pos+1:], c.events[pos:])
	c.events[pos] = agentlog.CtxEvent{Seq: 0, At: at, Kind: agentlog.CtxCompact, Role: "system"}
	for i := pos; i < len(c.events); i++ {
		c.events[i].Seq = i + 1
	}
	c.seq = len(c.events)
	c.hasCompact = true
}

// parts maps one part.data row to IR events. Malformed JSON and unknown
// shapes skip silently; recognized-but-unmapped types (file, patch,
// step-start) contribute nothing.
func (c *ctxMapper) parts(role, data string, ts time.Time) {
	var p ctxPart
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return // malformed part.data
	}
	switch p.Type {
	case "text":
		if p.Text == "" {
			return
		}
		c.emit(agentlog.CtxEvent{
			At: ts, Kind: agentlog.CtxMessage, Role: role, ResBytes: len(p.Text),
		})
	case "step-finish":
		tokens, ok := ctxStepTokens(p.Tokens)
		if !ok {
			return // readable part, no usable usage
		}
		c.emit(agentlog.CtxEvent{
			At: ts, Kind: agentlog.CtxTurnStart, Role: "assistant",
			Tokens: tokens, TokensKind: agentlog.CtxTokensExact,
		})
	case "compaction":
		c.hasCompact = true
		c.emit(agentlog.CtxEvent{At: ts, Kind: agentlog.CtxCompact, Role: "system"})
	case "tool":
		c.tool(p, ts)
	}
}

// tool maps a tool part: one ToolCall for state.input plus, when
// state.output is non-empty, a ToolResult with the output text.
func (c *ctxMapper) tool(p ctxPart, ts time.Time) {
	name := strings.TrimSpace(p.Tool)
	if name == "" {
		return
	}
	var input json.RawMessage
	if p.State != nil {
		input = p.State.Input
	}
	c.emit(agentlog.CtxEvent{
		At: ts, Kind: agentlog.CtxToolCall, Role: "assistant",
		Tool: name, ArgsKey: agentlog.CanonArgsKey(name, input),
		Label: agentlog.ToolLabel(name, input),
	})
	if p.State == nil {
		return
	}
	if out := outputText(p.State.Output); out != "" {
		c.emit(agentlog.CtxEvent{
			At: ts, Kind: agentlog.CtxToolResult, Role: "tool",
			Tool:     name,
			Err:      agentlog.ErrFromStatus(p.State.Status, out),
			ResBytes: len(out),
			Head:     agentlog.FirstLine(out),
		})
	}
}

// ctxStepTokens parses a step-finish tokens payload (see the file comment
// for the shapes).
func ctxStepTokens(raw json.RawMessage) (agentlog.CtxTokens, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return agentlog.CtxTokens{}, false
	}
	if trimmed[0] == '{' {
		var t struct {
			Input     *int64          `json:"input"`
			Output    *int64          `json:"output"`
			Reasoning json.RawMessage `json:"reasoning"`
			Cache     struct {
				Read  *int64 `json:"read"`
				Write *int64 `json:"write"`
			} `json:"cache"`
		}
		if err := json.Unmarshal(trimmed, &t); err != nil {
			return agentlog.CtxTokens{}, false
		}
		return agentlog.CtxTokens{
			Input:      ctxDeref(t.Input),
			Output:     ctxDeref(t.Output),
			Reasoning:  ctxReasoning(t.Reasoning),
			CacheRead:  ctxDeref(t.Cache.Read),
			CacheWrite: ctxDeref(t.Cache.Write),
		}, true
	}
	var scalar int64
	if err := json.Unmarshal(trimmed, &scalar); err == nil {
		return agentlog.CtxTokens{Input: scalar}, true
	}
	return agentlog.CtxTokens{}, false
}

// ctxReasoning accepts a reasoning value shaped as a number or as
// {output: number}.
func ctxReasoning(raw json.RawMessage) int64 {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return 0
	}
	if trimmed[0] == '{' {
		var r struct {
			Output *int64 `json:"output"`
		}
		if err := json.Unmarshal(trimmed, &r); err == nil {
			return ctxDeref(r.Output)
		}
		return 0
	}
	var n int64
	if err := json.Unmarshal(trimmed, &n); err == nil {
		return n
	}
	return 0
}

func ctxDeref(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
