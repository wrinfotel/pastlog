package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// twoProjectHome builds a home with two claude projects whose paths are
// siblings (/home/dev/myapp and /home/dev/myapp2): the second session file
// is the usage fixture with its recorded cwd rewritten, so the substring
// --project filter of the CLI over-matches while the GUI's exact drill-down
// must not.
func twoProjectHome(t *testing.T) string {
	t.Helper()
	home := claudeHome(t, "usage.jsonl")
	raw := fixture(t, "claudecode", "usage.jsonl")
	dst := filepath.Join(home, ".claude", "projects", "proj2",
		"3f9c81a2-2222-4222-8333-ccccccccccc2.jsonl")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, bytes.ReplaceAll(raw, []byte("/home/dev/myapp"), []byte("/home/dev/myapp2")), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func guiRowsJSON(t *testing.T, v any) []map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

// Projects must be the CLI's `stats --by project --agent X` in-process,
// keyed by the raw project path (identical to the display key on a temp
// home: the fixture cwd is not under the real user home, so tildePath is
// the identity).
func TestProjectsParityWithCLI(t *testing.T) {
	home := claudeHome(t, "usage.jsonl", "realistic.jsonl")
	a := homeApp(t, home)
	want := cliStatsJSON(t, home, "--by", "project", "--agent", "claude-code")
	out, err := a.Projects("claude-code")
	if err != nil {
		t.Fatalf("Projects: %v", err)
	}
	got := guiRowsJSON(t, out.Rows)
	if len(got) != len(want) {
		t.Fatalf("project rows = %d, want %d (CLI %v vs GUI %v)", len(got), len(want), want, got)
	}
	for i, row := range got {
		if row["project"] != want[i]["key"] {
			t.Errorf("row %d project = %v, want CLI key %v", i, row["project"], want[i]["key"])
		}
		for _, key := range []string{"sessions", "messages", "tokens", "cost_usd"} {
			if !reflect.DeepEqual(row[key], want[i][key]) {
				t.Errorf("row %d (%v) %s = %v, want %v", i, row["project"], key, row[key], want[i][key])
			}
		}
	}
}

// The project list also carries costs where the agent provides them
// (OpenCode's per-session cost): same numbers as the CLI's project rows.
func TestProjectsParityWithCLIOpenCodeCost(t *testing.T) {
	home := opencodeHome(t)
	a := homeApp(t, home)
	want := cliStatsJSON(t, home, "--by", "project", "--agent", "opencode")
	out, err := a.Projects("opencode")
	if err != nil {
		t.Fatalf("Projects: %v", err)
	}
	got := guiRowsJSON(t, out.Rows)
	if len(got) != len(want) {
		t.Fatalf("project rows = %d, want %d", len(got), len(want))
	}
	for i, row := range got {
		if row["project"] != want[i]["key"] {
			t.Errorf("row %d project = %v, want CLI key %v", i, row["project"], want[i]["key"])
		}
		for _, key := range []string{"sessions", "messages", "tokens", "cost_usd"} {
			if !reflect.DeepEqual(row[key], want[i][key]) {
				t.Errorf("row %d (%v) %s = %v, want %v", i, row["project"], key, row[key], want[i][key])
			}
		}
		if row["cost_usd"] == nil {
			t.Errorf("row %d (%v): opencode always provides a cost, got null", i, row["project"])
		}
	}
}

// ProjectStats must be the CLI's `stats --by model --agent X --project P`
// in-process (single-project home: exact and substring coincide).
func TestProjectStatsParityWithCLI(t *testing.T) {
	home := claudeHome(t, "usage.jsonl", "realistic.jsonl")
	a := homeApp(t, home)
	want := cliStatsJSON(t, home, "--by", "model", "--agent", "claude-code", "--project", "/home/dev/myapp")
	out, err := a.ProjectStats("claude-code", "/home/dev/myapp")
	if err != nil {
		t.Fatalf("ProjectStats: %v", err)
	}
	got := guiRowsJSON(t, out.Rows)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("project stats drift:\n GUI %v\n CLI %v", got, want)
	}
}

