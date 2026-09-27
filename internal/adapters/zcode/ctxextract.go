package zcode

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// This file implements agentlog.CtxSource for ZCode (SPEC context-analysis
// §3): the session's parts (same ordered cursor Entries uses) merged with
// the per-request rows of model_usage, mapped to the normalized IR. The
// adapter's own scans (listing, entries, usage) are untouched and the shared
// skipped counter is not used here — the context flow is best-effort by
// nature and skips unusable rows silently.
//
// Tokens: token facts come from model_usage, one row per model request
// (SCHEMA.md), so each row is a TurnStart with that request's window proxy
// Input+CacheRead+CacheWrite (Anthropic-style separate cache columns). Rows
// stream in started_at order and merge with the parts cursor by timestamp —
// a request lands before the parts it produced (ties included). Databases
// older than the model_usage table degrade to a token-less stream. Error
// shape: a part state.status naming an error is exact; an absent status
// falls back to the uniform regex (SPEC §2.2) — observed statuses are
// "completed"/"pending".

// ctxPart mirrors part.data for context extraction (the same shapes the
// opencode adapter observes; step-finish carries no usable usage here —
// token facts live in model_usage).
type ctxPart struct {
	Type  string        `json:"type"`
	Text  string        `json:"text"`
	Tool  string        `json:"tool"`
	State *toolStateRaw `json:"state"`
}

// ContextEvents implements agentlog.CtxSource.
func (a *Adapter) ContextEvents(s agentlog.Session) ([]agentlog.CtxEvent, error) {
	if a.dir == "" {
		return nil, fmt.Errorf("session %s not found in zcode storage", s.ID)
	}
	db, err := a.open()
	if err != nil {
		if a.markLocked(err) {
			return nil, fmt.Errorf("cannot analyze session %s: zcode database is locked", s.ID)
		}
		return nil, fmt.Errorf("cannot open zcode database: %v", err)
	}
	defer db.Close()

	var exists int
	if err := db.QueryRow(`SELECT 1 FROM session WHERE id = ?`, s.ID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("session %s not found in zcode storage", s.ID)
		}
		if a.markLocked(err) {
			return nil, fmt.Errorf("cannot analyze session %s: zcode database is locked", s.ID)
		}
		return nil, fmt.Errorf("cannot read zcode session %s: %v", s.ID, err)
	}

	// fallback compaction marker: sessions compacted by a build that records
	// the timestamp without a compaction part
	var compacting sql.NullInt64
	_ = db.QueryRow(`SELECT time_compacting FROM session WHERE id = ?`, s.ID).Scan(&compacting) //nolint:errcheck // best-effort fallback: older builds have no time_compacting column, the marker stays zero

	// the model requests: the IR's TurnStarts, in started_at order
	type ctxRequest struct {
		at     time.Time
		tokens agentlog.CtxTokens
	}
	var requests []ctxRequest
	rows, err := db.Query(`SELECT started_at, input_tokens, output_tokens, reasoning_tokens,
			cache_creation_input_tokens, cache_read_input_tokens
		FROM model_usage
		WHERE session_id = ? AND started_at IS NOT NULL
		ORDER BY started_at, id`, s.ID)
	if err != nil && !isNoSuchTable(err) {
		// older database: no telemetry table, zero turns (SCHEMA.md)
		if a.markLocked(err) {
			return nil, fmt.Errorf("cannot analyze session %s: zcode database is locked", s.ID)
		}
		return nil, fmt.Errorf("cannot read zcode usage: %v", err)
	}
	if err == nil {
		for rows.Next() {
			var startedAt int64
			var t agentlog.CtxTokens
			if err := rows.Scan(&startedAt, &t.Input, &t.Output, &t.Reasoning,
				&t.CacheWrite, &t.CacheRead); err != nil {
				continue // unreadable row: skip
			}
			requests = append(requests, ctxRequest{at: time.UnixMilli(startedAt), tokens: t})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			if a.markLocked(err) {
				return nil, fmt.Errorf("cannot analyze session %s: zcode database is locked", s.ID)
			}
			return nil, fmt.Errorf("cannot read zcode usage: %v", err)
		}
		rows.Close()
	}

	rows, err = db.Query(`SELECT m.id, m.data, p.data, p.time_created
		FROM message m LEFT JOIN part p ON p.message_id = m.id
		WHERE m.session_id = ?
		ORDER BY m.time_created, m.id, p.time_created, p.id`, s.ID)
	if err != nil {
		if a.markLocked(err) {
			return nil, fmt.Errorf("cannot analyze session %s: zcode database is locked", s.ID)
		}
		return nil, fmt.Errorf("cannot read zcode session %s: %v", s.ID, err)
	}
	defer rows.Close()

	c := &ctxMapper{events: []agentlog.CtxEvent{}}
	ri := 0
	// flushRequests emits every request that started at or before until, so
	// a TurnStart always precedes the parts it produced (ties included).
	flushRequests := func(until time.Time) {
		for ri < len(requests) {
			r := requests[ri]
			if r.at.After(until) {
				break
			}
			c.emit(agentlog.CtxEvent{
				At: r.at, Kind: agentlog.CtxTurnStart, Role: "assistant",
				Tokens: r.tokens, TokensKind: agentlog.CtxTokensExact,
			})
			ri++
		}
	}

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
		if !ts.IsZero() {
			flushRequests(ts)
		}
		c.parts(role, partData.String, ts)
	}
	if err := rows.Err(); err != nil {
		if a.markLocked(err) {
			return nil, fmt.Errorf("cannot analyze session %s: zcode database is locked", s.ID)
		}
		return nil, fmt.Errorf("cannot read zcode session %s: %v", s.ID, err)
	}
	// requests without later parts (failed/cancelled calls) still count
	for _, r := range requests[ri:] {
		c.emit(agentlog.CtxEvent{
			At: r.at, Kind: agentlog.CtxTurnStart, Role: "assistant",
			Tokens: r.tokens, TokensKind: agentlog.CtxTokensExact,
		})
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
// shapes skip silently; recognized-but-unmapped types (file, timeline,
// step-start, step-finish) contribute nothing.
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
