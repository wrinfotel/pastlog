package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// ctxFixtureID is the context fixture session's id (distinct from the
// listing fixtures; the CLI parity test resolves it by prefix).
const ctxFixtureID = "dddd4444-4444-4444-4444-dddddddddddd"

// writeFileAll creates parent directories and writes the file.
func writeFileAll(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// writeCtxFixture writes the golden context session (see GenerateContextFixture
// in gen.go) under <home>/.codex and returns the home path.
func writeCtxFixture(t *testing.T, home string) string {
	t.Helper()
	p := filepath.Join(home, ".codex", "sessions", "2026", "08", "03", "rollout-2026-08-03T10-00-00-"+ctxFixtureID+".jsonl")
	if err := writeFileAll(p, GenerateContextFixture(ctxFixtureID)); err != nil {
		t.Fatal(err)
	}
	return home
}

// ctxEvents runs ContextEvents over the fixture session.
func ctxEvents(t *testing.T, home, id string) []agentlog.CtxEvent {
	t.Helper()
	a := New(home)
	events, err := a.ContextEvents(agentlog.Session{ID: id, Agent: agentName})
	if err != nil {
		t.Fatalf("ContextEvents: %v", err)
	}
	return events
}

func TestContextEventsGolden(t *testing.T) {
	events := ctxEvents(t, writeCtxFixture(t, t.TempDir()), ctxFixtureID)

	var turns, results, compacts int
	var lastSum int64
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
				t.Errorf("tool result without a linked call: %+v", ev)
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

	// per-turn window sums mirror the claude-code fixture
	var sums []int64
	var errFlags int
	for _, ev := range events {
		switch ev.Kind {
		case agentlog.CtxTurnStart:
			sums = append(sums, ev.Tokens.Sum())
		case agentlog.CtxToolResult:
			if ev.Err {
				errFlags++
			}
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
	// t2, t4, t5, t6 are regex-flagged failures; the Read results are not
	if errFlags != 4 {
		t.Errorf("error-shaped results = %d, want 4", errFlags)
	}

	// the Read on main.go repeats with a turn between (R2's raw material)
	var readKey string
	readCalls := 0
	for _, ev := range events {
		if ev.Kind == agentlog.CtxToolCall && ev.Tool == "Read" {
			if readKey == "" {
				readKey = ev.ArgsKey
			}
			if ev.ArgsKey == readKey {
				readCalls++
			}
		}
	}
	if readCalls != 2 {
		t.Errorf("Read(main.go) calls = %d, want 2", readCalls)
	}
}

func TestContextEventsCumulativeFallback(t *testing.T) {
	// a rollout whose token_count records carry only cumulative totals: the
	// per-turn windows are the diffs (SPEC §6.2)
	home := t.TempDir()
	content := strings.Join([]string{
		`{"timestamp":"2026-08-03T10:00:00Z","type":"session_meta","payload":{"id":"eeee5555-5555-5555-5555-eeeeeeeeeeee","cwd":"/home/dev/app"}}`,
		`{"timestamp":"2026-08-03T10:00:05Z","type":"token_count","payload":{"info":{"total_token_usage":{"input_tokens":1000,"cached_input_tokens":0,"output_tokens":50}}}}`,
		`{"timestamp":"2026-08-03T10:00:10Z","type":"token_count","payload":{"info":{"total_token_usage":{"input_tokens":2400,"cached_input_tokens":600,"output_tokens":110}}}}`,
	}, "\n") + "\n"
	p := filepath.Join(home, ".codex", "sessions", "2026", "08", "03", "rollout-2026-08-03T10-00-00-eeee.jsonl")
	if err := writeFileAll(p, content); err != nil {
		t.Fatal(err)
	}
	events := ctxEvents(t, home, "eeee5555-5555-5555-5555-eeeeeeeeeeee")

	var sums []int64
	for _, ev := range events {
		if ev.Kind == agentlog.CtxTurnStart {
			sums = append(sums, ev.Tokens.Sum())
		}
	}
	if len(sums) != 2 || sums[0] != 1000 || sums[1] != 2000 {
		t.Errorf("cumulative-diff windows = %v, want [1000 2000]", sums)
	}
}

func TestContextEventsUnknownSession(t *testing.T) {
	a := New(t.TempDir())
	if _, err := a.ContextEvents(agentlog.Session{ID: "nope"}); err == nil {
		t.Errorf("unknown session should error")
	}
}
