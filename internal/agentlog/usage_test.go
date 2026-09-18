package agentlog

import (
	"errors"
	"strings"
	"testing"
)

// usageFake adds SessionsUsage support on top of metaFake.
type usageFake struct {
	metaFake
	usage []SessionUsage // if non-nil, returned by SessionsUsage
}

func (f *usageFake) SessionsUsage(iter func(SessionUsage) error) error {
	for _, su := range f.usage {
		if err := iter(su); err != nil {
			return err
		}
	}
	return f.scanErr
}

func usageIDs(got []SessionUsage) []string {
	out := make([]string, len(got))
	for i, su := range got {
		out[i] = su.ID
	}
	return out
}

// sampleUsage rows: three sessions across two fake agents with distinct
// models, projects, times and costs — the CollectUsage assertions below pin
// exact numbers, ordering and filtering against them.
func sampleUsage() []SessionUsage {
	return []SessionUsage{
		{
			Session:  Session{ID: "old", Project: "/home/dev/api", StartedAt: t1},
			Messages: 4,
			Usage: Usage{
				Input: 100, Output: 50, Reasoning: 10,
				CacheRead: 30, CacheWrite: 20,
				CostUSD: 0.25, HasCost: true, Model: "model-a",
			},
		},
		{
			Session:  Session{ID: "new", Project: "/home/dev/other", StartedAt: t3},
			Messages: 2,
			Usage:    Usage{Input: 7, Output: 3, Model: "model-b"},
		},
		{
			Session:  Session{ID: "mid", Project: `C:\dev\myapp`, StartedAt: t2},
			Messages: 0,
			Usage:    Usage{Model: "MODEL-C"}, // no tokens at all: still counted
		},
	}
}

func TestCollectUsageSortsNewestFirstAndSetsAgent(t *testing.T) {
	a := &usageFake{metaFake: metaFake{fakeAdapter: fakeAdapter{name: "claude-code"}}, usage: sampleUsage()}
	got := CollectUsage([]Adapter{a}, SessionFilter{}, "", nil)
	want := []string{"new", "mid", "old"} // newest first, ties by ID
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("row[%d].ID = %q, want %q", i, got[i].ID, id)
		}
		if got[i].Agent != "claude-code" {
			t.Errorf("row[%d].Agent = %q, want the adapter name", i, got[i].Agent)
		}
	}
	// exact values ride along untouched
	if got[2].Input != 100 || got[2].Output != 50 || got[2].Reasoning != 10 ||
		got[2].CacheRead != 30 || got[2].CacheWrite != 20 ||
		got[2].CostUSD != 0.25 || !got[2].HasCost || got[2].Model != "model-a" ||
		got[2].Messages != 4 {
		t.Errorf("oldest row usage drifted: %+v", got[2].Usage)
	}
}

func TestCollectUsageFilters(t *testing.T) {
	a := &usageFake{metaFake: metaFake{fakeAdapter: fakeAdapter{name: "claude-code"}}, usage: sampleUsage()}
	b := &usageFake{metaFake: metaFake{fakeAdapter: fakeAdapter{name: "codex"}}, usage: []SessionUsage{
		{Session: Session{ID: "codex", Project: "/home/dev/myapp", StartedAt: t2}, Usage: Usage{Input: 1, Model: "model-a"}},
	}}

	tests := []struct {
		name   string
		filter SessionFilter
		model  string
		want   []string
	}{
		{"no filter", SessionFilter{}, "", []string{"new", "codex", "mid", "old"}},
		{"agent", SessionFilter{Agent: "codex"}, "", []string{"codex"}},
		{"project substring case-insensitive", SessionFilter{Project: "MYAPP"}, "", []string{"codex", "mid"}},
		{"since inclusive", SessionFilter{Since: t2}, "", []string{"new", "codex", "mid"}},
		{"until inclusive", SessionFilter{Until: t2}, "", []string{"codex", "mid", "old"}},
		{"model substring case-insensitive", SessionFilter{}, "MODEL", []string{"new", "codex", "mid", "old"}},
		{"model exact-ish", SessionFilter{}, "model-b", []string{"new"}},
		{"model misses no-model rows", SessionFilter{}, "zzz", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CollectUsage([]Adapter{a, b}, tt.filter, tt.model, nil)
			if strings.Join(usageIDs(got), ",") != strings.Join(tt.want, ",") {
				t.Errorf("got %v, want %v", usageIDs(got), tt.want)
			}
		})
	}
}

// TestProjectFilterNormalizesSeparatorsOnUsage pins that the --project
// separator folding applies to the usage flow too (the mid row records a
// Windows path; the needle with forward slashes must match it and only it).
func TestProjectFilterNormalizesSeparatorsOnUsage(t *testing.T) {
	a := &usageFake{metaFake: metaFake{fakeAdapter: fakeAdapter{name: "claude-code"}}, usage: sampleUsage()}
	got := CollectUsage([]Adapter{a}, SessionFilter{Project: `dev/myapp`}, "", nil)
	if len(got) != 1 || got[0].ID != "mid" {
		t.Errorf("project filter over usage = %v, want [mid]", usageIDs(got))
	}
}

