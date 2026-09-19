package search

import (
	"testing"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// twoMatchingSessions builds an adapter whose two sessions each contain one
// matching entry, so progress and cancellation have something to chew on.
func twoMatchingSessions() *fakeAdapter {
	return &fakeAdapter{
		name: "a",
		metas: []agentlog.SessionMeta{
			{Session: agentlog.Session{ID: "s1", StartedAt: tOld}, Messages: 1},
			{Session: agentlog.Session{ID: "s2", StartedAt: tNew}, Messages: 1},
		},
		entries: map[string][]agentlog.Entry{
			"s1": {{Kind: agentlog.Message, Role: "user", Text: "alpha hit one"}},
			"s2": {{Kind: agentlog.Message, Role: "user", Text: "alpha hit two"}},
		},
	}
}

func TestRunProgressCalledPerScannedSession(t *testing.T) {
	a := twoMatchingSessions()
	var scanned, hits []int
	results := Run([]agentlog.Adapter{a}, mustMatcher(t, "alpha", MatchOptions{}),
		EngineOptions{Progress: func(done, h int) bool {
			scanned = append(scanned, done)
			hits = append(hits, h)
			return true
		}})
	if len(results) != 2 {
		t.Fatalf("both sessions must be scanned, got %d results", len(results))
	}
	if len(scanned) != 2 || scanned[0] != 1 || scanned[1] != 2 {
		t.Errorf("scanned ticks = %v, want [1 2]", scanned)
	}
	if len(hits) != 2 || hits[0] != 1 || hits[1] != 2 {
		t.Errorf("hits ticks = %v, want [1 2]", hits)
	}
}

func TestRunProgressCancelStopsTheScan(t *testing.T) {
	a := twoMatchingSessions()
	results := Run([]agentlog.Adapter{a}, mustMatcher(t, "alpha", MatchOptions{}),
		EngineOptions{Progress: func(done, _ int) bool { return done < 1 }})
	if len(results) != 1 || results[0].Session.ID != "s2" {
		t.Fatalf("cancel after the first session must keep only s2 (newest first), got %+v", results)
	}
	if len(results[0].Hits) != 1 {
		t.Errorf("s2's hit must survive, got %d hits", len(results[0].Hits))
	}
}

func TestRunProgressCancelAlwaysKeepsFirstScanned(t *testing.T) {
	// Post-scan semantics: the first tick happens after the first session,
	// so an always-cancelling hook still keeps that session's hits.
	a := twoMatchingSessions()
	results := Run([]agentlog.Adapter{a}, mustMatcher(t, "alpha", MatchOptions{}),
		EngineOptions{Progress: func(_, _ int) bool { return false }})
	if len(results) != 1 || results[0].Session.ID != "s2" {
		t.Fatalf("always-cancel must keep exactly the first scanned session (s2, newest first), got %+v", results)
	}
}

func TestRunProgressNotCalledWithoutSessions(t *testing.T) {
	empty := &fakeAdapter{name: "empty"}
	called := false
	Run([]agentlog.Adapter{empty}, mustMatcher(t, "alpha", MatchOptions{}),
		EngineOptions{Progress: func(_, _ int) bool { called = true; return true }})
	if called {
		t.Error("the hook must not fire when there is nothing to scan")
	}
}
