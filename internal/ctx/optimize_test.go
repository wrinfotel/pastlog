package ctx

import (
	"strings"
	"testing"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// Fixtures for the cross-session rules (SPEC-optimize.md). Events are
// numbered per session; keys are reused verbatim across "sessions" to model
// the same call repeating.

var (
	optNoTokens = agentlog.CtxTokensNone
	optExact    = agentlog.CtxTokensExact
)

func optTurn(seq int, in int64, exact bool) agentlog.CtxEvent {
	k := optNoTokens
	if exact {
		k = optExact
	}
	return agentlog.CtxEvent{Seq: seq, Kind: agentlog.CtxTurnStart, Tokens: agentlog.CtxTokens{Input: in}, TokensKind: k}
}

func optCall(seq int, tool, key, label string) agentlog.CtxEvent {
	return agentlog.CtxEvent{Seq: seq, Kind: agentlog.CtxToolCall, Tool: tool, ArgsKey: key, Label: label}
}

func optResult(seq int, key string, bytes int, err bool, head string) agentlog.CtxEvent {
	return agentlog.CtxEvent{Seq: seq, Kind: agentlog.CtxToolResult, ArgsKey: key, ResBytes: bytes, Err: err, Head: head}
}

func findRule(rep OptimizeReport, rule string) *OptimizeFinding {
	for i := range rep.Findings {
		if rep.Findings[i].Rule == rule {
			return &rep.Findings[i]
		}
	}
	return nil
}

func observe(o *Optimizer, id string, events ...agentlog.CtxEvent) {
	o.Observe(id, events)
}

// TestR6HeavyRepeats: the same heavy command in two sessions, three runs
// total — R6 fires with the group totals, and R7 must not double-report the
// same call.
func TestR6HeavyRepeats(t *testing.T) {
	o := NewOptimizer()
	key := "Bash\x00go build -v ./..."
	for _, id := range []string{"s1", "s2"} {
		observe(o, id,
			optCall(1, "Bash", key, "go build -v ./..."),
			optResult(2, key, 3000, false, ""),
			optCall(3, "Bash", key, "go build -v ./..."),
			optResult(4, key, 3000, false, ""),
		)
	}
	observe(o, "s1",
		optCall(5, "Bash", key, "go build -v ./..."),
		optResult(6, key, 3000, false, ""),
	)
	rep := o.Report()
	f := findRule(rep, "R6")
	if f == nil {
		t.Fatalf("R6 must fire: %+v", rep.Findings)
	}
	if f.Count != 5 || f.Sessions != 2 || f.Bytes != 15000 {
		t.Errorf("R6 numbers = %d/%d/%d, want 5 occurrences, 2 sessions, 15000 bytes", f.Count, f.Sessions, f.Bytes)
	}
	if f.Label != "go build -v ./..." {
		t.Errorf("label should carry the full command, got %q", f.Label)
	}
	if !strings.Contains(f.Desc, "×5 across 2 sessions") {
		t.Errorf("desc should carry the counts, got %q", f.Desc)
	}
	if g := findRule(rep, "R7"); g != nil {
		t.Errorf("R6 takes precedence — R7 must not fire for the same call, got %+v", g)
	}
}

// TestR7CrossSessionReads: a small read repeated in three sessions is the
// re-discovery tax; the same read in only two sessions stays under the
// threshold.
func TestR7CrossSessionReads(t *testing.T) {
	o := NewOptimizer()
	key := "Read\x00main.go"
	for _, id := range []string{"s1", "s2", "s3"} {
		observe(o, id, optCall(1, "Read", key, "main.go"), optResult(2, key, 100, false, ""))
	}
	for _, id := range []string{"s1", "s2"} {
		observe(o, id, optCall(3, "Read", "Read\x00aux.go", "aux.go"), optResult(4, "Read\x00aux.go", 100, false, ""))
	}
	rep := o.Report()
	f := findRule(rep, "R7")
	if f == nil {
		t.Fatalf("R7 must fire: %+v", rep.Findings)
	}
	if f.Count != 3 || f.Sessions != 3 || f.Bytes != 300 {
		t.Errorf("R7 numbers = %d/%d/%d, want 3/3/300", f.Count, f.Sessions, f.Bytes)
	}
	if g := findRule(rep, "R7"); g.Count != 3 {
		t.Errorf("only the 3-session group may fire, got %+v", rep.Findings)
	}
}

// TestR8RecurringFailures: the same command failing across sessions burns
// its failed output; the finding counts failed bytes only, and a 2-session
// triple failure beats R7 (precedence).
func TestR8RecurringFailures(t *testing.T) {
	o := NewOptimizer()
	key := "Bash\x00go test ./..."
	observe(o, "s1",
		optCall(1, "Bash", key, "go test ./..."),
		optResult(2, key, 500, true, "FAIL app"),
		optCall(3, "Bash", key, "go test ./..."),
		optResult(4, key, 600, true, "FAIL app"),
	)
	observe(o, "s2",
		optCall(5, "Bash", key, "go test ./..."),
		optResult(6, key, 700, true, "FAIL app"),
	)
	rep := o.Report()
	f := findRule(rep, "R8")
	if f == nil {
		t.Fatalf("R8 must fire: %+v", rep.Findings)
	}
	if f.Count != 3 || f.Sessions != 2 || f.Bytes != 1800 {
		t.Errorf("R8 numbers = %d/%d/%d, want 3 fails, 2 sessions, 1800 failed bytes", f.Count, f.Sessions, f.Bytes)
	}
	if g := findRule(rep, "R7"); g != nil {
		t.Errorf("R8 takes precedence over R7 for the same group, got %+v", g)
	}
}

// TestSingleSessionRepeatsBelongToPerSessionRules: heavy and failing
// patterns inside one session are R1/R2/R3's territory — no cross finding.
func TestSingleSessionRepeatsBelongToPerSessionRules(t *testing.T) {
	o := NewOptimizer()
	heavy := "Bash\x00big"
	observe(o, "s1",
		optCall(1, "Bash", heavy, "big"),
		optResult(2, heavy, 9000, false, ""),
		optCall(3, "Bash", heavy, "big"),
		optResult(4, heavy, 9000, false, ""),
		optCall(5, "Bash", heavy, "big"),
		optResult(6, heavy, 9000, true, "boom"),
	)
	rep := o.Report()
	if len(rep.Findings) != 0 {
		t.Errorf("single-session repeats must not fire cross rules, got %+v", rep.Findings)
	}
}

// TestR9StaticPrefix: exact first turns across three sessions expose the
// always-riding prefix (the smallest one); estimated or missing usage never
// accuses.
func TestR9StaticPrefix(t *testing.T) {
	o := NewOptimizer()
	observe(o, "s1",
		optTurn(0, 17000, true), optTurn(1, 18000, true),
		optCall(2, "Read", "Read\x00main.go", "main.go"), optResult(3, "Read\x00main.go", 100, false, ""),
	)
	observe(o, "s2", optTurn(0, 18000, true), optTurn(1, 19000, true))
	observe(o, "s3", optTurn(0, 25000, true))
	rep := o.Report()
	f := findRule(rep, "R9")
	if f == nil {
		t.Fatalf("R9 must fire: %+v", rep.Findings)
	}
	if f.Sessions != 3 || f.Count != 5 {
		t.Errorf("R9 sessions/turns = %d/%d, want 3/5 (2+2+1 turns)", f.Sessions, f.Count)
	}
	if f.Tokens != 17000*5 {
		t.Errorf("R9 exposure = %d, want %d (smallest prefix × turns)", f.Tokens, 17000*5)
	}
	if !strings.Contains(f.Desc, "17k") {
		t.Errorf("desc should carry the smallest prefix, got %q", f.Desc)
	}
}

// TestR9GatedOnExactAndSize: estimated usage or a small prefix stays silent.
func TestR9GatedOnExactAndSize(t *testing.T) {
	o := NewOptimizer()
	for _, id := range []string{"s1", "s2", "s3"} {
		observe(o, id, optTurn(0, 30000, false))
	}
	if f := findRule(o.Report(), "R9"); f != nil {
		t.Errorf("estimated usage must not fire R9, got %+v", f)
	}

	o2 := NewOptimizer()
	for _, id := range []string{"s1", "s2", "s3"} {
		observe(o2, id, optTurn(0, 5000, true))
	}
	if f := findRule(o2.Report(), "R9"); f != nil {
		t.Errorf("a small prefix must not fire R9, got %+v", f)
	}
}

// TestOptimizeCapAndSort: findings sort by impact descending and the report
// keeps only the top patterns, counting the rest.
func TestOptimizeCapAndSort(t *testing.T) {
	o := NewOptimizer()
	// 12 distinct heavy commands with growing sizes, each in 2 sessions
	for i := 0; i < 12; i++ {
		key := "Bash\x00cmd" + string(rune('a'+i))
		for _, id := range []string{"s1", "s2"} {
			observe(o, id,
				optCall(1, "Bash", key, "cmd"+string(rune('a'+i))),
				optResult(2, key, 3000+i, false, ""),
				optCall(3, "Bash", key, "cmd"+string(rune('a'+i))),
				optResult(4, key, 3000+i, false, ""),
			)
		}
	}
	rep := o.Report()
	if len(rep.Findings) != maxFindings {
		t.Fatalf("findings = %d, want the top %d", len(rep.Findings), maxFindings)
	}
	if rep.Truncated != 2 {
		t.Errorf("truncated = %d, want 2", rep.Truncated)
	}
	for i := 1; i < len(rep.Findings); i++ {
		if rep.Findings[i].impact() > rep.Findings[i-1].impact() {
			t.Fatalf("findings not sorted by impact: %+v", rep.Findings)
		}
	}
	if want := 4 * (3000 + 11); rep.Findings[0].Bytes != want { // two sessions × two calls
		t.Errorf("the biggest pattern must lead, got %d bytes, want %d", rep.Findings[0].Bytes, want)
	}
}

// TestOptimizeAdvice: one advice line per fired rule, from the shared pool.
func TestOptimizeAdvice(t *testing.T) {
	o := NewOptimizer()
	heavy := "Bash\x00heavy"
	for _, id := range []string{"s1", "s2"} {
		observe(o, id,
			optCall(1, "Bash", heavy, "heavy"),
			optResult(2, heavy, 3000, true, "boom"),
			optCall(3, "Bash", heavy, "heavy"),
			optResult(4, heavy, 3000, true, "boom"),
			optCall(5, "Bash", heavy, "heavy"),
			optResult(6, heavy, 3000, true, "boom"),
		)
	}
	rep := o.Report()
	adv := rep.Advice()
	if len(adv) != 1 || adv[0].Rule != "R6" {
		t.Fatalf("advice = %+v, want exactly R6's", adv)
	}
	if adv[0].Text != advicePool["R6"].Text {
		t.Errorf("advice text %q is not the pool line", adv[0].Text)
	}
}

// TestOptimizeUnattributableResults: results without a linked call never
// crash the fold nor join a group.
func TestOptimizeUnattributableResults(t *testing.T) {
	o := NewOptimizer()
	observe(o, "s1",
		optResult(1, "", 5000, false, ""),
		optCall(2, "Read", "Read\x00x", "x"),
		optResult(3, "Read\x00x", 100, false, ""),
	)
	rep := o.Report()
	if len(rep.Findings) != 0 || rep.Sessions != 1 {
		t.Errorf("unattributable results must stay silent, got %+v (sessions %d)", rep.Findings, rep.Sessions)
	}
}
