package search

import (
	"testing"
	"time"

	"github.com/pastlog/pastlog/internal/adapters/claudecode"
	"github.com/pastlog/pastlog/internal/adapters/codex"
	"github.com/pastlog/pastlog/internal/agentlog"
)

var (
	tOld = time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	tMid = time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	tNew = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
)

// fakeAdapter simulates a line-oriented adapter and records how the engine
// drives it: how many raw lines the prefilter inspected, how many were
// parsed, which sessions were scanned and whether the plain Entries path was
// used.
type fakeAdapter struct {
	name      string
	metas     []agentlog.SessionMeta
	entries   map[string][]agentlog.Entry
	keepCalls int
	parsed    int
	scanned   []string
	usedPlain bool
}

func (f *fakeAdapter) Name() string      { return f.name }
func (f *fakeAdapter) Detect() bool      { return true }
func (f *fakeAdapter) SkippedLines() int { return 0 }

func (f *fakeAdapter) SessionsMeta(iter func(agentlog.SessionMeta) error) error {
	for _, m := range f.metas {
		if err := iter(m); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeAdapter) Sessions(iter func(agentlog.Session) error) error {
	for _, m := range f.metas {
		if err := iter(m.Session); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeAdapter) Entries(s agentlog.Session, iter func(agentlog.Entry) error) error {
	f.usedPlain = true
	f.scanned = append(f.scanned, s.ID)
	for _, e := range f.entries[s.ID] {
		if err := iter(e); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeAdapter) EntriesFiltered(s agentlog.Session, keep func([]byte) bool, iter func(agentlog.Entry) error) error {
	f.scanned = append(f.scanned, s.ID)
	for _, e := range f.entries[s.ID] {
		line := []byte("raw:" + e.Text)
		f.keepCalls++
		if keep != nil && !keep(line) {
			continue // prefilter rejected: never parsed
		}
		f.parsed++
		if err := iter(e); err != nil {
			return err
		}
	}
	return nil
}

func TestRealAdaptersImplementLineFiltered(t *testing.T) {
	home := t.TempDir()
	var _ LineFilteredAdapter = claudecode.New(home)
	var _ LineFilteredAdapter = codex.New(home)
}

func TestRunFindsHitsAcrossAdaptersNewestFirst(t *testing.T) {
	a := &fakeAdapter{
		name: "claude-code",
		metas: []agentlog.SessionMeta{
			{Session: agentlog.Session{ID: "a1", StartedAt: tOld}, Messages: 1},
		},
		entries: map[string][]agentlog.Entry{
			"a1": {{Kind: agentlog.Message, Role: "user", Text: "the jwt secret"}},
		},
	}
	b := &fakeAdapter{
		name: "codex",
		metas: []agentlog.SessionMeta{
			{Session: agentlog.Session{ID: "c1", StartedAt: tNew}, Messages: 2},
		},
		entries: map[string][]agentlog.Entry{
			"c1": {{Kind: agentlog.Message, Role: "user", Text: "jwt payload here"}},
		},
	}
	results := Run([]agentlog.Adapter{a, b}, mustMatcher(t, "jwt", MatchOptions{}), EngineOptions{})
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Session.ID != "c1" || results[1].Session.ID != "a1" {
		t.Errorf("results out of order: %s then %s", results[0].Session.ID, results[1].Session.ID)
	}
	if results[0].Session.Messages != 2 {
		t.Errorf("Messages = %d, want 2 (carried from SessionMeta)", results[0].Session.Messages)
	}
	if results[0].Session.Agent != "codex" {
		t.Errorf("Agent = %q, want codex (set by the engine)", results[0].Session.Agent)
	}
	if got := results[0].Hits[0].Line; got != "jwt payload here" {
		t.Errorf("hit line = %q", got)
	}
}

func TestRunDropsSessionsWithoutHits(t *testing.T) {
	a := &fakeAdapter{
		name: "claude-code",
		metas: []agentlog.SessionMeta{
			{Session: agentlog.Session{ID: "quiet", StartedAt: tNew}},
			{Session: agentlog.Session{ID: "noisy", StartedAt: tOld}},
		},
		entries: map[string][]agentlog.Entry{
			"quiet": {{Kind: agentlog.Message, Text: "nothing here"}},
			"noisy": {{Kind: agentlog.Message, Text: "jwt found"}},
		},
	}
	results := Run([]agentlog.Adapter{a}, mustMatcher(t, "jwt", MatchOptions{}), EngineOptions{})
	if len(results) != 1 || results[0].Session.ID != "noisy" {
		t.Fatalf("got %+v, want one result for noisy", results)
	}
}

func TestRunAgentFilter(t *testing.T) {
	a := &fakeAdapter{
		name:  "claude-code",
		metas: []agentlog.SessionMeta{{Session: agentlog.Session{ID: "a1", StartedAt: tNew}}},
		entries: map[string][]agentlog.Entry{
			"a1": {{Kind: agentlog.Message, Text: "jwt in claude"}},
		},
	}
	b := &fakeAdapter{
		name:  "codex",
		metas: []agentlog.SessionMeta{{Session: agentlog.Session{ID: "c1", StartedAt: tOld}}},
		entries: map[string][]agentlog.Entry{
			"c1": {{Kind: agentlog.Message, Text: "jwt in codex"}},
		},
	}
	results := Run([]agentlog.Adapter{a, b}, mustMatcher(t, "jwt", MatchOptions{}),
		EngineOptions{Filter: agentlog.SessionFilter{Agent: "codex"}})
	if len(results) != 1 || results[0].Session.ID != "c1" {
		t.Fatalf("agent filter failed: %+v", results)
	}
	if a.scanned != nil {
		t.Error("other adapters should not be scanned at all")
	}
}

func TestRunMaxHitsCapsTotalHits(t *testing.T) {
	a := &fakeAdapter{
		name: "claude-code",
		metas: []agentlog.SessionMeta{
			{Session: agentlog.Session{ID: "s1", StartedAt: tNew}},
			{Session: agentlog.Session{ID: "s2", StartedAt: tMid}},
			{Session: agentlog.Session{ID: "s3", StartedAt: tOld}},
		},
		entries: map[string][]agentlog.Entry{
			"s1": {{Kind: agentlog.Message, Text: "jwt one"}},
			"s2": {{Kind: agentlog.Message, Text: "jwt two"}},
			"s3": {{Kind: agentlog.Message, Text: "jwt three"}},
		},
	}
	results := Run([]agentlog.Adapter{a}, mustMatcher(t, "jwt", MatchOptions{}), EngineOptions{MaxHits: 2})
	total := 0
	for _, r := range results {
		total += len(r.Hits)
	}
	if total != 2 {
		t.Errorf("total hits = %d, want exactly 2", total)
	}
	if len(a.scanned) != 2 {
		t.Errorf("scanned sessions = %v, want 2 (scan stops at the cap)", a.scanned)
	}
}

func TestRunLimitCapsSessionsScanned(t *testing.T) {
	a := &fakeAdapter{
		name: "claude-code",
		metas: []agentlog.SessionMeta{
			{Session: agentlog.Session{ID: "s1", StartedAt: tNew}},
			{Session: agentlog.Session{ID: "s2", StartedAt: tMid}},
			{Session: agentlog.Session{ID: "s3", StartedAt: tOld}},
		},
		entries: map[string][]agentlog.Entry{
			"s1": {{Kind: agentlog.Message, Text: "jwt one"}},
			"s2": {{Kind: agentlog.Message, Text: "jwt two"}},
			"s3": {{Kind: agentlog.Message, Text: "jwt three"}},
		},
	}
	results := Run([]agentlog.Adapter{a}, mustMatcher(t, "jwt", MatchOptions{}), EngineOptions{Sessions: 2})
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if len(a.scanned) != 2 {
		t.Errorf("scanned = %v, want only the two newest sessions", a.scanned)
	}
}

func TestRunNoHits(t *testing.T) {
	a := &fakeAdapter{
		name:  "claude-code",
		metas: []agentlog.SessionMeta{{Session: agentlog.Session{ID: "s1", StartedAt: tNew}}},
		entries: map[string][]agentlog.Entry{
			"s1": {{Kind: agentlog.Message, Text: "nothing relevant"}},
		},
	}
	results := Run([]agentlog.Adapter{a}, mustMatcher(t, "jwt", MatchOptions{}), EngineOptions{})
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

// TestRunPrefilterSkipsParsing proves the engine drives the raw-line
// prefilter: non-matching lines are rejected before parsing.
func TestRunPrefilterSkipsParsing(t *testing.T) {
	a := &fakeAdapter{
		name:  "claude-code",
		metas: []agentlog.SessionMeta{{Session: agentlog.Session{ID: "s1", StartedAt: tNew}}},
		entries: map[string][]agentlog.Entry{
			"s1": {
				{Kind: agentlog.Message, Text: "has needle inside"},
				{Kind: agentlog.Message, Text: "nothing to see"},
				{Kind: agentlog.Message, Text: "still nothing"},
			},
		},
	}
	results := Run([]agentlog.Adapter{a}, mustMatcher(t, "needle", MatchOptions{}), EngineOptions{})
	if len(results) != 1 || len(results[0].Hits) != 1 {
		t.Fatalf("hits = %+v, want 1", results)
	}
	if a.keepCalls != 3 {
		t.Errorf("keep calls = %d, want 3 (one per raw line)", a.keepCalls)
	}
	if a.parsed != 1 {
		t.Errorf("parsed lines = %d, want 1 (prefilter rejected the rest)", a.parsed)
	}
	if a.usedPlain {
		t.Error("engine should use EntriesFiltered when the prefilter is on")
	}
}

func TestRunPlainEntriesWhenPrefilterDisabled(t *testing.T) {
	a := &fakeAdapter{
		name:  "claude-code",
		metas: []agentlog.SessionMeta{{Session: agentlog.Session{ID: "s1", StartedAt: tNew}}},
		entries: map[string][]agentlog.Entry{
			"s1": {{Kind: agentlog.Message, Text: `say "needle"`}},
		},
	}
	results := Run([]agentlog.Adapter{a}, mustMatcher(t, `say "needle"`, MatchOptions{}), EngineOptions{})
	if len(results) != 1 {
		t.Fatalf("quoted needle should still match, got %+v", results)
	}
	if !a.usedPlain {
		t.Error("prefilter disabled: engine must fall back to plain Entries")
	}
}

// TestRunContextTracksPreviousEntry uses regex mode (no prefilter): the
// context of a hit is the entry immediately before it in stream order.
func TestRunContextTracksPreviousEntry(t *testing.T) {
	a := &fakeAdapter{
		name:  "claude-code",
		metas: []agentlog.SessionMeta{{Session: agentlog.Session{ID: "s1", StartedAt: tNew}}},
		entries: map[string][]agentlog.Entry{
			"s1": {
				{Kind: agentlog.Message, Text: "hit one X"},
				{Kind: agentlog.Message, Text: "middle"},
				{Kind: agentlog.Message, Text: "hit two X"},
			},
		},
	}
	results := Run([]agentlog.Adapter{a}, mustMatcher(t, "X", MatchOptions{Regex: true}), EngineOptions{})
	if len(results) != 1 || len(results[0].Hits) != 2 {
		t.Fatalf("hits = %+v, want 2", results)
	}
	if c := results[0].Hits[0].Context; c != "" {
		t.Errorf("first hit context = %q, want empty", c)
	}
	if c := results[0].Hits[1].Context; c != "middle" {
		t.Errorf("second hit context = %q, want the previous entry text", c)
	}
}

// TestRunContextIsPreviousSurvivingEntry documents the prefilter trade-off:
// with the raw-line prefilter on, non-candidate lines are never parsed, so
// the context of a hit is the previous entry that survived the prefilter.
func TestRunContextIsPreviousSurvivingEntry(t *testing.T) {
	a := &fakeAdapter{
		name:  "claude-code",
		metas: []agentlog.SessionMeta{{Session: agentlog.Session{ID: "s1", StartedAt: tNew}}},
		entries: map[string][]agentlog.Entry{
			"s1": {
				{Kind: agentlog.Message, Text: "hit one X"},
				{Kind: agentlog.Message, Text: "middle"}, // rejected by the prefilter
				{Kind: agentlog.Message, Text: "hit two X"},
			},
		},
	}
	results := Run([]agentlog.Adapter{a}, mustMatcher(t, "X", MatchOptions{}), EngineOptions{})
	if len(results) != 1 || len(results[0].Hits) != 2 {
		t.Fatalf("hits = %+v, want 2", results)
	}
	if c := results[0].Hits[1].Context; c != "hit one X" {
		t.Errorf("second hit context = %q, want the previous surviving entry", c)
	}
}
