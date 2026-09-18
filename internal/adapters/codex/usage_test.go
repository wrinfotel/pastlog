package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// writeRolloutLines writes the given lines into one rollout file inside a
// fresh home (defensive-case tests with inline records).
func writeRolloutLines(t *testing.T, lines ...string) *Adapter {
	t.Helper()
	home := t.TempDir()
	day := filepath.Join(home, ".codex", "sessions", "2026", "07", "01")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(day, "rollout-2026-07-01T10-00-00-ddd0e1f2-7777-4777-8777-bbbb22223333.jsonl"),
		[]byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return New(home)
}

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

// TestSessionsUsageLastWins pins the M7 token semantics: total_token_usage
// values are cumulative over the rollout, so the LAST token_count record is
// the session's usage; the model comes from the LAST turn_context record.
// The fixture deliberately makes the later records strictly greater.
func TestSessionsUsageLastWins(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"2026/08/02/rollout-2026-08-02T14-03-20-f3a7b8c9-3333-4333-8333-777777777777.jsonl": "usage.jsonl",
	})
	got := listUsage(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	su := got[0]
	if su.ID != "f3a7b8c9-3333-4333-8333-777777777777" {
		t.Errorf("ID = %q", su.ID)
	}
	if su.Agent != "codex" {
		t.Errorf("Agent = %q", su.Agent)
	}
	// the LAST token_count (250/45/80/12), NOT the first (100/20/30/5) and
	// not a sum (350/65/110/17): values are cumulative
	want := agentlog.Usage{
		Input: 250, Output: 80, Reasoning: 12,
		CacheRead: 45, CacheWrite: 0,
		Model: "gpt-5.3-codex", // LAST turn_context, not the first
	}
	if su.Input != want.Input || su.Output != want.Output || su.Reasoning != want.Reasoning ||
		su.CacheRead != want.CacheRead || su.CacheWrite != want.CacheWrite {
		t.Errorf("tokens = in %d out %d reasoning %d cacheRead %d cacheWrite %d, want %+v (last total_token_usage wins)",
			su.Input, su.Output, su.Reasoning, su.CacheRead, su.CacheWrite, want)
	}
	if su.Model != want.Model {
		t.Errorf("Model = %q, want %q (last turn_context wins)", su.Model, want.Model)
	}
	if su.CostUSD != 0 || su.HasCost {
		t.Errorf("codex provides no cost, got %f/%v", su.CostUSD, su.HasCost)
	}
	if su.Messages != 1 { // same semantic as SessionsMeta: 1 user message
		t.Errorf("Messages = %d, want 1", su.Messages)
	}
	// the session bounds/size/project stay identical to SessionsMeta
	metas := listMetas(t, a)
	if su.Session != metas[0].Session {
		t.Errorf("SessionsUsage session fields drifted from SessionsMeta:\n%+v\nvs\n%+v", su.Session, metas[0].Session)
	}
}

// TestSessionsUsageWithoutUsage pins the zero case: a rollout without
// token_count/turn_context records yields zeroes and an empty model — and is
// still yielded (zero-usage sessions count in the aggregates).
func TestSessionsUsageWithoutUsage(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"2026/07/01/rollout-2026-07-01T10-00-00-a5b8c9d0-4444-4444-8444-888888888888.jsonl": "no-usage.jsonl",
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
	if su.Messages != 1 {
		t.Errorf("Messages = %d, want 1 (zero-usage sessions still count messages)", su.Messages)
	}
}

// TestTokenCountAndTurnContextRecognizedSilently pins the M7 ruling (codex
// skip-count semantics change): token_count and turn_context records move
// from skip+counted to recognized-silent — no entries, NOT counted — while
// event_msg and unknown payload types stay skip+counted.
func TestTokenCountAndTurnContextRecognizedSilently(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"2026/08/02/rollout-2026-08-02T14-03-20-f3a7b8c9-3333-4333-8333-777777777777.jsonl": "usage.jsonl",
	})
	_ = listUsage(t, a)
	if a.SkippedLines() != 0 {
		t.Errorf("SkippedLines = %d, want 0 (token_count/turn_context are recognized-silent)", a.SkippedLines())
	}
	// recognized-silent means no entries either: 1 message total
	metas := listMetas(t, a)
	if len(metas) != 1 || metas[0].Messages != 1 {
		t.Errorf("got %+v, want 1 session with 1 message (token/turn records add no entries)", metas)
	}
}

// TestTokenCountMalformedPayloadCountsSkipped pins that a token_count record
// whose payload is unusable stays in the skip+counted class (codex convention
// for known types with unusable payloads), while a payload without the
// total_token_usage object is readable and contributes zero.
func TestTokenCountMalformedPayloadCountsSkipped(t *testing.T) {
	a := writeRolloutLines(t,
		`{"timestamp":"2026-07-01T10:00:00Z","type":"session_meta","payload":{"id":"b6c9d0e1-5555-4555-8555-999999999999","cwd":"/home/dev/app"}}`,
		`{"timestamp":"2026-07-01T10:00:05Z","type":"token_count","payload":{"info":42}}`,                                       // unusable payload shape
		`{"timestamp":"2026-07-01T10:00:10Z","type":"token_count","payload":{"info":{}}}`,                                       // no total_token_usage: zero, readable
		`{"timestamp":"2026-07-01T10:00:15Z","type":"token_count","payload":{"info":{"total_token_usage":{"input_tokens":7}}}}`, // partial totals fill what they can
	)
	got := listUsage(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	if got[0].Input != 7 || got[0].Output != 0 {
		t.Errorf("Input = %d Output = %d, want 7/0 (missing fields are zero)", got[0].Input, got[0].Output)
	}
	if a.SkippedLines() != 1 {
		t.Errorf("SkippedLines = %d, want 1 (info:42 is an unusable payload)", a.SkippedLines())
	}
}

// TestTurnContextMalformedPayloadCountsSkipped pins the same for
// turn_context: an unusable payload skips and counts; a usable one is
// recognized-silent and carries the model.
func TestTurnContextMalformedPayloadCountsSkipped(t *testing.T) {
	a := writeRolloutLines(t,
		`{"timestamp":"2026-07-01T10:00:00Z","type":"session_meta","payload":{"id":"c7d0e1f2-6666-4666-8666-aaaa11112222","cwd":"/home/dev/app"}}`,
		`{"timestamp":"2026-07-01T10:00:05Z","type":"turn_context","payload":"gpt"}`,        // unusable payload shape
		`{"timestamp":"2026-07-01T10:00:10Z","type":"turn_context","payload":{"cwd":"/x"}}`, // no model: readable, contributes none
		`{"timestamp":"2026-07-01T10:00:15Z","type":"turn_context","payload":{"model":"gpt-5.3"}}`,
	)
	got := listUsage(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	if got[0].Model != "gpt-5.3" {
		t.Errorf("Model = %q, want gpt-5.3", got[0].Model)
	}
	if a.SkippedLines() != 1 {
		t.Errorf("SkippedLines = %d, want 1 (payload \"gpt\" is an unusable payload)", a.SkippedLines())
	}
}
