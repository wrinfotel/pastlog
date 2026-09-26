package geminicli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrinfotel/pastlog/internal/agentlog"
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
// fixture carries: usage accumulates over the session's gemini records and
// Model is the LAST non-empty record model.
func TestSessionsUsageRealisticFixture(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"chats/session-2026-08-02T14-03-b7c9d0e1.jsonl": "usage.jsonl",
	})
	got := listUsage(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	su := got[0]
	if su.ID != "b7c9d0e1-5555-4555-8555-999999999999" {
		t.Errorf("ID = %q", su.ID)
	}
	if su.Agent != "gemini-cli" {
		t.Errorf("Agent = %q", su.Agent)
	}
	// input 150+0+90, output 40+5+25, cached 10+0+5, thoughts 8+0+4;
	// tool (3+0+1) and total (198+5+115) are IGNORED per the M7 brief
	want := agentlog.Usage{
		Input: 240, Output: 70, Reasoning: 12,
		CacheRead: 15, CacheWrite: 0,
		Model: "gemini-2.5-pro", // last non-empty model wins
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
		t.Errorf("gemini-cli provides no cost, got %f/%v", su.CostUSD, su.HasCost)
	}
	if su.Messages != 4 { // same semantic as SessionsMeta: 1 user + 3 assistant texts
		t.Errorf("Messages = %d, want 4", su.Messages)
	}
	// the session bounds/size/project stay identical to SessionsMeta
	metas := listMetas(t, a)
	if su.Session != metas[0].Session {
		t.Errorf("SessionsUsage session fields drifted from SessionsMeta:\n%+v\nvs\n%+v", su.Session, metas[0].Session)
	}
}

