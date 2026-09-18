package search

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wrinfotel/pastlog/internal/adapters/claudecode"
	"github.com/wrinfotel/pastlog/internal/adapters/codex"
	"github.com/wrinfotel/pastlog/internal/adapters/geminicli"
	"github.com/wrinfotel/pastlog/internal/agentlog"
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
	metaErr   error // returned by SessionsMeta/Sessions (listing failure)
	entryErr  error // returned by Entries/EntriesFiltered (scan failure)
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
	return f.metaErr
}

func (f *fakeAdapter) Sessions(iter func(agentlog.Session) error) error {
	for _, m := range f.metas {
		if err := iter(m.Session); err != nil {
			return err
		}
	}
	return f.metaErr
}

func (f *fakeAdapter) Entries(s agentlog.Session, iter func(agentlog.Entry) error) error {
	f.usedPlain = true
	f.scanned = append(f.scanned, s.ID)
	if f.entryErr != nil {
		return f.entryErr
	}
	for _, e := range f.entries[s.ID] {
		if err := iter(e); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeAdapter) EntriesFiltered(s agentlog.Session, keep func([]byte) bool, iter func(agentlog.Entry) error) error {
	f.scanned = append(f.scanned, s.ID)
	if f.entryErr != nil {
		return f.entryErr
	}
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

// TestRunNotesFailingAdapter pins the M4-B2 plumbing: a listing failure and
// a scan failure each produce exactly one note naming the adapter, while
// hits from healthy adapters are unaffected.
func TestRunNotesFailingAdapter(t *testing.T) {
	brokenList := &fakeAdapter{name: "broken-list", metaErr: errors.New("permission denied")}
	brokenScan := &fakeAdapter{
		name:     "broken-scan",
		metas:    []agentlog.SessionMeta{{Session: agentlog.Session{ID: "s1", StartedAt: tNew}}},
		entryErr: errors.New("read error"),
	}
	healthy := &fakeAdapter{
		name: "claude-code",
		metas: []agentlog.SessionMeta{
			{Session: agentlog.Session{ID: "a1", StartedAt: tOld}, Messages: 1},
		},
		entries: map[string][]agentlog.Entry{
			"a1": {{Kind: agentlog.Message, Role: "user", Text: "the jwt secret"}},
		},
	}

	var notes []string
	results := Run([]agentlog.Adapter{brokenList, brokenScan, healthy},
		mustMatcher(t, "jwt", MatchOptions{}),
		EngineOptions{Note: func(n string) { notes = append(notes, n) }})
	if len(results) != 1 || results[0].Session.ID != "a1" {
		t.Fatalf("healthy adapter's hits must survive, got %+v", results)
	}
	if len(notes) != 2 {
		t.Fatalf("got %d notes (%v), want 2 — one per failing adapter", len(notes), notes)
	}
	if !strings.Contains(notes[0], "broken-list: storage unreadable") ||
		!strings.Contains(notes[1], "broken-scan: storage unreadable") {
		t.Errorf("notes should name each failing adapter, got %v", notes)
	}
}

// TestRunMaxHitsCapIsNotAStorageFailure pins the M6 DoD finding: when a scan
// stops because the total-hits cap is reached, the engine's errStop sentinel
// must not surface as an "unreadable storage" note — the cap is normal,
// expected behavior, and the results are complete.
func TestRunMaxHitsCapIsNotAStorageFailure(t *testing.T) {
	entries := make([]agentlog.Entry, 0, 10)
	for i := 0; i < 10; i++ {
		entries = append(entries, agentlog.Entry{Kind: agentlog.Message, Role: "user", Text: "needle in the log"})
	}
	big := &fakeAdapter{
		name:  "big",
		metas: []agentlog.SessionMeta{{Session: agentlog.Session{ID: "s1", StartedAt: tNew}}},
		entries: map[string][]agentlog.Entry{
			"s1": entries,
		},
	}

	var notes []string
	results := Run([]agentlog.Adapter{big},
		mustMatcher(t, "needle", MatchOptions{}),
		EngineOptions{MaxHits: 3, Note: func(n string) { notes = append(notes, n) }})
	if len(results) != 1 || len(results[0].Hits) != 3 {
		t.Fatalf("cap must stop the scan with exactly MaxHits hits, got %+v", results)
	}
	if len(notes) != 0 {
		t.Errorf("reaching the hit cap must not emit a note, got %v", notes)
	}
}

// TestRunFastListingHitsMatchFullListing pins the fix-round regression
// guarantee end to end over real adapters: switching the search flow to the
// fast metadata listing (first record line + stat) changes nothing about the
// results — same sessions, same order, same hits with identical highlight
// offsets — including under filters.
func TestRunFastListingHitsMatchFullListing(t *testing.T) {
	home := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// line-1-complete claude file, a fallback claude file (summary first) and
	// a codex rollout with a meta-first line — covering both listing modes
	write(filepath.Join(".claude", "projects", "C--Users-dev-app", "61616161-6161-4161-8161-616161616161.jsonl"),
		`{"type":"user","sessionId":"61616161-6161-4161-8161-616161616161","cwd":"/home/dev/app","timestamp":"2026-08-02T14:00:00Z","message":{"role":"user","content":"the zephyr valve sticks"}}`+"\n"+
			`{"type":"summary","summary":"zephyr valve review","sessionId":"61616161-6161-4161-8161-616161616161"}`+"\n"+
			`{"type":"user","sessionId":"61616161-6161-4161-8161-616161616161","cwd":"/home/dev/app","timestamp":"2026-08-02T14:01:00Z","message":{"role":"user","content":"zephyr valve replaced"}}`+"\n")
	write(filepath.Join(".claude", "projects", "C--Users-dev-app", "62626262-6262-4262-8262-626262626262.jsonl"),
		`{"type":"summary","summary":"older notes","sessionId":"62626262-6262-4262-8262-626262626262"}`+"\n"+
			`{"type":"user","sessionId":"62626262-6262-4262-8262-626262626262","cwd":"/home/dev/app","timestamp":"2026-07-01T09:00:00Z","message":{"role":"user","content":"zephyr mentioned here too"}}`+"\n")
	write(filepath.Join(".codex", "sessions", "2026", "08", "02", "rollout-2026-08-02T15-00-00-63636363-6363-4363-8363-636363636363.jsonl"),
		`{"timestamp":"2026-08-02T15:00:00Z","type":"session_meta","payload":{"id":"63636363-6363-4363-8363-636363636363","cwd":"/home/dev/api"}}`+"\n"+
			`{"timestamp":"2026-08-02T15:00:10Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"zephyr calibration drifted"}]}}`+"\n")
	// gemini-cli sessions over the same flow: one metadata-first file and one
	// whose metadata record carries a sessionId but no startTime — the fast
	// path cannot know the start timestamp the full parse backfills from the
	// records, so that file must drop to the full parse (FastMetaSource
	// filters/sort parity; final review finding 2)
	write(filepath.Join(".gemini", "tmp", "ca11bad5gme", "chats", "session-2026-08-02T16-00-64646464.jsonl"),
		`{"sessionId":"64646464-6464-4644-8464-646464646464","startTime":"2026-08-02T16:00:00Z","lastUpdated":"2026-08-02T16:01:00Z","kind":"main","directories":["/home/dev/app"],"summary":"zephyr notes"}`+"\n"+
			`{"id":"m1","timestamp":"2026-08-02T16:00:30Z","type":"user","content":"zephyr sealant spec reviewed"}`+"\n")
	write(filepath.Join(".gemini", "tmp", "ca11bad5gme", "chats", "session-2026-08-02T16-30-65656565.jsonl"),
		`{"sessionId":"65656565-6565-4655-8565-656565656565","directories":["/home/dev/api"]}`+"\n"+
			`{"id":"m1","timestamp":"2026-08-02T16:30:00Z","type":"user","content":"zephyr valve torque spec"}`+"\n")

	adapters := []agentlog.Adapter{claudecode.New(home), codex.New(home), geminicli.New(home)}
	run := func(fast bool, filter agentlog.SessionFilter) []string {
		t.Helper()
		results := Run(adapters, mustMatcher(t, "zephyr", MatchOptions{}), EngineOptions{Filter: filter, FastListing: fast})
		var out []string
		for _, r := range results {
			for _, h := range r.Hits {
				out = append(out, fmt.Sprintf("%s|%s|%d|%d|%s", r.Session.ID, h.Line, h.MatchStart, h.MatchEnd, h.Context))
			}
		}
		return out
	}

	filters := []agentlog.SessionFilter{
		{},
		{Project: "app"},
		{Since: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
	}
	for i, f := range filters {
		full, fast := run(false, f), run(true, f)
		if strings.Join(full, "\n") != strings.Join(fast, "\n") {
			t.Errorf("filter #%d: fast listing changed the results:\nfull:\n%s\nfast:\n%s", i, strings.Join(full, "\n"), strings.Join(fast, "\n"))
		}
	}
	if n := len(run(false, agentlog.SessionFilter{})); n != 7 {
		t.Errorf("fixture should produce 7 hits, got %d", n)
	}
}

type fastFakeAdapter struct {
	fakeAdapter
	fastErr error
}

func (f *fastFakeAdapter) SessionsMetaFast(iter func(agentlog.SessionMeta) error) (bool, error) {
	return true, f.fastErr
}

// TestRunNotesFailingFastAdapter pins that a failing fast listing is noted
// exactly like a failing full listing.
func TestRunNotesFailingFastAdapter(t *testing.T) {
	broken := &fastFakeAdapter{fakeAdapter: fakeAdapter{name: "broken"}, fastErr: errors.New("permission denied")}
	healthy := &fakeAdapter{
		name: "claude-code",
		metas: []agentlog.SessionMeta{
			{Session: agentlog.Session{ID: "a1", StartedAt: tOld}, Messages: 1},
		},
		entries: map[string][]agentlog.Entry{
			"a1": {{Kind: agentlog.Message, Role: "user", Text: "the jwt secret"}},
		},
	}
	var notes []string
	results := Run([]agentlog.Adapter{broken, healthy}, mustMatcher(t, "jwt", MatchOptions{}),
		EngineOptions{FastListing: true, Note: func(n string) { notes = append(notes, n) }})
	if len(results) != 1 || results[0].Session.ID != "a1" {
		t.Fatalf("healthy adapter's hits must survive, got %+v", results)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "broken: storage unreadable") {
		t.Errorf("fast listing failure should be noted once, got %v", notes)
	}
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
