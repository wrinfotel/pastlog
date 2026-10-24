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

// TestContextEventsNewRolloutFormat pins the context flow against the current
// codex rollout shape: TurnStart events come from event_msg token_count
// envelopes (the top-level token_count record is gone), custom_tool_call /
// custom_tool_call_output map to ToolCall / ToolResult via call_id, and
// token_usage_record lines emit NOTHING here (the per-turn usage they repeat
// is already the token_count envelope's last_token_usage — emitting both
// would double every turn).
func TestContextEventsNewRolloutFormat(t *testing.T) {
	home := t.TempDir()
	content := strings.Join([]string{
		`{"timestamp":"2026-10-24T10:00:00Z","ordinal":1,"type":"session_meta","payload":{"id":"5e6f7071-5555-4555-8555-555555555555","cwd":"/home/dev/app"}}`,
		`{"timestamp":"2026-10-24T10:00:01Z","ordinal":2,"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1200,"cached_input_tokens":300,"output_tokens":100},"last_token_usage":{"input_tokens":1200,"cached_input_tokens":300,"output_tokens":100}}}}`,
		`{"timestamp":"2026-10-24T10:00:02Z","ordinal":3,"type":"response_item","payload":{"type":"custom_tool_call","call_id":"call_1","name":"exec","input":"const r = await tools.exec_command({cmd:\"cat main.go\"});\ntext(r.output);\n"}}`,
		`{"timestamp":"2026-10-24T10:00:03Z","ordinal":4,"type":"token_usage_record","payload":{"turn_token_usage":{"input_tokens":1200,"cached_input_tokens":300,"output_tokens":100},"thread_token_usage":{"input_tokens":1200,"cached_input_tokens":300,"output_tokens":100}}}`,
		`{"timestamp":"2026-10-24T10:00:04Z","ordinal":5,"type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"call_1","output":[{"type":"input_text","text":"package main\nfunc main() {}\n"}]}}`,
		`{"timestamp":"2026-10-24T10:00:05Z","ordinal":6,"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":2600,"cached_input_tokens":1600,"output_tokens":160},"last_token_usage":{"input_tokens":1400,"cached_input_tokens":1300,"output_tokens":60}}}}`,
		`{"timestamp":"2026-10-24T10:00:06Z","ordinal":7,"type":"event_msg","payload":{"type":"task_complete","last_agent_message":"done"}}`,
		`{"timestamp":"2026-10-24T10:00:07Z","ordinal":8,"type":"response_item","payload":{"type":"compaction","id":"cmp_1","encrypted_content":"gAAAA-expired"}}`,
	}, "\n") + "\n"
	p := filepath.Join(home, ".codex", "sessions", "2026", "10", "24", "rollout-2026-10-24T10-00-00-5e6f7071.jsonl")
	if err := writeFileAll(p, content); err != nil {
		t.Fatal(err)
	}
	events := ctxEvents(t, home, "5e6f7071-5555-4555-8555-555555555555")

	var turns []int64
	var calls, results, compacts int
	for _, ev := range events {
		switch ev.Kind {
		case agentlog.CtxTurnStart:
			if ev.TokensKind != agentlog.CtxTokensExact {
				t.Errorf("turn tokens not exact: %+v", ev.Tokens)
			}
			turns = append(turns, ev.Tokens.Sum())
		case agentlog.CtxToolCall:
			calls++
			if ev.Tool != "exec" {
				t.Errorf("tool call tool = %q, want exec", ev.Tool)
			}
			if ev.ArgsKey == "" || ev.ArgsKey == "exec\x00{}" {
				t.Errorf("ArgsKey should carry the call input, got %q", ev.ArgsKey)
			}
		case agentlog.CtxToolResult:
			results++
			if ev.Tool != "exec" {
				t.Errorf("tool result not correlated via call_id, tool = %q", ev.Tool)
			}
			if ev.Head != "package main" {
				t.Errorf("result head = %q, want %q", ev.Head, "package main")
			}
		case agentlog.CtxCompact:
			compacts++
		}
	}
	if len(turns) != 2 || turns[0] != 1500 || turns[1] != 2700 {
		t.Errorf("window sums = %v, want [1500 2700] (token_usage_record must not add turns)", turns)
	}
	if calls != 1 || results != 1 {
		t.Errorf("custom tool calls=%d results=%d, want 1/1", calls, results)
	}
	if compacts != 1 {
		t.Errorf("compactions = %d, want 1 (the response_item compaction payload is the new boundary spelling)", compacts)
	}
}

func TestContextEventsUnknownSession(t *testing.T) {
	a := New(t.TempDir())
	if _, err := a.ContextEvents(agentlog.Session{ID: "nope"}); err == nil {
		t.Errorf("unknown session should error")
	}
}
