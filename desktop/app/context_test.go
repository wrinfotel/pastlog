package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wrinfotel/pastlog/internal/adapters/codex"
)

// The context panel's backend. The App glue is agent-agnostic (one resolve +
// one CtxSource call), so one JSONL agent's golden fixture exercises the full
// surface; the cross-agent parity lives in internal/cli/context_parity_test.go.

const appCtxID = "dddd4444-4444-4444-4444-dddddddddddd"

func ctxHome(t *testing.T, ids ...string) string {
	t.Helper()
	home := t.TempDir()
	for _, id := range ids {
		name := "rollout-2026-08-03T10-00-00-" + id + ".jsonl"
		p := filepath.Join(home, ".codex", "sessions", "2026", "08", "03", name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(codex.GenerateContextFixture(id)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func TestContextGolden(t *testing.T) {
	a := homeApp(t, ctxHome(t, appCtxID))
	out, err := a.Context("dddd4444")
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if out.Status != "ok" {
		t.Fatalf("status = %q, want ok", out.Status)
	}
	if out.Session == nil || out.Session.ID != appCtxID {
		t.Fatalf("session = %+v, want id %s", out.Session, appCtxID)
	}
	p := out.Profile
	if p == nil {
		t.Fatal("profile missing")
	}
	if p.Final != 19000 || !p.FinalExact {
		t.Errorf("final = %d (exact=%v), want 19000 exact", p.Final, p.FinalExact)
	}
	if p.Turns != 8 {
		t.Errorf("turns = %d, want 8", p.Turns)
	}
	if p.Compactions != 1 {
		t.Errorf("compactions = %d, want 1", p.Compactions)
	}
	rules := map[string]bool{}
	for _, f := range p.Findings {
		rules[f.Rule] = true
		if f.Rule == "" || f.Desc == "" {
			t.Errorf("finding without rule or description: %+v", f)
		}
	}
	for _, r := range []string{"R1", "R2", "R3", "R4", "R5"} {
		if !rules[r] {
			t.Errorf("findings missing %s: %+v", r, p.Findings)
		}
	}
	// one advice line per fired rule, keyed to it
	if len(p.Advice) != 5 {
		t.Errorf("advice lines = %d, want 5", len(p.Advice))
	}
	for _, adv := range p.Advice {
		if adv.Rule == "" || adv.Text == "" {
			t.Errorf("advice without rule or text: %+v", adv)
		}
	}
	// 8 bars + one gap space at the compaction boundary
	if got, want := len([]rune(p.Sparkline)), 9; got != want {
		t.Errorf("sparkline %q has %d runes, want %d", p.Sparkline, got, want)
	}
	if p.Precision != "exact tokens" {
		t.Errorf("precision = %q, want exact tokens", p.Precision)
	}
	// the fixture's context-only records are skipped by the regular listing
	// parser and surface as the shared stderr note — same as the CLI
	if len(out.Notes) != 1 || out.Notes[0] != "1 unreadable lines skipped" {
		t.Errorf("notes = %v, want the skipped-lines note", out.Notes)
	}
}

func TestContextAmbiguousPrefix(t *testing.T) {
	a := homeApp(t, ctxHome(t, appCtxID, "dddd4444-4444-4444-4444-dddddddddd99"))
	out, err := a.Context("dddd4444")
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if out.Status != "ambiguous" {
		t.Fatalf("status = %q, want ambiguous", out.Status)
	}
	if len(out.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(out.Candidates))
	}
	if out.Profile != nil {
		t.Errorf("ambiguous outcome should carry no profile")
	}
}

func TestContextNotFound(t *testing.T) {
	a := homeApp(t, ctxHome(t, appCtxID))
	out, err := a.Context("nope1234")
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if out.Status != "notfound" {
		t.Fatalf("status = %q, want notfound", out.Status)
	}
	if len(out.Notes) != 1 || out.Notes[0] != "1 unreadable lines skipped" {
		t.Errorf("notes = %v, want the skipped-lines note", out.Notes)
	}
}
