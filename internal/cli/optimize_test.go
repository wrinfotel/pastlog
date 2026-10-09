package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The optimize report's fixtures: three same-project sessions repeating the
// same calls — a heavy authenticated fetch (R6), a failing test command
// (R8), a small read (R7) — with exact first-turn usage large enough for
// the static-prefix rule (R9).

// optFixtureToken assembles the fake secret at runtime: the literal must
// never appear contiguous in source (push protection).
func optFixtureToken() string { return "ghp_" + "0123456789abcdefABCD" }

func optFixtureSession(i int) string {
	var b strings.Builder
	line := func(s string) { b.WriteString(s); b.WriteString("\n") }
	rec := func(format string, args ...any) { line(fmt.Sprintf(format, args...)) }

	id := fmt.Sprintf("eeee5555-5555-4555-8555-%012d", i)
	day := 3 + i
	rec(`{"type":"summary","summary":"Optimize fixture %d","sessionId":%q}`, i, id)
	// turn 1: the exact 17k first turn + the small recurring read (R7)
	rec(`{"type":"assistant","sessionId":%q,"cwd":"/home/dev/app","timestamp":"2026-08-%02dT10:00:05Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/home/dev/app/main.go"}}],"usage":{"input_tokens":17000,"output_tokens":40,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`, id, day)
	rec(`{"type":"user","sessionId":%q,"timestamp":"2026-08-%02dT10:00:06Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"package main\n"}]}}`, id, day)
	// turns 2-3: the heavy authenticated fetch, twice per session (R6)
	secret := optFixtureToken()
	for j, at := range []int{10, 20} {
		rec(`{"type":"assistant","sessionId":%q,"cwd":"/home/dev/app","timestamp":"2026-08-%02dT10:%02d:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t%d","name":"Bash","input":{"command":%s}}],"usage":{"input_tokens":17200,"output_tokens":40,"cache_read_input_tokens":17000,"cache_creation_input_tokens":100}}}`,
			id, day, at, 2+j, jsonString(fmt.Sprintf("curl -s -H \"Authorization: Bearer %s\" https://internal/api | tee out.json", secret)))
		rec(`{"type":"user","sessionId":%q,"timestamp":"2026-08-%02dT10:%02d:04Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t%d","content":%s}]}}`,
			id, day, at, 2+j, jsonString(strings.Repeat("{\"item\":\"payload row for the optimize fixture\"}\n", 90)))
	}
	// turns 4-5: the failing test command, twice per session (R8)
	for j, at := range []int{30, 40} {
		rec(`{"type":"assistant","sessionId":%q,"cwd":"/home/dev/app","timestamp":"2026-08-%02dT10:%02d:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t%d","name":"Bash","input":{"command":"go test ./..."}}],"usage":{"input_tokens":17200,"output_tokens":40,"cache_read_input_tokens":17000,"cache_creation_input_tokens":100}}}`,
			id, day, at, 4+j)
		rec(`{"type":"user","sessionId":%q,"timestamp":"2026-08-%02dT10:%02d:02Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t%d","is_error":true,"content":"FAIL github.com/dev/app 0.5s\nbuild failed: undefined: Config"}]}}`,
			id, day, at, 4+j)
	}
	return b.String()
}