// The GUI drill-down is by exact path, not the CLI's substring: from a home
// with sibling projects myapp/myapp2, opening myapp must count only myapp's
// sessions even though the substring filter matches both. The CLI side of
// the comparison is computed too, proving the test discriminates.
func TestProjectStatsExactProjectNotSubstring(t *testing.T) {
	home := twoProjectHome(t)
	a := homeApp(t, home)

	projs, err := a.Projects("claude-code")
	if err != nil {
		t.Fatalf("Projects: %v", err)
	}
	if len(projs.Rows) != 2 {
		t.Fatalf("project rows = %d, want 2 (%+v)", len(projs.Rows), projs.Rows)
	}
	if projs.Rows[0].Project != "/home/dev/myapp" || projs.Rows[1].Project != "/home/dev/myapp2" {
		t.Errorf("projects = %q, %q; want /home/dev/myapp then /home/dev/myapp2",
			projs.Rows[0].Project, projs.Rows[1].Project)
	}
	for _, row := range projs.Rows {
		if row.Sessions != 1 {
			t.Errorf("project %q sessions = %d, want 1", row.Project, row.Sessions)
		}
	}

	out, err := a.ProjectStats("claude-code", "/home/dev/myapp")
	if err != nil {
		t.Fatalf("ProjectStats: %v", err)
	}
	// the fixture session switched models mid-way (TASK.md backlog): the
	// model drill-down shows one row per used model, each counting the
	// session ("sessions that used this model")
	if len(out.Rows) != 2 {
		t.Errorf("exact project stats = %+v, want one row per used model", out.Rows)
	}
	for _, row := range out.Rows {
		if row.Sessions != 1 {
			t.Errorf("model row %q sessions = %d, want 1", row.Key, row.Sessions)
		}
	}
	if len(out.Rows) == 2 && (out.Rows[0].Key != "claude-opus-4-1" || out.Rows[1].Key != "claude-sonnet-4-5") {
		t.Errorf("model rows = %q, %q; want ascending opus then sonnet", out.Rows[0].Key, out.Rows[1].Key)
	}

	sub := cliStatsJSON(t, home, "--by", "model", "--agent", "claude-code", "--project", "myapp")
	total := 0
	for _, row := range sub {
		total += int(row["sessions"].(float64)) // JSON round-trip: numbers are float64
	}
	// the substring over-matches both projects (2 sessions), and each split
	// session counts on both of its model rows: 2 × 2 = 4
	if total != 4 {
		t.Errorf("CLI substring --project myapp matched %d session×model rows, want 4 (fixture must over-match for this test to discriminate)", total)
	}
}

func TestProjectsValidation(t *testing.T) {
	a := homeApp(t, claudeHome(t, "usage.jsonl"))
	want := `unknown agent "nope" (available: claude-code, codex, gemini-cli, opencode, zcode)`
	if _, err := a.Projects("nope"); err == nil || err.Error() != want {
		t.Errorf("Projects unknown agent error = %v, want %q", err, want)
	}
	if _, err := a.ProjectStats("nope", "/w"); err == nil || err.Error() != want {
		t.Errorf("ProjectStats unknown agent error = %v, want %q", err, want)
	}
}

// Projects and ProjectStats are read-only over agent storage (spec §3).
func TestProjectsReadOnly(t *testing.T) {
	home := opencodeHome(t)
	a := homeApp(t, home)
	check := assertHomeUnchanged(t, home)
	defer check()

	projs, err := a.Projects("opencode")
	if err != nil {
		t.Fatalf("Projects: %v", err)
	}
	if len(projs.Rows) == 0 {
		t.Fatal("opencode fixture must have projects")
	}
	if _, err := a.ProjectStats("opencode", projs.Rows[0].Project); err != nil {
		t.Fatalf("ProjectStats: %v", err)
	}
}
