package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wrinfotel/pastlog/internal/cli"
)

func cliStatsJSON(t *testing.T, home string, args ...string) []map[string]any {
	t.Helper()
	var out, errOut bytes.Buffer
	full := append([]string{"stats", "--json", "--home", home}, args...)
	if code := cli.Execute(full, &out, &errOut); code != 0 {
		t.Fatalf("pastlog %v exited %d (stderr: %s)", full, code, errOut.String())
	}
	var want []map[string]any
	if err := json.Unmarshal(out.Bytes(), &want); err != nil {
		t.Fatalf("CLI output unmarshal: %v", err)
	}
	return want
}

func guiStatsJSON(t *testing.T, a *App, o StatsOptions) []map[string]any {
	t.Helper()
	got, err := a.Stats(o)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	raw, err := json.Marshal(got.Rows)
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

// Parity runs the GUI and the CLI over identical options and compares the
// JSON structs (deep-equal after a JSON round-trip, so only schema-visible
// differences can show up).
func TestStatsParityWithCLI(t *testing.T) {
	home := claudeHome(t, "usage.jsonl")
	a := homeApp(t, home)
	cases := []struct {
		name string
		cli  []string
		opts StatsOptions
	}{
		{"by agent", []string{"--by", "agent"}, StatsOptions{By: "agent"}},
		{"by project", []string{"--by", "project"}, StatsOptions{By: "project"}},
		{"by day", []string{"--by", "day"}, StatsOptions{By: "day"}},
		{"by model", []string{"--by", "model"}, StatsOptions{By: "model"}},
		{"model filter", []string{"--by", "model", "--model", "claude"}, StatsOptions{By: "model", Model: "claude"}},
		{"model filter miss", []string{"--by", "model", "--model", "nope"}, StatsOptions{By: "model", Model: "nope"}},
		{"agent filter", []string{"--by", "agent", "--agent", "claude-code"}, StatsOptions{By: "agent", Filter: FilterOptions{Agent: "claude-code"}}},
		{"project filter", []string{"--by", "agent", "--project", "myapp"}, StatsOptions{By: "agent", Filter: FilterOptions{Project: "myapp"}}},
		{"since", []string{"--by", "day", "--since", "2026-08-03"}, StatsOptions{By: "day", Filter: FilterOptions{Since: "2026-08-03"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := cliStatsJSON(t, home, tc.cli...)
			got := guiStatsJSON(t, a, tc.opts)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("stats drift:\n GUI %v\n CLI %v", got, want)
			}
		})
	}
}

func TestStatsDefaultByIsAgent(t *testing.T) {
	home := claudeHome(t, "usage.jsonl")
	a := homeApp(t, home)
	got, err := a.Stats(StatsOptions{})
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if got.By != "agent" {
		t.Errorf("default --by = %q, want agent", got.By)
	}
}

func TestStatsInvalidBy(t *testing.T) {
	a := homeApp(t, claudeHome(t, "usage.jsonl"))
	if _, err := a.Stats(StatsOptions{By: "week"}); err == nil ||
		err.Error() != `invalid --by value "week": use agent, project, day or model` {
		t.Errorf("invalid --by error = %v", err)
	}
}

func TestStatsUnknownAgentAndBadFilter(t *testing.T) {
	a := homeApp(t, claudeHome(t, "usage.jsonl"))
	if _, err := a.Stats(StatsOptions{Filter: FilterOptions{Agent: "nope"}}); err == nil {
		t.Error("unknown agent accepted")
	}
	if _, err := a.Stats(StatsOptions{Filter: FilterOptions{Since: "5x"}}); err == nil {
		t.Error("invalid since accepted")
	}
}

func TestStatsEmptySelectionIsEmptyNotError(t *testing.T) {
	// A home with no agents at all: the selection is truly empty and stats
	// stays a non-error empty result (stats has no grep semantics).
	empty := t.TempDir()
	a := homeApp(t, empty)
	check := assertHomeUnchanged(t, empty)
	defer check()
	got, err := a.Stats(StatsOptions{})
	if err != nil {
		t.Fatalf("empty selection must not error: %v", err)
	}
	if len(got.Rows) != 0 {
		t.Errorf("empty selection rows = %d, want 0", len(got.Rows))
	}

	// A zero-usage session still counts as a session (TASK.md semantics) —
	// parity with the CLI decides, not zero rows.
	home := claudeHome(t, "realistic.jsonl")
	b := homeApp(t, home)
	want := cliStatsJSON(t, home, "--by", "agent")
	if !reflect.DeepEqual(guiStatsJSON(t, b, StatsOptions{By: "agent"}), want) {
		t.Error("zero-usage selection must match the CLI aggregate")
	}
}

func TestStatsExportParityWithCLI(t *testing.T) {
	home := claudeHome(t, "usage.jsonl")
	a := homeApp(t, home)
	check := assertHomeUnchanged(t, home)
	defer check()

	dest := filepath.Join(t.TempDir(), "stats.json")
	if err := a.ExportStats(StatsOptions{By: "model"}, dest); err != nil {
		t.Fatalf("ExportStats: %v", err)
	}
	var cliOut, cliErr bytes.Buffer
	if code := cli.Execute([]string{"stats", "--json", "--by", "model", "--home", home}, &cliOut, &cliErr); code != 0 {
		t.Fatalf("CLI stats exited %d: %s", code, cliErr.String())
	}
	exported, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(exported, cliOut.Bytes()) {
		t.Errorf("GUI stats export is not byte-equal to CLI --json:\nGUI %s\nCLI %s", exported, cliOut.Bytes())
	}
}