// TestCollectUsageSkipsNonUsageSourceSilently pins the M7 ruling: an adapter
// without usage data contributes nothing and must NOT emit a note — stats
// simply aggregates the rest.
func TestCollectUsageSkipsNonUsageSourceSilently(t *testing.T) {
	plain := &fakeAdapter{name: "plain-agent", sessions: []Session{{ID: "x", StartedAt: t1}}}
	a := &usageFake{metaFake: metaFake{fakeAdapter: fakeAdapter{name: "claude-code"}}, usage: sampleUsage()}
	var notes []string
	got := CollectUsage([]Adapter{plain, a}, SessionFilter{}, "", func(n string) { notes = append(notes, n) })
	if len(got) != 3 {
		t.Errorf("only the UsageSource adapter should contribute, got %v", usageIDs(got))
	}
	if len(notes) != 0 {
		t.Errorf("no note expected for a non-UsageSource adapter, got %v", notes)
	}
}

// TestCollectUsageNotesUnreadableStorageOnce pins the best-effort note: a
// failing UsageSource scan produces exactly one UnreadableNote and keeps the
// healthy adapters' rows.
func TestCollectUsageNotesUnreadableStorageOnce(t *testing.T) {
	healthy := &usageFake{metaFake: metaFake{fakeAdapter: fakeAdapter{name: "codex"}}, usage: []SessionUsage{
		{Session: Session{ID: "c", StartedAt: t1}, Usage: Usage{Input: 5}},
	}}
	broken := &usageFake{metaFake: metaFake{fakeAdapter: fakeAdapter{name: "claude-code", scanErr: errors.New("permission denied")}}}

	var notes []string
	got := CollectUsage([]Adapter{broken, healthy}, SessionFilter{}, "", func(n string) { notes = append(notes, n) })
	if len(got) != 1 || got[0].ID != "c" {
		t.Errorf("healthy adapter's usage must survive, got %v", usageIDs(got))
	}
	if len(notes) != 1 {
		t.Fatalf("got %d notes, want 1: %v", len(notes), notes)
	}
	if !strings.Contains(notes[0], "claude-code: storage unreadable") ||
		!strings.Contains(notes[0], "permission denied") {
		t.Errorf("note should name the adapter and the cause, got %q", notes[0])
	}
}

func TestCollectUsageNoteNilIsLegal(t *testing.T) {
	broken := &usageFake{metaFake: metaFake{fakeAdapter: fakeAdapter{name: "claude-code", scanErr: errors.New("permission denied")}}}
	got := CollectUsage([]Adapter{broken}, SessionFilter{}, "", nil)
	if len(got) != 0 {
		t.Errorf("expected no rows, got %v", usageIDs(got))
	}
}

// TestCollectUsageIgnoresLimit pins the M7 ruling that stats aggregates the
// whole selection: SessionFilter.Limit stays a sessions/search concern.
func TestCollectUsageIgnoresLimit(t *testing.T) {
	a := &usageFake{metaFake: metaFake{fakeAdapter: fakeAdapter{name: "claude-code"}}, usage: sampleUsage()}
	got := CollectUsage([]Adapter{a}, SessionFilter{Limit: 1}, "", nil)
	if len(got) != 3 {
		t.Errorf("limit must not truncate usage rows, got %d", len(got))
	}
}

// TestMatchesModel pins the --model filter semantics directly: empty filter
// matches everything; otherwise a case-insensitive substring of the model.
func TestMatchesModel(t *testing.T) {
	tests := []struct {
		model, filter string
		want          bool
	}{
		{"gpt-5.3-codex", "gpt", true},
		{"gpt-5.3-codex", "GPT-5", true},
		{"gpt-5.3-codex", "CODEX", true},
		{"", "gpt", false},
		{"", "", true},
		{"claude-sonnet-4-5", "sonnet", true},
		{"claude-sonnet-4-5", "opus", false},
	}
	for _, tt := range tests {
		if got := matchesModel(tt.model, tt.filter); got != tt.want {
			t.Errorf("matchesModel(%q, %q) = %v, want %v", tt.model, tt.filter, got, tt.want)
		}
	}
}

// TestCollectUsageIterEarlyStop pins that an iterator error aborts the scan
// and surfaces (same propagation contract as CollectSessions).
func TestCollectUsageIterEarlyStop(t *testing.T) {
	sentinel := errors.New("stop")
	a := &usageFake{metaFake: metaFake{fakeAdapter: fakeAdapter{name: "claude-code"}}, usage: sampleUsage()}
	_, err := CollectUsage([]Adapter{a}, SessionFilter{}, "", nil), a.SessionsUsage(func(SessionUsage) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Errorf("SessionsUsage should propagate iter error, got %v", err)
	}
}
