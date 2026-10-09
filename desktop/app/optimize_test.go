package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrinfotel/pastlog/internal/adapters/codex"
)

// The optimize panel's backend. The golden codex fixture repeats its heavy
// command and its failures once per session, so three same-project copies
// light up R6 (3 runs ≥ 3, avg ≈ 19.6k bytes), R8 (12 failures) and R7
// (main.go re-read in all three); the 1.5k first turn stays under R9's
// 16k threshold.

func optHome(t *testing.T, ids ...string) string {
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

func TestOptimizeMergesAcrossSessions(t *testing.T) {
	a := homeApp(t, optHome(t,
		"eeee5555-5555-4555-8555-555555555501",
		"eeee5555-5555-4555-8555-555555555502",
		"eeee5555-5555-4555-8555-555555555503",
	))
	out, err := a.Optimize(OptimizeOptions{Filter: emptyFilterOpts()})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	rep := out.Report
	if rep == nil {
		t.Fatal("report missing")
	}
	if rep.Sessions != 3 {
		t.Errorf("sessions = %d, want 3", rep.Sessions)
	}
	rules := map[string]bool{}
	for _, f := range rep.Findings {
		rules[f.Rule] = true
		if f.Desc == "" || f.Sessions != 3 {
			t.Errorf("incomplete finding: %+v", f)
		}
	}
	for _, r := range []string{"R6", "R7", "R8"} {
		if !rules[r] {
			t.Errorf("findings missing %s: %+v", r, rep.Findings)
		}
	}
	if rules["R9"] {
		t.Errorf("a 1.5k first turn must not fire R9: %+v", rep.Findings)
	}
	if len(rep.Advice) != 3 {
		t.Errorf("advice lines = %d, want one per fired rule", len(rep.Advice))
	}
}

func TestOptimizeProjectFilter(t *testing.T) {
	a := homeApp(t, optHome(t,
		"eeee5555-5555-4555-8555-555555555501",
		"eeee5555-5555-4555-8555-555555555502",
	))
	f := emptyFilterOpts()
	f.Project = "/home/dev/app"
	out, err := a.Optimize(OptimizeOptions{Filter: f})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	if out.Report.Sessions != 2 || len(out.Report.Findings) == 0 {
		t.Fatalf("project scope must analyze 2 sessions with findings, got %+v", out.Report)
	}

	f.Project = "/somewhere/else"
	out, err = a.Optimize(OptimizeOptions{Filter: f})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	if out.Report.Sessions != 0 || len(out.Report.Findings) != 0 {
		t.Errorf("a foreign project must yield an empty report, got %+v", out.Report)
	}
}

// optSecretSession is a two-turn codex session whose heavy command carries a
// fake bearer token (assembled by concatenation — push protection).
func optSecretSession(id string) string {
	secret := "ghp_" + "0123456789abcdefABCD"
	cmd := fmt.Sprintf("curl -s -H \"Authorization: Bearer %s\" https://internal/api", secret)
	var b strings.Builder
	line := func(format string, args ...any) {
		fmt.Fprintf(&b, format, args...)
		b.WriteString("\n")
	}
	line(`{"timestamp":"2026-08-03T10:00:00Z","type":"session_meta","payload":{"id":%q,"cwd":"/home/dev/app"}}`, id)
	line(`{"timestamp":"2026-08-03T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"pull the ci data"}]}}`)
	for turn, at := range []string{"10:00:05", "10:00:20"} {
		line(`{"timestamp":"2026-08-03T%sZ","type":"token_count","payload":{"info":{"total_token_usage":{"input_tokens":1200,"cached_input_tokens":300,"output_tokens":100},"last_token_usage":{"input_tokens":1200,"cached_input_tokens":300,"output_tokens":100}}}}`, at)
		line(`{"timestamp":"2026-08-03T%[1]sZ","type":"response_item","payload":{"type":"function_call","name":"Bash","arguments":%[2]q,"call_id":"t%[3]d"}}`, at, cmd, turn+1)
		line(`{"timestamp":"2026-08-03T%[1]sZ","type":"response_item","payload":{"type":"function_call_output","call_id":"t%[3]d","output":%[4]q}}`, at, cmd, turn+1, strings.Repeat("x", 3000))
	}
	return b.String()
}

func TestOptimizeMaskedByDefault(t *testing.T) {
	ids := []string{"eeee5555-5555-4555-8555-555555555501", "eeee5555-5555-4555-8555-555555555502"}
	home := t.TempDir()
	for _, id := range ids {
		p := filepath.Join(home, ".codex", "sessions", "2026", "08", "03", "rollout-2026-08-03T10-00-00-"+id+".jsonl")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(optSecretSession(id)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := homeApp(t, home)

	out, err := a.Optimize(OptimizeOptions{Filter: emptyFilterOpts()})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	raw := "ghp_" + "0123456789abcdefABCD"
	for _, f := range out.Report.Findings {
		if strings.Contains(f.Label, raw) || strings.Contains(f.Desc, raw) {
			t.Fatalf("masked report leaked the token: %+v", f)
		}
	}
	if len(out.Report.Findings) == 0 || !strings.Contains(out.Report.Findings[0].Label, "ghp_…ABCD") {
		t.Errorf("the masked label should keep the key family and last four: %+v", out.Report.Findings)
	}

	if _, err := a.SetMaskSecrets(false); err != nil {
		t.Fatalf("SetMaskSecrets: %v", err)
	}
	out, err = a.Optimize(OptimizeOptions{Filter: emptyFilterOpts()})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	if len(out.Report.Findings) == 0 || !strings.Contains(out.Report.Findings[0].Label, raw) {
		t.Errorf("masking off must print the label verbatim: %+v", out.Report.Findings)
	}
}

func TestOptimizeCancelKeepsPartial(t *testing.T) {
	sink := &cancelSink{}
	a := New(WithConfigDir(t.TempDir()), WithSink(sink))
	dir := t.TempDir()
	for _, id := range []string{
		"eeee5555-5555-4555-8555-555555555501",
		"eeee5555-5555-4555-8555-555555555502",
		"eeee5555-5555-4555-8555-555555555503",
	} {
		p := filepath.Join(dir, ".codex", "sessions", "2026", "08", "03", "rollout-2026-08-03T10-00-00-"+id+".jsonl")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(codex.GenerateContextFixture(id)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sink.a = a // the sink cancels the app that owns it; wire after construction
	if _, err := a.SetHome(dir); err != nil {
		t.Fatal(err)
	}

	out, err := a.Optimize(OptimizeOptions{Filter: emptyFilterOpts()})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	if out.Report == nil || out.Report.Sessions != 1 {
		t.Errorf("post-scan cancel keeps the first session only, got %+v", out.Report)
	}
	cancelled := false
	for _, n := range out.Notes {
		if strings.Contains(n, "cancelled") {
			cancelled = true
		}
	}
	if !cancelled {
		t.Errorf("notes should carry the cancellation, got %v", out.Notes)
	}
}
