package agentlog

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

var (
	t1 = time.Date(2026, 8, 2, 14, 3, 0, 0, time.UTC)
	t2 = time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	t3 = time.Date(2026, 9, 1, 20, 30, 0, 0, time.UTC)
)

// fakeAdapter is a controllable Adapter for tests (no MetaSource).
type fakeAdapter struct {
	name     string
	detect   bool
	sessions []Session
	entries  map[string][]Entry
	scanErr  error
}

// metaFake adds SessionsMeta support on top of fakeAdapter.
type metaFake struct {
	fakeAdapter
	metas []SessionMeta // if non-nil, returned by SessionsMeta
}

func (f *metaFake) SessionsMeta(iter func(SessionMeta) error) error {
	for _, m := range f.metas {
		if err := iter(m); err != nil {
			return err
		}
	}
	return f.scanErr
}

func (f *fakeAdapter) Name() string { return f.name }
func (f *fakeAdapter) Detect() bool { return f.detect }
func (f *fakeAdapter) Sessions(iter func(Session) error) error {
	for _, s := range f.sessions {
		if err := iter(s); err != nil {
			return err
		}
	}
	return f.scanErr
}

func (f *fakeAdapter) Entries(s Session, iter func(Entry) error) error {
	for _, e := range f.entries[s.ID] {
		if err := iter(e); err != nil {
			return err
		}
	}
	return nil
}

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	if _, ok := reg.Get("claude-code"); ok {
		t.Fatal("empty registry should not find adapters")
	}

	a := &fakeAdapter{name: "claude-code"}
	b := &fakeAdapter{name: "codex"}
	reg.Register(a)
	reg.Register(b)

	if got, ok := reg.Get("codex"); !ok || got.Name() != "codex" {
		t.Errorf("Get(codex) = %v, %v; want codex adapter", got.Name(), ok)
	}
	if _, ok := reg.Get("gemini-cli"); ok {
		t.Error("Get(gemini-cli) should miss")
	}
	got := reg.Adapters()
	if len(got) != 2 || got[0].Name() != "claude-code" || got[1].Name() != "codex" {
		t.Errorf("Adapters() = %v, want [claude-code codex]", names(got))
	}
}

func TestRegistryRegisterReplacesSameName(t *testing.T) {
	reg := NewRegistry()
	replacement := &fakeAdapter{name: "claude-code"}
	reg.Register(&fakeAdapter{name: "claude-code"})
	reg.Register(replacement)

	got := reg.Adapters()
	if len(got) != 1 || got[0] != Adapter(replacement) {
		t.Errorf("duplicate register should replace, got %d adapters", len(got))
	}
}

func names(as []Adapter) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.Name()
	}
	return out
}

func TestCollectSessionsSortsNewestFirst(t *testing.T) {
	a := &fakeAdapter{name: "claude-code", sessions: []Session{
		{ID: "b", Agent: "claude-code", StartedAt: t1},
		{ID: "c", Agent: "claude-code", StartedAt: t3},
		{ID: "a", Agent: "claude-code", StartedAt: t1},
	}}
	got := CollectSessions([]Adapter{a}, SessionFilter{}, nil)
	want := []string{"c", "a", "b"}
	if len(got) != len(want) {
		t.Fatalf("got %d sessions, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("session[%d].ID = %q, want %q (newest first, ties by ID)", i, got[i].ID, id)
		}
	}
}

func TestCollectSessionsFilters(t *testing.T) {
	sessions := []Session{
		{ID: "claude-old", Agent: "claude-code", Project: "/home/dev/myapp", StartedAt: t1},
		{ID: "claude-new", Agent: "claude-code", Project: "/home/dev/other", StartedAt: t3},
		{ID: "codex", Agent: "codex", Project: "/home/dev/myapp", StartedAt: t2},
	}
	a := &fakeAdapter{name: "claude-code", sessions: sessions[:2]}
	b := &fakeAdapter{name: "codex", sessions: sessions[2:]}

	tests := []struct {
		name   string
		filter SessionFilter
		want   []string
	}{
		{"no filter", SessionFilter{}, []string{"claude-new", "codex", "claude-old"}},
		{"agent", SessionFilter{Agent: "claude-code"}, []string{"claude-new", "claude-old"}},
		{"project substring case-insensitive", SessionFilter{Project: "MYAPP"}, []string{"codex", "claude-old"}},
		{"since inclusive", SessionFilter{Since: t2}, []string{"claude-new", "codex"}},
		{"until inclusive", SessionFilter{Until: t2}, []string{"codex", "claude-old"}},
		{"since+until window", SessionFilter{Since: t1, Until: t2}, []string{"codex", "claude-old"}},
		{"limit", SessionFilter{Limit: 2}, []string{"claude-new", "codex"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CollectSessions([]Adapter{a, b}, tt.filter, nil)
			ids := make([]string, len(got))
			for i, m := range got {
				ids[i] = m.ID
			}
			if fmt.Sprint(ids) != fmt.Sprint(tt.want) {
				t.Errorf("got %v, want %v", ids, tt.want)
			}
		})
	}
}

