package geminicli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// writeCtxFile writes content under <home>/.gemini/<rel> (creating dirs).
func writeCtxFile(t *testing.T, home, rel, content string) string {
	t.Helper()
	p := filepath.Join(home, ".gemini", "tmp", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

// ctxEvents runs ContextEvents over a session id.
func ctxEvents(t *testing.T, home, id string) []agentlog.CtxEvent {
	t.Helper()
	a := New(home)
	events, err := a.ContextEvents(agentlog.Session{ID: id, Agent: agentName})
	if err != nil {
		t.Fatalf("ContextEvents: %v", err)
	}
	return events
}

// ctxFixtureID is the context fixture session's id (the metadata sessionId
// and the file name agree; the CLI parity test resolves it by prefix).
const ctxFixtureID = "eeee6666-6666-6666-6666-eeeeeeeeeeee"

// writeCtxFixture writes the golden context session (see
// GenerateContextFixture in gen.go) under <home>/.gemini and returns the
// home path.
func writeCtxFixture(t *testing.T, home string) string {
	t.Helper()
	return writeCtxFile(t, home, "projhash/chats/session-"+ctxFixtureID+".jsonl", GenerateContextFixture(ctxFixtureID))
}

func TestContextEventsGolden(t *testing.T) {
	home := writeCtxFixture(t, t.TempDir())
	events := ctxEvents(t, home, ctxFixtureID)

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
	// t4-t6 status-flagged errors; t2's status is absent and the regex
	// fallback flags it (4 error-shaped results overall)
	if errShaped != 4 {
		t.Errorf("error-shaped results = %d, want 4", errShaped)
	}

	// per-turn window sums
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

func TestContextEventsLegacy(t *testing.T) {
	// the same golden session inside a monolithic chats.json
	var b strings.Builder
	b.WriteString(`{"sessions":[`)
	fmt.Fprintf(&b, `{"sessionId":%q,"startTime":"2026-08-03T10:00:00Z","directories":["/home/dev/app"],`, ctxLegacyID)
	b.WriteString(`"messages":[`)
	b.WriteString(`{"type":"user","timestamp":"2026-08-03T10:00:01Z","content":"build is failing, look into it"},`)
	b.WriteString(`{"type":"gemini","timestamp":"2026-08-03T10:00:05Z","toolCalls":[{"name":"Read","args":{"file_path":"/home/dev/app/main.go"},"result":"package main\nfunc main() {}\n","status":"executed"}],"tokens":{"input":1500,"output":100}}`)
	b.WriteString(`]}`)
	b.WriteString(`]}`)
	home := writeCtxFile(t, t.TempDir(), "projhash/chats.json", b.String())

	events := ctxEvents(t, home, ctxLegacyID)
	var turns, calls int
	for _, ev := range events {
		switch ev.Kind {
		case agentlog.CtxTurnStart:
			turns++
		case agentlog.CtxToolCall:
			calls++
		}
	}
	if turns != 1 || calls != 1 {
		t.Errorf("legacy events: turns=%d calls=%d, want 1/1", turns, calls)
	}
}

const ctxLegacyID = "eeee7777-7777-7777-7777-eeeeeeeeeeee"

func TestContextEventsUnknownSession(t *testing.T) {
	a := New(t.TempDir())
	if _, err := a.ContextEvents(agentlog.Session{ID: "nope"}); err == nil {
		t.Errorf("unknown session should error")
	}
}
