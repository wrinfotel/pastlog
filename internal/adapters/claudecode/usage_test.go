package claudecode

import (
	"testing"

	"github.com/pastlog/pastlog/internal/agentlog"
)

// listUsage streams SessionsUsage through a collecting iterator.
func listUsage(t *testing.T, a *Adapter) []agentlog.SessionUsage {
	t.Helper()
	var out []agentlog.SessionUsage
	if err := a.SessionsUsage(func(su agentlog.SessionUsage) error {
		out = append(out, su)
		return nil
	}); err != nil {
		t.Fatalf("SessionsUsage: %v", err)
	}
	return out
}

// TestSessionsUsageRealisticFixture pins the exact token numbers the M7
// fixture carries: usage accumulates across the session's assistant records
// and Model is the LAST non-empty message.model.
func TestSessionsUsageRealisticFixture(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"proj/d1e5f6a7-1111-4111-8111-555555555555.jsonl": "usage.jsonl",
	})
	got := listUsage(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	su := got[0]
	if su.ID != "d1e5f6a7-1111-4111-8111-555555555555" {
		t.Errorf("ID = %q", su.ID)
	}
	if su.Agent != "claude-code" {
		t.Errorf("Agent = %q", su.Agent)
	}
	// input 120+80+60, output 45+0+55, cache write 30+0+10, cache read 200+230+300
	want := agentlog.Usage{
		Input: 260, Output: 100, Reasoning: 0,
		CacheRead: 730, CacheWrite: 40,
		Model: "claude-opus-4-1", // last non-empty model wins
	}
	if su.Input != want.Input || su.Output != want.Output || su.Reasoning != want.Reasoning ||
		su.CacheRead != want.CacheRead || su.CacheWrite != want.CacheWrite {
		t.Errorf("tokens = in %d out %d reasoning %d cacheRead %d cacheWrite %d, want %+v",
			su.Input, su.Output, su.Reasoning, su.CacheRead, su.CacheWrite, want)
	}
	if su.Model != want.Model {
		t.Errorf("Model = %q, want %q (last non-empty wins)", su.Model, want.Model)
	}
	if su.CostUSD != 0 || su.HasCost {
		t.Errorf("claude-code provides no cost, got %f/%v", su.CostUSD, su.HasCost)
	}
	if su.Messages != 4 { // same semantic as SessionsMeta: user + 3 assistant texts
		t.Errorf("Messages = %d, want 4", su.Messages)
	}
	// the session bounds/size/project stay identical to SessionsMeta
	metas := listMetas(t, a)
	if su.Session != metas[0].Session {
		t.Errorf("SessionsUsage session fields drifted from SessionsMeta:\n%+v\nvs\n%+v", su.Session, metas[0].Session)
	}
}

// TestSessionsUsageWithoutUsage pins the zero case: a session whose records
// carry no usage/model fields contributes zeroes and an empty model — and it
// is still yielded (sessions with zero usage count in the aggregates).
func TestSessionsUsageWithoutUsage(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"proj/e2f6a7b8-2222-4222-8222-666666666666.jsonl": "no-usage.jsonl",
	})
	got := listUsage(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	su := got[0]
	if su.Input != 0 || su.Output != 0 || su.Reasoning != 0 || su.CacheRead != 0 || su.CacheWrite != 0 {
		t.Errorf("usage should be all zero, got %+v", su.Usage)
	}
	if su.Model != "" {
		t.Errorf("Model = %q, want empty", su.Model)
	}
	if su.Messages != 2 {
		t.Errorf("Messages = %d, want 2 (zero-usage sessions still count messages)", su.Messages)
	}
}

// TestSessionsUsageToleratesMalformedUsage pins the defensive rule: usage of
// the wrong shape (string, null, wrong-typed field) contributes zero for that
// field and never makes the line unreadable.
func TestSessionsUsageToleratesMalformedUsage(t *testing.T) {
	a := writeHomeLine(t,
		`{"type":"assistant","sessionId":"12121212-1212-4212-8212-121212121212","cwd":"/home/dev/app","timestamp":"2026-07-01T10:00:00Z","message":{"role":"assistant","model":"claude-sonnet-4-5","content":"string usage","usage":"not-an-object"}}`,
		`{"type":"assistant","sessionId":"12121212-1212-4212-8212-121212121212","cwd":"/home/dev/app","timestamp":"2026-07-01T10:00:05Z","message":{"role":"assistant","content":"null usage","usage":null}}`,
		`{"type":"assistant","sessionId":"12121212-1212-4212-8212-121212121212","cwd":"/home/dev/app","timestamp":"2026-07-01T10:00:10Z","message":{"role":"assistant","content":"half-broken usage","usage":{"input_tokens":"many","output_tokens":45}}}`,
		`{"type":"assistant","sessionId":"12121212-1212-4212-8212-121212121212","cwd":"/home/dev/app","timestamp":"2026-07-01T10:00:15Z","message":{"role":"assistant","content":"good usage","usage":{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":2,"cache_read_input_tokens":3}}}`,
	)
	got := listUsage(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1 (malformed usage keeps the line readable)", len(got))
	}
	su := got[0]
	if su.Input != 10 || su.Output != 50 || su.CacheWrite != 2 || su.CacheRead != 3 {
		t.Errorf("tokens = in %d out %d cw %d cr %d, want 10/50/2/3 (wrong-typed input_tokens contributes zero)",
			su.Input, su.Output, su.CacheWrite, su.CacheRead)
	}
	if su.Model != "claude-sonnet-4-5" {
		t.Errorf("Model = %q, want the only non-empty model", su.Model)
	}
	if a.SkippedLines() != 0 {
		t.Errorf("SkippedLines = %d, want 0 (nothing here is unreadable)", a.SkippedLines())
	}
}
