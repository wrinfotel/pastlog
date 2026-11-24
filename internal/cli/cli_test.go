package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrinfotel/pastlog/internal/render"
)

func TestCodexHomeDiscoveryAndReportedTotal(t *testing.T) {
	home, config := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CODEX_HOME", config)
	dir := filepath.Join(config, "sessions", "2026", "11", "21")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := `{"type":"session_meta","payload":{"id":"test-id","cwd":"/demo"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":120,"cached_input_tokens":70,"output_tokens":20,"reasoning_output_tokens":8,"total_tokens":140}}}}
`
	if err := os.WriteFile(filepath.Join(dir, "rollout-2026-11-21T11-00-00-test.jsonl"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"stats", "--agent", "codex", "--json"}
	code, out, stderr := run(t, args...)
	var rows []render.StatsRowJSON
	if err := json.Unmarshal([]byte(out), &rows); code != 0 || err != nil || len(rows) != 1 {
		t.Fatalf("stats = %q, exit = %d, error = %v, stderr = %q", out, code, err, stderr)
	}
	if got := rows[0].Tokens; got.Total != 140 || got.Output != 20 || got.Reasoning != 8 {
		t.Errorf("tokens = %+v, want total 140 with reasoning included in output", got)
	}
	code, out, _ = run(t, append([]string{"--home", home}, args...)...)
	if code != 0 || out != "[]\n" {
		t.Errorf("explicit home: exit = %d, output = %q", code, out)
	}
}

func TestClaudeConfigDirDiscoveryAndExplicitHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	fixture := searchShowHome(t)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(fixture, ".claude"))
	for _, tc := range []struct {
		name string
		home []string
		want int
	}{
		{name: "auto discovery", want: 0},
		{name: "explicit home", home: []string{"--home", home}, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append(tc.home, "search", "--agent", "claude-code", "localStorage")
			code, out, errOut := run(t, args...)
			if code != tc.want || (tc.want == 0 && !strings.Contains(out, "localStorage")) {
				t.Errorf("exit = %d, want %d; output %q; stderr %q", code, tc.want, out, errOut)
			}
		})
	}
}

func TestVersionCommand(t *testing.T) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := Execute([]string{"version"}, out, errOut)
	if code != 0 {
		t.Fatalf("version exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}
	want := "pastlog 0.0.0-dev (commit none, date unknown)\n"
	if got := out.String(); got != want {
		t.Errorf("version output = %q, want %q", got, want)
	}
}

func TestRealErrorsExit2WithLowercaseMessage(t *testing.T) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := Execute([]string{"version", "--no-such-flag"}, out, errOut)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if out.Len() != 0 {
		t.Errorf("stdout should stay empty on error, got %q", out.String())
	}
	msg := errOut.String()
	if len(msg) == 0 || msg[0] >= 'A' && msg[0] <= 'Z' {
		t.Errorf("error message should be lowercase and non-empty, got %q", msg)
	}
}
