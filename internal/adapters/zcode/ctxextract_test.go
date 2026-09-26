package zcode

import (
	"path/filepath"
	"testing"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// ctxSessionID is the context fixture session's id (distinct from the
// listing fixtures; the CLI parity test resolves it by prefix).
const ctxSessionID = "sess_ctxfixture0000000000000000a1"

// writeCtxFixtureDB creates the golden context session (see
// GenerateContextFixtureDB in gen.go) under a test directory.
func writeCtxFixtureDB(t *testing.T, dir string) string {
	t.Helper()
	path, err := GenerateContextFixtureDB(dir, ctxSessionID)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func ctxEvents(t *testing.T, dbPath, id string) []agentlog.CtxEvent {
	t.Helper()
	a := NewDir(filepath.Dir(dbPath))
	events, err := a.ContextEvents(agentlog.Session{ID: id, Agent: agentName})
	if err != nil {
		t.Fatalf("ContextEvents: %v", err)
	}
	return events
}

func TestContextEventsGolden(t *testing.T) {
	dbPath := writeCtxFixtureDB(t, t.TempDir())
	events := ctxEvents(t, dbPath, ctxSessionID)

	var turns, results, compacts int
	var lastSum int64
	var errShaped int
	for _, ev := range events {
		switch ev.Kind {
		case agentlog.CtxTurnStart:
			turns++
			if ev.TokensKind != agentlog.CtxTokensExact {
				t.Errorf("turn %d: tokens not exact", turns)
			}
			lastSum = ev.Tokens.Sum()
		case agentlog.CtxToolResult:
			results++
			if ev.Tool == "" {
				t.Errorf("tool result without a tool name: %+v", ev)
			}
			if ev.Err {
				errShaped++
			}
		case agentlog.CtxCompact:
			compacts++
		}
	}
	if turns != 8 {
		t.Errorf("turns = %d, want 8", turns)
	}
	if results != 7 {
		t.Errorf("tool results = %d, want 7", results)
	}
	if compacts != 1 {
		t.Errorf("compactions = %d, want 1", compacts)
	}
	if lastSum != 19000 {
		t.Errorf("final window = %d, want 19000", lastSum)
	}
	// t4-t6 are status-flagged errors; t2's "completed" status is trusted
	if errShaped != 3 {
		t.Errorf("error-shaped results = %d, want 3", errShaped)
	}

	// per-turn window sums mirror the claude-code fixture
	var sums []int64
	for _, ev := range events {
		if ev.Kind == agentlog.CtxTurnStart {
			sums = append(sums, ev.Tokens.Sum())
		}
	}
	wantSums := []int64{1500, 2700, 3100, 60000, 62000, 64000, 18000, 19000}
	if len(sums) != len(wantSums) {
		t.Fatalf("window sums = %v, want %v", sums, wantSums)
	}
	for i := range wantSums {
		if sums[i] != wantSums[i] {
			t.Errorf("window sum %d = %d, want %d", i, sums[i], wantSums[i])
		}
	}

	// merge ordering: a request's TurnStart precedes the parts it produced
	firstCall, firstTurn := -1, -1
	for i, ev := range events {
		if firstCall < 0 && ev.Kind == agentlog.CtxToolCall {
			firstCall = i
		}
		if firstTurn < 0 && ev.Kind == agentlog.CtxTurnStart {
			firstTurn = i
		}
	}
	if firstTurn < 0 || firstCall < 0 || firstTurn > firstCall {
		t.Errorf("TurnStart (idx %d) must precede the first tool call (idx %d)", firstTurn, firstCall)
	}
}

func TestContextEventsUnknownSession(t *testing.T) {
	a := NewDir(t.TempDir())
	if _, err := a.ContextEvents(agentlog.Session{ID: "sess_nope"}); err == nil {
		t.Errorf("unknown session should error")
	}
}
