package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/search"
)

// The exported JSON shapes (R-D4) must be byte-identical to what the render
// functions write: the desktop services return these types over the same
// encoder, and the GUI export paths call the render writers directly.

var (
	jtZero = time.Time{}
	jt1    = time.Date(2026, 1, 2, 10, 30, 0, 0, time.UTC)
	jt2    = time.Date(2026, 3, 4, 23, 59, 0, 0, time.UTC)
)

func encode(t *testing.T, w func(*bytes.Buffer) error) string {
	t.Helper()
	var buf bytes.Buffer
	if err := w(&buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.String()
}

func TestSessionJSONMatchesSessionsRenderer(t *testing.T) {
	metas := []agentlog.SessionMeta{
		{Session: agentlog.Session{ID: "aaaa", Agent: "opencode", Project: "/h/p", Title: "t",
			StartedAt: jt1, EndedAt: jt2, SizeBytes: 42}, Messages: 3},
		{Session: agentlog.Session{ID: "bbbb", Agent: "codex"}, Messages: 0},
	}
	viaRenderer := encode(t, func(b *bytes.Buffer) error { return SessionsJSON(b, metas) })
	viaTypes := encode(t, func(b *bytes.Buffer) error { return writeJSON(b, SessionRowsJSON(metas)) })
	if viaRenderer != viaTypes {
		t.Errorf("sessions JSON drift:\nrenderer: %s\ntypes:    %s", viaRenderer, viaTypes)
	}
}

func TestShowDocMatchesShowRenderer(t *testing.T) {
	meta := agentlog.SessionMeta{Session: agentlog.Session{ID: "cccc", Agent: "claude-code",
		Project: "/h/p", StartedAt: jt1}, Messages: 2}
	entries := []agentlog.Entry{
		{Kind: agentlog.Message, Role: "user", Text: "hello", Timestamp: jt1},
		{Kind: agentlog.ToolCall, Role: "tool", Text: "ls -la"},
		{Kind: agentlog.ToolResult, Text: "out"},
		{Kind: agentlog.Summary, Text: "did things", Timestamp: jt2},
	}
	viaRenderer := encode(t, func(b *bytes.Buffer) error { return ShowJSON(b, meta, entries) })
	viaTypes := encode(t, func(b *bytes.Buffer) error { return writeJSON(b, NewShowDoc(meta, entries)) })
	if viaRenderer != viaTypes {
		t.Errorf("show JSON drift:\nrenderer: %s\ntypes:    %s", viaRenderer, viaTypes)
	}
}

func TestSearchResultJSONMatchesSearchRenderer(t *testing.T) {
	// Non-ASCII line pins the rune-offset conversion.
	line := "héllo wörld ünicode jwt"
	start := len([]byte(line[:strings.IndexByte(line, 'j')]))
	results := []search.Result{{
		Session: agentlog.SessionMeta{Session: agentlog.Session{ID: "dddd", Agent: "codex",
			Project: "/h/p", StartedAt: jt1}, Messages: 5},
		Hits: []search.Hit{{
			Entry:      agentlog.Entry{Kind: agentlog.Message, Role: "assistant", Timestamp: jt1},
			Context:    "previous line",
			Line:       line,
			MatchStart: start,
			MatchEnd:   start + 3,
		}},
	}}
	viaRenderer := encode(t, func(b *bytes.Buffer) error { return SearchJSON(b, results) })
	viaTypes := encode(t, func(b *bytes.Buffer) error { return writeJSON(b, NewSearchResults(results)) })
	if viaRenderer != viaTypes {
		t.Errorf("search JSON drift:\nrenderer: %s\ntypes:    %s", viaRenderer, viaTypes)
	}
}

func TestStatsRowJSONMatchesStatsRenderer(t *testing.T) {
	rows := []StatsRow{
		{Key: "opencode", Sessions: 2, Messages: 7,
			Usage: agentlog.Usage{Input: 100, Output: 20, Reasoning: 5, CacheRead: 9, CacheWrite: 11, HasCost: true, CostUSD: 0.5}},
		{Key: "codex", Sessions: 1, Messages: 1,
			Usage: agentlog.Usage{Input: 10}},
	}
	viaRenderer := encode(t, func(b *bytes.Buffer) error { return StatsJSON(b, rows) })
	viaTypes := encode(t, func(b *bytes.Buffer) error { return writeJSON(b, NewStatsRows(rows)) })
	if viaRenderer != viaTypes {
		t.Errorf("stats JSON drift:\nrenderer: %s\ntypes:    %s", viaRenderer, viaTypes)
	}
}

func TestAgentJSONMatchesAgentsRenderer(t *testing.T) {
	path := "/home/u/.claude"
	rows := []AgentRow{
		{Name: "claude-code", Detected: true, Path: path, Sessions: 3, Bytes: 1024},
		{Name: "codex", Detected: false},
	}
	viaRenderer := encode(t, func(b *bytes.Buffer) error { return AgentsJSON(b, rows) })
	viaTypes := encode(t, func(b *bytes.Buffer) error { return writeJSON(b, AgentRowsJSON(rows)) })
	if viaRenderer != viaTypes {
		t.Errorf("agents JSON drift:\nrenderer: %s\ntypes:    %s", viaRenderer, viaTypes)
	}
}

// TestSessionJSONZeroTimesAreNull pins the absent-timestamp semantics that
// the frontend relies on (displayDate "-" ⇔ null).
func TestSessionJSONZeroTimesAreNull(t *testing.T) {
	raw, err := json.Marshal(NewSessionJSON(agentlog.SessionMeta{Session: agentlog.Session{ID: "x", Agent: "a"}}))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"started_at", "ended_at"} {
		if v, ok := m[key]; !ok || v != nil {
			t.Errorf("%s = %v (present=%v), want null", key, v, ok)
		}
	}
	if _, ok := m["id"]; !ok {
		t.Error("id key missing")
	}
}
