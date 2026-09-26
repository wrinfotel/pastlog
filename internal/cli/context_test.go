package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// jsonString encodes s as one JSON string literal (escapes included) —
// results with newlines must look exactly like real JSONL lines.
var jsonString = func(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// ctxFixtureContent builds one claude-code session exercising every rule:
// usage per turn (exact), a repeated read (R2), an oversized tool result
// (R1), an error loop (R3), a compaction boundary (R4), and a big-turn jump
// (R5). Line order mirrors real logs: turn → calls → results → …
func ctxFixtureContent() string {
	var b strings.Builder
	line := func(s string) { b.WriteString(s); b.WriteString("\n") }
	rec := func(s string) { line(s) }

	rec(`{"type":"summary","summary":"Debug flaky build","sessionId":"cccc3333-3333-4333-8333-cccccccccccc"}`)
	// turn 1: user prompt, assistant thinks+calls Read on main.go
	rec(`{"type":"user","sessionId":"cccc3333-3333-4333-8333-cccccccccccc","cwd":"/home/dev/app","timestamp":"2026-08-03T10:00:00Z","message":{"role":"user","content":"build is failing, look into it"}}`)
	rec(`{"type":"assistant","sessionId":"cccc3333-3333-4333-8333-cccccccccccc","cwd":"/home/dev/app","timestamp":"2026-08-03T10:00:05Z","message":{"role":"assistant","content":[{"type":"text","text":"checking the build first"},{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/home/dev/app/main.go"}}],"usage":{"input_tokens":1200,"output_tokens":100,"cache_read_input_tokens":0,"cache_creation_input_tokens":300}}}`)
	rec(`{"type":"user","sessionId":"cccc3333-3333-4333-8333-cccccccccccc","timestamp":"2026-08-03T10:00:06Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"package main\nfunc main() {}\n"}]}}`)
	// turn 2: run tests → first failure
	rec(`{"type":"assistant","sessionId":"cccc3333-3333-4333-8333-cccccccccccc","cwd":"/home/dev/app","timestamp":"2026-08-03T10:00:10Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"go test ./..."}}],"usage":{"input_tokens":1400,"output_tokens":60,"cache_read_input_tokens":1200,"cache_creation_input_tokens":100}}}`)
	rec(`{"type":"user","sessionId":"cccc3333-3333-4333-8333-cccccccccccc","timestamp":"2026-08-03T10:00:12Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t2","is_error":true,"content":"FAIL github.com/dev/app 0.5s\nbuild failed: undefined: Config"}]}}`)
	// turn 3: giant build log result (R1: most of all result bytes); the
	// log is embedded with \n escapes, exactly like a real JSONL line
	rec(`{"type":"assistant","sessionId":"cccc3333-3333-4333-8333-cccccccccccc","cwd":"/home/dev/app","timestamp":"2026-08-03T10:00:20Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t3","name":"Bash","input":{"command":"go build -v ./... 2>&1 | tee /tmp/build.log; cat /tmp/build.log"}}],"usage":{"input_tokens":1600,"output_tokens":50,"cache_read_input_tokens":1400,"cache_creation_input_tokens":100}}}`)
	log := strings.Repeat("building package github.com/dev/app/internal ...\n", 400)
	line(`{"type":"user","sessionId":"cccc3333-3333-4333-8333-cccccccccccc","timestamp":"2026-08-03T10:00:24Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t3","content":` + jsonString(log) + `}]}}`)
	// turn 4: three identical test failures in a row (R3 fires on t4–t6)
	rec(`{"type":"assistant","sessionId":"cccc3333-3333-4333-8333-cccccccccccc","cwd":"/home/dev/app","timestamp":"2026-08-03T10:00:30Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t4","name":"Bash","input":{"command":"go test ./..."}}],"usage":{"input_tokens":30000,"output_tokens":40,"cache_read_input_tokens":1600,"cache_creation_input_tokens":28400}}}`)
	rec(`{"type":"user","sessionId":"cccc3333-3333-4333-8333-cccccccccccc","timestamp":"2026-08-03T10:00:31Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t4","is_error":true,"content":"FAIL github.com/dev/app 0.5s\nbuild failed: undefined: Config"}]}}`)
	rec(`{"type":"assistant","sessionId":"cccc3333-3333-4333-8333-cccccccccccc","cwd":"/dev/null","timestamp":"2026-08-03T10:00:35Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t5","name":"Bash","input":{"command":"go test ./..."}}],"usage":{"input_tokens":31000,"output_tokens":40,"cache_read_input_tokens":30000,"cache_creation_input_tokens":1000}}}`)
	rec(`{"type":"user","sessionId":"cccc3333-3333-4333-8333-cccccccccccc","timestamp":"2026-08-03T10:00:36Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t5","is_error":true,"content":"FAIL github.com/dev/app 0.5s\nbuild failed: undefined: Config"}]}}`)
	rec(`{"type":"assistant","sessionId":"cccc3333-3333-4333-8333-cccccccccccc","cwd":"/home/dev/app","timestamp":"2026-08-03T10:00:38Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t6","name":"Bash","input":{"command":"go test ./..."}}],"usage":{"input_tokens":32000,"output_tokens":30,"cache_read_input_tokens":31000,"cache_creation_input_tokens":1000}}}`)
	rec(`{"type":"user","sessionId":"cccc3333-3333-4333-8333-cccccccccccc","timestamp":"2026-08-03T10:00:39Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t6","is_error":true,"content":"FAIL github.com/dev/app 0.5s\nbuild failed: undefined: Config"}]}}`)
	// compaction boundary after turn 5, then usage drops (R4)
	rec(`{"type":"user","sessionId":"cccc3333-3333-4333-8333-cccccccccccc","timestamp":"2026-08-03T10:00:40Z","isCompact":true,"message":{"role":"user","content":"[Request interrupted by user for tool use]compact"}}`)
	rec(`{"type":"assistant","sessionId":"cccc3333-3333-4333-8333-cccccccccccc","cwd":"/home/dev/app","timestamp":"2026-08-03T10:00:45Z","message":{"role":"assistant","content":[{"type":"text","text":"context compacted; continuing with the fix"}],"usage":{"input_tokens":9000,"output_tokens":30,"cache_read_input_tokens":0,"cache_creation_input_tokens":9000}}}`)
	// re-read of main.go after edits (R2: turn gap between t1 and t9)
	rec(`{"type":"assistant","sessionId":"cccc3333-3333-4333-8333-cccccccccccc","cwd":"/home/dev/app","timestamp":"2026-08-03T10:00:50Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t9","name":"Read","input":{"file_path":"/home/dev/app/main.go"}}],"usage":{"input_tokens":9500,"output_tokens":40,"cache_read_input_tokens":9000,"cache_creation_input_tokens":500}}}`)
	rec(`{"type":"user","sessionId":"cccc3333-3333-4333-8333-cccccccccccc","timestamp":"2026-08-03T10:00:51Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t9","content":"package main\nfunc main() { config := Config() }\n"}]}}`)
	return b.String()
}

// ctxFixtureHome isolates XDG and writes the context fixture session.
func ctxFixtureHome(t *testing.T) string {
	t.Helper()
	isolateDataHome(t)
	home := t.TempDir()
	p := filepath.Join(home, ".claude", "projects", "-home-dev-app", "cccc3333-3333-4333-8333-cccccccccccc.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(ctxFixtureContent()), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestContextGolden(t *testing.T) {
	home := ctxFixtureHome(t)
	code, out, errOut := run(t, "--home", home, "context", "cccc3333")
	if code != 0 {
		t.Fatalf("context exit = %d, stderr: %s", code, errOut)
	}
	for _, want := range []string{
		"context profile: cccc3333 (claude-code,",
		"R1",
		"R2",
		"R3",
		"R4",
		"R5",
		"compactions: 1",
		"advice:",
		"precision: exact tokens (claude-code)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("context output missing %q:\n%s", want, out)
		}
	}
	t.Logf("context output:\n%s", out)
}

func TestContextUnknownSession(t *testing.T) {
	home := ctxFixtureHome(t)
	code, out, errOut := run(t, "--home", home, "context", "nope")
	if code != 2 {
		t.Fatalf("unknown session exit = %d, want 2 (stderr: %s)", code, errOut)
	}
	if !strings.Contains(errOut, "no session matches") {
		t.Errorf("stderr should name the miss, got %q", errOut)
	}
	if out != "" {
		t.Errorf("stdout should stay empty, got %q", out)
	}
}