func optFixtureHome(t *testing.T) string {
	t.Helper()
	isolateDataHome(t)
	home := t.TempDir()
	for i := 1; i <= 3; i++ {
		p := filepath.Join(home, ".claude", "projects", "-home-dev-app",
			fmt.Sprintf("eeee5555-5555-4555-8555-%012d.jsonl", i))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(optFixtureSession(i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func TestOptimizeGolden(t *testing.T) {
	home := optFixtureHome(t)
	code, out, errOut := run(t, "--home", home, "optimize")
	if code != 0 {
		t.Fatalf("optimize exit = %d, stderr: %s", code, errOut)
	}
	for _, want := range []string{
		"optimize report: 3 sessions analyzed",
		"R6",
		"R7",
		"R8",
		"R9",
		"advice:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("optimize output missing %q:\n%s", want, out)
		}
	}
	// findings lead with the biggest impact: R9's token exposure (17k × 15
	// turns) dwarfs the byte findings
	r9 := strings.Index(out, "R9")
	r6 := strings.Index(out, "R6 ")
	if r9 == -1 || r6 == -1 || r9 > r6 {
		t.Errorf("R9 must lead the findings by impact:\n%s", out)
	}
	t.Logf("optimize output:\n%s", out)
}

func TestOptimizeMaskedByDefault(t *testing.T) {
	home := optFixtureHome(t)
	code, out, _ := run(t, "--home", home, "optimize")
	if code != 0 {
		t.Fatalf("optimize exit = %d", code)
	}
	if strings.Contains(out, optFixtureToken()) {
		t.Error("human output must mask the fixture token")
	}
	if !strings.Contains(out, "ghp_…ABCD") {
		t.Errorf("masked form ghp_…ABCD expected:\n%s", out)
	}

	code, out, _ = run(t, "--home", home, "optimize", "--no-mask")
	if code != 0 {
		t.Fatalf("optimize --no-mask exit = %d", code)
	}
	if !strings.Contains(out, optFixtureToken()) {
		t.Error("--no-mask must print the command verbatim")
	}

	// the machine channel stays verbatim, like search/timeline
	var stdout strings.Builder
	_ = stdout
	code, out, _ = run(t, "--home", home, "optimize", "--json")
	if code != 0 {
		t.Fatalf("optimize --json exit = %d", code)
	}
	if !strings.Contains(out, optFixtureToken()) {
		t.Error("--json must stay verbatim")
	}
}

func TestOptimizeJSONSchema(t *testing.T) {
	home := optFixtureHome(t)
	code, out, _ := run(t, "--home", home, "optimize", "--json", "--no-mask")
	if code != 0 {
		t.Fatalf("optimize --json exit = %d", code)
	}
	var rep struct {
		Sessions int `json:"sessions"`
		Findings []struct {
			Rule     string `json:"rule"`
			Tool     string `json:"tool"`
			Label    string `json:"label"`
			Desc     string `json:"desc"`
			Sessions int    `json:"sessions"`
			Count    int    `json:"count"`
			Bytes    int    `json:"bytes"`
			Tokens   int64  `json:"tokens"`
		} `json:"findings"`
		Advice []struct {
			Rule string `json:"rule"`
			Text string `json:"text"`
		} `json:"advice"`
		Truncated int `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if rep.Sessions != 3 {
		t.Errorf("sessions = %d, want 3", rep.Sessions)
	}
	rules := map[string]int{}
	for _, f := range rep.Findings {
		rules[f.Rule]++
		if f.Desc == "" || f.Sessions != 3 || f.Count == 0 {
			t.Errorf("incomplete finding: %+v", f)
		}
	}
	for _, r := range []string{"R6", "R7", "R8", "R9"} {
		if rules[r] != 1 {
			t.Errorf("rule %s fired %d times, want exactly 1 (%+v)", r, rules[r], rep.Findings)
		}
	}
	// R9 carries tokens and no bytes; the byte rules the reverse
	payload := strings.Repeat("{\"item\":\"payload row for the optimize fixture\"}\n", 90)
	for _, f := range rep.Findings {
		if f.Rule == "R9" && f.Tokens != 17000*15 {
			t.Errorf("R9 tokens = %d, want %d", f.Tokens, 17000*15)
		}
		if f.Rule == "R6" && (f.Bytes != 6*len(payload) || f.Tokens != 0) {
			t.Errorf("R6 bytes/tokens = %d/%d, want %d/0", f.Bytes, f.Tokens, 6*len(payload))
		}
	}
	if len(rep.Advice) != 4 {
		t.Errorf("advice lines = %d, want one per fired rule", len(rep.Advice))
	}
	if rep.Truncated != 0 {
		t.Errorf("truncated = %d, want 0", rep.Truncated)
	}
}

func TestOptimizeEmptyAndBadAgent(t *testing.T) {
	home := optFixtureHome(t)
	code, out, _ := run(t, "--home", home, "optimize", "--project", "nowhere")
	if code != 0 {
		t.Fatalf("empty optimize exit = %d", code)
	}
	if !strings.Contains(out, "no cross-session waste patterns") {
		t.Errorf("empty scope message expected, got:\n%s", out)
	}

	code, _, errOut := run(t, "--home", home, "optimize", "--agent", "bogus")
	if code != 2 {
		t.Fatalf("unknown agent exit = %d, want 2", code)
	}
	if !strings.Contains(errOut, "unknown agent") {
		t.Errorf("stderr should name the agent error, got %q", errOut)
	}
}