// TestSessionsUsageWithoutUsage pins the zero case: a session whose records
// carry no tokens/model fields contributes zeroes and an empty model — and
// is still yielded.
func TestSessionsUsageWithoutUsage(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"chats/session-2026-07-01T10-00-c8d0e1f2.jsonl": "no-usage.jsonl",
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

// TestSessionsUsageLegacyChatsJSON pins that the legacy monolithic store
// rides the same record processing: the fixture's first session carries
// tokens+model on its gemini record, the second has none.
func TestSessionsUsageLegacyChatsJSON(t *testing.T) {
	a := newTestAdapter(t, map[string]string{"chats.json": "chats.json"})
	got := listUsage(t, a)
	if len(got) != 2 {
		t.Fatalf("got %d sessions, want 2 (legacy chats.json)", len(got))
	}
	var s1, s2 agentlog.SessionUsage
	for _, su := range got {
		switch su.ID {
		case legacyID1:
			s1 = su
		case legacyID2:
			s2 = su
		}
	}
	if s1.Input != 55 || s1.Output != 22 || s1.CacheRead != 4 || s1.Reasoning != 6 {
		t.Errorf("legacy usage = %+v, want in 55 out 22 cached 4 thoughts 6 (tool/total ignored)", s1.Usage)
	}
	if s1.Model != "gemini-2.5-flash" {
		t.Errorf("legacy Model = %q, want gemini-2.5-flash", s1.Model)
	}
	if s1.Messages != 2 {
		t.Errorf("legacy Messages = %d, want 2", s1.Messages)
	}
	if s2.Input != 0 || s2.Output != 0 || s2.Model != "" {
		t.Errorf("second legacy session should be zero-usage, got %+v", s2.Usage)
	}
}

// TestSessionsUsageToleratesMalformedTokens pins that records with unusable
// token shapes keep the record's other behavior: a wrong-typed tokens field
// makes the RECORD skipped+counted (known type, unusable shape — the gemini
// convention), while absent/null tokens simply contribute zero.
func TestSessionsUsageToleratesMalformedTokens(t *testing.T) {
	a := writeSessionLines(t,
		`{"sessionId":"d9e1f2a3-7777-4777-8777-bbbb22223333","startTime":"2026-07-01T10:00:00Z","kind":"main","directories":["/home/dev/app"]}`,
		`{"id":"x1","timestamp":"2026-07-01T10:00:05Z","type":"user","content":"with string tokens"}`,
		`{"id":"x2","timestamp":"2026-07-01T10:00:10Z","type":"gemini","content":"bad tokens","tokens":"many"}`,
		`{"id":"x3","timestamp":"2026-07-01T10:00:15Z","type":"gemini","content":"null tokens","tokens":null}`,
		`{"id":"x4","timestamp":"2026-07-01T10:00:20Z","type":"gemini","content":"good tokens","tokens":{"input":11,"output":7,"thoughts":2}}`,
	)
	got := listUsage(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	su := got[0]
	if su.Input != 11 || su.Output != 7 || su.Reasoning != 2 {
		t.Errorf("tokens = in %d out %d reasoning %d, want 11/7/2 (the bad-tokens record contributes nothing)", su.Input, su.Output, su.Reasoning)
	}
	if su.Messages != 3 { // user + the two readable gemini texts
		t.Errorf("Messages = %d, want 3", su.Messages)
	}
	if a.SkippedLines() != 1 {
		t.Errorf("SkippedLines = %d, want 1 (tokens:\"many\" is an unusable record shape)", a.SkippedLines())
	}
}

// writeSessionLines writes the given lines into one session JSONL file
// inside a fresh home (defensive-case tests with inline records).
func writeSessionLines(t *testing.T, lines ...string) *Adapter {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, ".gemini", "tmp", "hash1337", "chats", "session-2026-07-01T10-00-d9e1f2a3.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return New(home)
}

// TestSessionsUsageModelBreakdown pins the per-model split (TASK.md
// backlog): each record's tokens land on its model, a record without a model
// stays with the model in effect, entries keep first-use order and sum to
// the session totals.
func TestSessionsUsageModelBreakdown(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"chats/session-2026-08-02T14-03-b7c9d0e1.jsonl": "usage.jsonl",
	})
	su := listUsage(t, a)[0]
	want := []agentlog.Usage{
		// the middle turn names no model → the model in effect (flash)
		{Model: "gemini-2.5-flash", Input: 150, Output: 45, CacheRead: 10, Reasoning: 8},
		{Model: "gemini-2.5-pro", Input: 90, Output: 25, CacheRead: 5, Reasoning: 4},
	}
	if len(su.Models) != len(want) {
		t.Fatalf("Models = %d entries, want %d", len(su.Models), len(want))
	}
	for i, w := range want {
		if su.Models[i] != w {
			t.Errorf("Models[%d] = %+v, want %+v", i, su.Models[i], w)
		}
	}
	if su.Models[0].Input+su.Models[1].Input != su.Input {
		t.Errorf("breakdown input %d does not sum to the session total %d",
			su.Models[0].Input+su.Models[1].Input, su.Input)
	}
	if su.Model != "gemini-2.5-pro" {
		t.Errorf("Model = %q, want the latest model (split must not touch it)", su.Model)
	}
}

// TestSessionsUsageLegacyModelBreakdown pins the same per-model split on the
// legacy monolithic path, which rides the same record processing.
func TestSessionsUsageLegacyModelBreakdown(t *testing.T) {
	a := &Adapter{}
	raw := `{"sessionId":"legacy-split-5555-5555-5555-555555555555","messages":[
		{"id":"m1","timestamp":"2026-07-01T09:00:30Z","type":"gemini","content":"first turn",
		 "model":"gemini-2.5-flash","tokens":{"input":10,"output":2,"cached":1,"thoughts":3}},
		{"id":"m2","timestamp":"2026-07-01T09:01:30Z","type":"gemini","content":"second turn",
		 "model":"gemini-2.5-pro","tokens":{"input":5,"output":1}}]}`
	su, _, ok := a.legacySession([]byte(raw))
	if !ok {
		t.Fatal("legacySession should accept a well-formed record")
	}
	want := []agentlog.Usage{
		{Model: "gemini-2.5-flash", Input: 10, Output: 2, CacheRead: 1, Reasoning: 3},
		{Model: "gemini-2.5-pro", Input: 5, Output: 1},
	}
	if len(su.Models) != len(want) {
		t.Fatalf("legacy Models = %d entries, want %d", len(su.Models), len(want))
	}
	for i, w := range want {
		if su.Models[i] != w {
			t.Errorf("legacy Models[%d] = %+v, want %+v", i, su.Models[i], w)
		}
	}
	if su.Model != "gemini-2.5-pro" {
		t.Errorf("legacy Model = %q, want gemini-2.5-pro", su.Model)
	}
}
