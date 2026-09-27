package codex

import "fmt"

// GenerateContextFixture returns the SPEC §6.6 golden session as one
// rollout's JSONL content: exact per-turn usage, a repeated read (R2), an
// oversized tool result (R1), an error loop (R3), a compaction boundary
// (R4), and a big-turn jump (R5). The per-turn window sums mirror the
// claude-code fixture in internal/cli/context_test.go (1500, 2700, 3100,
// 60000, 62000, 64000, 18000, 19000) so the cross-agent parity test in
// internal/cli can assert identical findings for all five agents.
func GenerateContextFixture(sessionID string) string {
	var b []byte
	line := func(format string, args ...any) {
		b = append(b, fmt.Appendf(nil, format, args...)...)
		b = append(b, '\n')
	}
	line(`{"timestamp":"2026-08-03T10:00:00Z","type":"session_meta","payload":{"id":%q,"cwd":"/home/dev/app"}}`, sessionID)
	line(`{"timestamp":"2026-08-03T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"build is failing, look into it"}]}}`)
	// turn 1: window 1500; Read on main.go
	line(`{"timestamp":"2026-08-03T10:00:05Z","type":"token_count","payload":{"info":{"total_token_usage":{"input_tokens":1200,"cached_input_tokens":300,"output_tokens":100},"last_token_usage":{"input_tokens":1200,"cached_input_tokens":300,"output_tokens":100}}}}`)
	line(`{"timestamp":"2026-08-03T10:00:05Z","type":"response_item","payload":{"type":"function_call","name":"Read","arguments":%q,"call_id":"t1"}}`, `{"file_path":"/home/dev/app/main.go"}`)
	line(`{"timestamp":"2026-08-03T10:00:06Z","type":"response_item","payload":{"type":"function_call_output","call_id":"t1","output":%q}}`, "package main\nfunc main() {}\n")
	// turn 2: window 2700; first test failure
	line(`{"timestamp":"2026-08-03T10:00:10Z","type":"token_count","payload":{"info":{"total_token_usage":{"input_tokens":2600,"cached_input_tokens":1600,"output_tokens":160},"last_token_usage":{"input_tokens":1400,"cached_input_tokens":1300,"output_tokens":60}}}}`)
	line(`{"timestamp":"2026-08-03T10:00:10Z","type":"response_item","payload":{"type":"function_call","name":"Bash","arguments":%q,"call_id":"t2"}}`, `{"command":"go test ./..."}`)
	line(`{"timestamp":"2026-08-03T10:00:12Z","type":"response_item","payload":{"type":"function_call_output","call_id":"t2","output":%q}}`, ctxFailText)
	// turn 3: window 3100; giant build log (R1)
	line(`{"timestamp":"2026-08-03T10:00:20Z","type":"token_count","payload":{"info":{"total_token_usage":{"input_tokens":4200,"cached_input_tokens":3100,"output_tokens":210},"last_token_usage":{"input_tokens":1600,"cached_input_tokens":1500,"output_tokens":50}}}}`)
	line(`{"timestamp":"2026-08-03T10:00:20Z","type":"response_item","payload":{"type":"function_call","name":"Bash","arguments":%q,"call_id":"t3"}}`, `{"command":"go build -v ./... 2>&1 | tee /tmp/build.log; cat /tmp/build.log"}`)
	line(`{"timestamp":"2026-08-03T10:00:24Z","type":"response_item","payload":{"type":"function_call_output","call_id":"t3","output":%q}}`, ctxGiantLog)
	// turns 4-6: windows 60000/62000/64000; three identical failures (R3, R5)
	line(`{"timestamp":"2026-08-03T10:00:30Z","type":"token_count","payload":{"info":{"total_token_usage":{"input_tokens":33000,"cached_input_tokens":33100,"output_tokens":250},"last_token_usage":{"input_tokens":30000,"cached_input_tokens":30000,"output_tokens":40}}}}`)
	line(`{"timestamp":"2026-08-03T10:00:30Z","type":"response_item","payload":{"type":"function_call","name":"Bash","arguments":%q,"call_id":"t4"}}`, `{"command":"go test ./..."}`)
	line(`{"timestamp":"2026-08-03T10:00:31Z","type":"response_item","payload":{"type":"function_call_output","call_id":"t4","output":%q}}`, ctxFailText)
	line(`{"timestamp":"2026-08-03T10:00:35Z","type":"token_count","payload":{"info":{"total_token_usage":{"input_tokens":64000,"cached_input_tokens":64100,"output_tokens":290},"last_token_usage":{"input_tokens":31000,"cached_input_tokens":31000,"output_tokens":40}}}}`)
	line(`{"timestamp":"2026-08-03T10:00:35Z","type":"response_item","payload":{"type":"function_call","name":"Bash","arguments":%q,"call_id":"t5"}}`, `{"command":"go test ./..."}`)
	line(`{"timestamp":"2026-08-03T10:00:36Z","type":"response_item","payload":{"type":"function_call_output","call_id":"t5","output":%q}}`, ctxFailText)
	line(`{"timestamp":"2026-08-03T10:00:38Z","type":"token_count","payload":{"info":{"total_token_usage":{"input_tokens":96000,"cached_input_tokens":96100,"output_tokens":320},"last_token_usage":{"input_tokens":32000,"cached_input_tokens":32000,"output_tokens":30}}}}`)
	line(`{"timestamp":"2026-08-03T10:00:38Z","type":"response_item","payload":{"type":"function_call","name":"Bash","arguments":%q,"call_id":"t6"}}`, `{"command":"go test ./..."}`)
	line(`{"timestamp":"2026-08-03T10:00:39Z","type":"response_item","payload":{"type":"function_call_output","call_id":"t6","output":%q}}`, ctxFailText)
	// compaction boundary, then usage drops (R4)
	line(`{"timestamp":"2026-08-03T10:00:40Z","type":"compacted","payload":{"message":"context compacted"}}`)
	line(`{"timestamp":"2026-08-03T10:00:45Z","type":"token_count","payload":{"info":{"total_token_usage":{"input_tokens":105000,"cached_input_tokens":96100,"output_tokens":350},"last_token_usage":{"input_tokens":9000,"cached_input_tokens":9000,"output_tokens":30}}}}`)
	// re-read of main.go after edits (R2: turn gap between t1 and t9)
	line(`{"timestamp":"2026-08-03T10:00:50Z","type":"token_count","payload":{"info":{"total_token_usage":{"input_tokens":114500,"cached_input_tokens":105100,"output_tokens":390},"last_token_usage":{"input_tokens":9500,"cached_input_tokens":9500,"output_tokens":40}}}}`)
	line(`{"timestamp":"2026-08-03T10:00:50Z","type":"response_item","payload":{"type":"function_call","name":"Read","arguments":%q,"call_id":"t9"}}`, `{"file_path":"/home/dev/app/main.go"}`)
	line(`{"timestamp":"2026-08-03T10:00:51Z","type":"response_item","payload":{"type":"function_call_output","call_id":"t9","output":%q}}`, "package main\nfunc main() { config := Config() }\n")
	return string(b)
}

// ctxGiantLog and ctxFailText are the golden session's shared payloads.
var (
	ctxGiantLog = repeatCtxLine("building package github.com/dev/app/internal ...\n", 400)
	ctxFailText = "FAIL github.com/dev/app 0.5s\nbuild failed: undefined: Config"
)

func repeatCtxLine(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
