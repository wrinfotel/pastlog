package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wrinfotel/pastlog/internal/cli"
	"github.com/wrinfotel/pastlog/internal/render"
)

// Synthetic homes built from the adapter testdata fixtures (same pattern as
// internal/adapters/claudecode/adapter_test.go buildHome).

const claudeFixtureSession = "3f9c81a2-1111-4222-8333-cccccccccccc"

func fixture(t *testing.T, agent, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "internal", "adapters", agent, "testdata", name))
	if err != nil {
		t.Fatalf("fixture %s/%s: %v", agent, name, err)
	}
	return raw
}

func claudeHome(t *testing.T, names ...string) string {
	t.Helper()
	home := t.TempDir()
	for i, name := range names {
		rel := filepath.Join(".claude", "projects", "proj",
			"3f9c81a2-1111-4222-8333-cccccccccc"+string(rune('a'+i))+".jsonl")
		dst := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, fixture(t, "claudecode", name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func homeApp(t *testing.T, home string) *App {
	t.Helper()
	a := New(WithConfigDir(filepath.Join(t.TempDir(), "cfg")))
	if _, err := a.SetHome(home); err != nil {
		t.Fatalf("SetHome: %v", err)
	}
	return a
}

func TestOverviewDetectsClaudeFixture(t *testing.T) {
	a := homeApp(t, claudeHome(t, "realistic.jsonl"))
	out := a.Overview()
	if out.Error != "" {
		t.Fatalf("unexpected error: %q", out.Error)
	}
	if out.Home == "" {
		t.Error("home must be populated")
	}
	if len(out.Agents) != 4 {
		t.Fatalf("agent rows = %d, want 4", len(out.Agents))
	}
	detected := 0
	for _, row := range out.Agents {
		switch {
		case row.Name == "claude-code":
			if !row.Detected || row.Sessions != 1 {
				t.Errorf("claude-code row = %+v, want detected with 1 session", row)
			}
		case row.Detected:
			t.Errorf("%s must not be detected on a claude-only home", row.Name)
		}
		if row.Detected {
			detected++
		}
	}
	if detected != 1 {
		t.Errorf("detected rows = %d, want 1", detected)
	}
}

func TestOverviewEmptyHome(t *testing.T) {
	a := homeApp(t, t.TempDir())
	out := a.Overview()
	if out.Error != "" {
		t.Fatalf("empty home is not an error: %q", out.Error)
	}
	for _, row := range out.Agents {
		if row.Detected {
			t.Errorf("%s unexpectedly detected on an empty home", row.Name)
		}
	}
}

func TestOverviewBadHomeSurfacesError(t *testing.T) {
	a := New(WithConfigDir(filepath.Join(t.TempDir(), "cfg")))
	missing := filepath.Join(t.TempDir(), "missing")
	a.cfg.Home = missing // simulate a persisted override whose target vanished
	out := a.Overview()
	if out.Error == "" {
		t.Error("a missing home must surface the error for the friendly empty state")
	}
}

func TestDiagnosticsSurfacesSkippedLines(t *testing.T) {
	a := homeApp(t, claudeHome(t, "realistic.jsonl", "truncated.jsonl"))
	out := a.Diagnostics()
	if out.Skipped == 0 {
		t.Error("truncated fixture must produce skipped lines")
	}
	var found bool
	for _, note := range out.Warnings {
		if strings.Contains(note, "unreadable lines skipped") {
			found = true
		}
	}
	if !found {
		t.Errorf("warnings must carry the skipped total, got %v", out.Warnings)
	}
}

// runCLI executes the real CLI in-process — the strongest parity net the
// desktop services can be held to.
func runCLI(t *testing.T, args ...string) []byte {
	t.Helper()
	var out, errOut bytes.Buffer
	if code := cli.Execute(args, &out, &errOut); code != 0 {
		t.Fatalf("pastlog %v exited %d (stderr: %s)", args, code, errOut.String())
	}
	return out.Bytes()
}

func TestSessionsParityWithCLI(t *testing.T) {
	home := claudeHome(t, "realistic.jsonl")
	a := homeApp(t, home)
	cases := []struct {
		name string
		args []string
		opts FilterOptions
	}{
		{"no filter", []string{"sessions", "--json"}, FilterOptions{}},
		{"agent", []string{"sessions", "--json", "--agent", "claude-code"}, FilterOptions{Agent: "claude-code"}},
		{"project", []string{"sessions", "--json", "--project", "myapp"}, FilterOptions{Project: "myapp"}},
		{"limit", []string{"sessions", "--json", "--limit", "1"}, FilterOptions{Limit: 1}},
		{"since date", []string{"sessions", "--json", "--since", "2026-08-03"}, FilterOptions{Since: "2026-08-03"}},
		{"until date", []string{"sessions", "--json", "--until", "2026-08-03"}, FilterOptions{Until: "2026-08-03"}},
		{"project miss", []string{"sessions", "--json", "--project", "nomatch"}, FilterOptions{Project: "nomatch"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var want []render.SessionJSON
			if err := json.Unmarshal(runCLI(t, append(tc.args, "--home", home)...), &want); err != nil {
				t.Fatalf("CLI output unmarshal: %v", err)
			}
			got, err := a.Sessions(tc.opts)
			if err != nil {
				t.Fatalf("Sessions: %v", err)
			}
			if !reflect.DeepEqual(got.Sessions, want) {
				t.Errorf("sessions drift:\n GUI  %+v\n CLI  %+v", got.Sessions, want)
			}
		})
	}
}

func TestSessionsFilterErrors(t *testing.T) {
	a := homeApp(t, claudeHome(t, "realistic.jsonl"))
	if _, err := a.Sessions(FilterOptions{Agent: "nope"}); err == nil ||
		err.Error() != `unknown agent "nope" (available: claude-code, codex, gemini-cli, opencode)` {
		t.Errorf("unknown agent error = %v", err)
	}
	if _, err := a.Sessions(FilterOptions{Limit: -1}); err == nil ||
		err.Error() != "invalid limit -1: use a non-negative number" {
		t.Errorf("negative limit error = %v", err)
	}
	if _, err := a.Sessions(FilterOptions{Since: "5x"}); err == nil ||
		!strings.Contains(err.Error(), `invalid --since value "5x"`) {
		t.Errorf("invalid since error = %v", err)
	}
	if _, err := a.Sessions(FilterOptions{Until: "nonsense"}); err == nil ||
		!strings.Contains(err.Error(), `invalid --until value "nonsense"`) {
		t.Errorf("invalid until error = %v", err)
	}
}
