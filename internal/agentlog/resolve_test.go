package agentlog

import (
	"errors"
	"testing"
	"time"
)

// fakeAdapter drives ResolveSession without touching real storage.
type resolveAdapter struct {
	name      string
	sessions  []Session
	listError error
}

func (f *resolveAdapter) Name() string                                 { return f.name }
func (f *resolveAdapter) Detect() bool                                 { return true }
func (f *resolveAdapter) Entries(_ Session, _ func(Entry) error) error { return nil }

func (f *resolveAdapter) Sessions(iter func(Session) error) error {
	if f.listError != nil {
		return f.listError
	}
	for _, s := range f.sessions {
		if err := iter(s); err != nil {
			return err
		}
	}
	return nil
}

func resolveSession(id, project string, start time.Time) Session {
	return Session{ID: id, Agent: "test", Project: project, StartedAt: start, SizeBytes: 10}
}

func TestResolveSessionExactID(t *testing.T) {
	a := &resolveAdapter{name: "a", sessions: []Session{
		resolveSession("aaaaaaaa-1111", "/p1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		resolveSession("bbbbbbbb-2222", "/p2", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)),
	}}
	res, err := ResolveSession([]Adapter{a}, "bbbbbbbb-2222")
	if err != nil {
		t.Fatalf("exact id rejected: %v", err)
	}
	if res.Meta.ID != "bbbbbbbb-2222" {
		t.Errorf("resolved %q, want bbbbbbbb-2222", res.Meta.ID)
	}
	if res.Adapter != a {
		t.Error("resolution lost the owning adapter")
	}
	if len(res.Candidates) != 0 || len(res.Unreadable) != 0 {
		t.Errorf("unexpected extras: candidates=%d unreadable=%d", len(res.Candidates), len(res.Unreadable))
	}
}

func TestResolveSessionUniquePrefix(t *testing.T) {
	a := &resolveAdapter{name: "a", sessions: []Session{
		resolveSession("aaaa1111-xxxx", "/p1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		resolveSession("bbbb2222-yyyy", "/p2", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)),
	}}
	res, err := ResolveSession([]Adapter{a}, "aaaa")
	if err != nil {
		t.Fatalf("unique prefix rejected: %v", err)
	}
	if res.Meta.ID != "aaaa1111-xxxx" {
		t.Errorf("resolved %q, want aaaa1111-xxxx", res.Meta.ID)
	}
}

func TestResolveSessionAmbiguousListsCandidatesNewestFirst(t *testing.T) {
	a := &resolveAdapter{name: "a", sessions: []Session{
		resolveSession("cccc3333-old", "/old", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		resolveSession("cccc4444-new", "/new", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)),
		resolveSession("cccc5555-tie", "/tie", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)),
	}}
	res, err := ResolveSession([]Adapter{a}, "cccc")
	if err != nil {
		t.Fatalf("ambiguous prefix must resolve to candidates, got error: %v", err)
	}
	if res.Found() {
		t.Error("ambiguous prefix must not report a found session")
	}
	var got []string
	for _, c := range res.Candidates {
		got = append(got, c.ID)
	}
	want := []string{"cccc4444-new", "cccc5555-tie", "cccc3333-old"} // newest first, tie by ID ascending
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate order = %v, want %v", got, want)
		}
	}
}

func TestResolveSessionNoMatch(t *testing.T) {
	a := &resolveAdapter{name: "a", sessions: []Session{resolveSession("aaaaaaaa-1", "/p", time.Time{})}}
	res, err := ResolveSession([]Adapter{a}, "zzzz")
	if err == nil {
		t.Fatal("no-match must error")
	}
	if want := `no session matches id prefix "zzzz"`; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if res.Found() || len(res.Candidates) != 0 {
		t.Errorf("unexpected resolution state: found=%v candidates=%d", res.Found(), len(res.Candidates))
	}
}

func TestResolveSessionNoMatchNotesUnreadable(t *testing.T) {
	boom := errors.New("database is locked")
	a := &resolveAdapter{name: "a", listError: boom}
	res, err := ResolveSession([]Adapter{a}, "zzzz")
	if err == nil {
		t.Fatal("no-match must error even with unreadable storage")
	}
	want := []string{"a: " + boom.Error()}
	if len(res.Unreadable) != 1 || res.Unreadable[0] != want[0] {
		t.Errorf("unreadable = %v, want %v", res.Unreadable, want)
	}
}

func TestResolveSessionListsAcrossAdapters(t *testing.T) {
	old := &resolveAdapter{name: "old", sessions: []Session{
		resolveSession("cccc6666-from-old", "/p", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
	}}
	newer := &resolveAdapter{name: "newer", sessions: []Session{
		resolveSession("cccc7777-from-newer", "/p", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)),
	}}
	res, err := ResolveSession([]Adapter{old, newer}, "cccc")
	if err != nil {
		t.Fatalf("cross-adapter ambiguity rejected: %v", err)
	}
	if len(res.Candidates) != 2 || res.Candidates[0].ID != "cccc7777-from-newer" {
		t.Errorf("candidates = %v, want the newer adapter's session first", res.Candidates)
	}
	for _, c := range res.Candidates {
		if c.Agent == "" {
			t.Error("candidates must carry the owning adapter name")
		}
	}
}
