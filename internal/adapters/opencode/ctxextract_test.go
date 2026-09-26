package opencode

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/wrinfotel/pastlog/internal/agentlog"

	_ "modernc.org/sqlite"
)

// ctxSessionID is the context fixture session's id (distinct from the
// listing fixtures; the CLI parity test resolves it by prefix).
const ctxSessionID = "ses_ctxfixture0000000000000000a1"

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
}

func TestContextEventsScalarTokens(t *testing.T) {
	// fixture-era step-finish shapes carried a scalar: it maps to Input
	dbPath := writeCtxFixtureDB(t, t.TempDir())

	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(fmt.Sprintf(
		`INSERT INTO message (id, session_id, time_created, time_updated, data)
		 VALUES ('msg_ctx10', '%s', 1786221100000, 1786221100000, '{"role":"assistant"}')`, ctxSessionID)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data)
		 VALUES ('prt_ctx10', 'msg_ctx10', '` + ctxSessionID + `', 1786220996000, 1786220996000, '{"type":"step-finish","tokens":42}')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	events := ctxEvents(t, dbPath, ctxSessionID)
	var last agentlog.CtxEvent
	for _, ev := range events {
		if ev.Kind == agentlog.CtxTurnStart {
			last = ev
		}
	}
	if last.Tokens.Sum() != 42 {
		t.Errorf("scalar tokens → Sum = %d, want 42", last.Tokens.Sum())
	}
}

func TestContextEventsUnknownSession(t *testing.T) {
	a := NewDir(t.TempDir())
	if _, err := a.ContextEvents(agentlog.Session{ID: "ses_nope"}); err == nil {
		t.Errorf("unknown session should error")
	}
}