// TestProjectFilterNormalizesSeparators pins the Windows path-separator
// normalization (M4): `--project dev/myapp` must hit a session whose recorded
// cwd uses backslashes, and vice versa. Matching stays case-insensitive.
func TestProjectFilterNormalizesSeparators(t *testing.T) {
	sessions := []Session{
		{ID: "win", Project: `C:\Users\dev\myapp`, StartedAt: t1},
		{ID: "unix", Project: "/home/dev/elsewhere", StartedAt: t2},
	}
	a := &fakeAdapter{name: "claude-code", sessions: sessions}

	tests := []struct{ filter, want string }{
		{`dev/myapp`, "win"},       // needle with slashes, cwd with backslashes
		{`dev\elsewhere`, "unix"},  // needle with backslashes, cwd with slashes
		{`users\dev\MYAPP`, "win"}, // mixed separators and case
		{"myapp", "win"},           // plain substring still works
	}
	for _, tt := range tests {
		got := CollectSessions([]Adapter{a}, SessionFilter{Project: tt.filter}, nil)
		if len(got) != 1 || got[0].ID != tt.want {
			t.Errorf("project filter %q = %v, want [%s]", tt.filter, ids(got), tt.want)
		}
	}
}

func ids(metas []SessionMeta) []string {
	out := make([]string, len(metas))
	for i, m := range metas {
		out[i] = m.ID
	}
	return out
}

func TestCollectSessionsUsesAdapterMeta(t *testing.T) {
	a := &metaFake{fakeAdapter: fakeAdapter{name: "claude-code"}, metas: []SessionMeta{
		{Session: Session{ID: "x", StartedAt: t1}, Messages: 7},
	}}
	got := CollectSessions([]Adapter{a}, SessionFilter{}, nil)
	if len(got) != 1 || got[0].Messages != 7 {
		t.Errorf("meta messages = %+v, want 7", got)
	}
}

func TestCollectSessionsFallbackCountsMessageEntries(t *testing.T) {
	a := &fakeAdapter{
		name:     "claude-code",
		sessions: []Session{{ID: "x", StartedAt: t1}},
		entries: map[string][]Entry{
			"x": {
				{Kind: Message, Role: "user"},
				{Kind: ToolCall, Role: "assistant"},
				{Kind: Message, Role: "assistant"},
				{Kind: ToolResult, Role: "tool"},
				{Kind: Summary},
				{Kind: Message, Role: "user"},
			},
		},
	}
	got := CollectSessions([]Adapter{a}, SessionFilter{}, nil)
	if len(got) != 1 || got[0].Messages != 3 {
		t.Errorf("fallback message count = %d, want 3", got[0].Messages)
	}
}

func TestCollectSessionsStopsOnAdapterError(t *testing.T) {
	a := &fakeAdapter{
		name:    "claude-code",
		scanErr: errors.New("disk gone"),
	}
	got := CollectSessions([]Adapter{a}, SessionFilter{}, nil)
	if len(got) != 0 {
		t.Errorf("expected no sessions on error, got %d", len(got))
	}
}

func TestAgentFilterSkipsOtherAdaptersWithoutScanning(t *testing.T) {
	a := &fakeAdapter{name: "claude-code", sessions: []Session{{ID: "x", StartedAt: t1}}}
	b := &fakeAdapter{name: "codex", sessions: []Session{{ID: "y", StartedAt: t2}}}
	got := CollectSessions([]Adapter{a, b}, SessionFilter{Agent: "codex"}, nil)
	if len(got) != 1 || got[0].ID != "y" {
		t.Errorf("agent filter should select only codex, got %v", got)
	}
}

// TestCollectSessionsNotesUnreadableStorage pins the M4-B1 plumbing: a
// failing adapter produces exactly one note naming the adapter and the
// cause, while healthy adapters' sessions are unaffected.
func TestCollectSessionsNotesUnreadableStorage(t *testing.T) {
	healthy := &metaFake{fakeAdapter: fakeAdapter{name: "codex"}, metas: []SessionMeta{
		{Session: Session{ID: "c", StartedAt: t1}, Messages: 2},
	}}
	broken := &metaFake{fakeAdapter: fakeAdapter{name: "claude-code", scanErr: errors.New("permission denied")}}

	var notes []string
	got := CollectSessions([]Adapter{broken, healthy}, SessionFilter{}, func(n string) { notes = append(notes, n) })
	if len(got) != 1 || got[0].ID != "c" {
		t.Errorf("healthy adapter's sessions must survive, got %v", ids(got))
	}
	if len(notes) != 1 {
		t.Fatalf("got %d notes, want 1: %v", len(notes), notes)
	}
	if !strings.Contains(notes[0], "claude-code: storage unreadable") ||
		!strings.Contains(notes[0], "permission denied") {
		t.Errorf("note should name the adapter and the cause, got %q", notes[0])
	}
	if notes[0][:1] == strings.ToUpper(notes[0][:1]) {
		t.Errorf("note should be lowercase, got %q", notes[0])
	}
}

// TestCollectSessionsNoteNilIsLegal pins that a nil note callback is allowed
// (best-effort listing stays silent when nobody listens).
func TestCollectSessionsNoteNilIsLegal(t *testing.T) {
	broken := &metaFake{fakeAdapter: fakeAdapter{name: "claude-code", scanErr: errors.New("permission denied")}}
	got := CollectSessions([]Adapter{broken}, SessionFilter{}, nil)
	if len(got) != 0 {
		t.Errorf("expected no sessions, got %v", ids(got))
	}
}

func TestTotalSkipped(t *testing.T) {
	a := &fakeAdapter{name: "claude-code"}
	b := &skippingAdapter{skipped: 12}
	if got := TotalSkipped([]Adapter{a, b}); got != 12 {
		t.Errorf("TotalSkipped = %d, want 12", got)
	}
}

type skippingAdapter struct {
	fakeAdapter
	skipped int
}

func (s *skippingAdapter) SkippedLines() int { return s.skipped }
