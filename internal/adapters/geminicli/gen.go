package geminicli

import "fmt"

// GenerateContextFixture returns the SPEC §6.6 golden session as one
// session file's JSONL content: exact per-turn usage, a repeated read (R2),
// an oversized tool result (R1), an error loop (R3), a compaction boundary
// (R4), and a big-turn jump (R5). The per-turn window sums mirror the
// claude-code fixture in internal/cli/context_test.go (1500, 2700, 3100,
// 60000, 62000, 64000, 18000, 19000; gemini has no cache-write field, so
// each sum is Input alone) so the cross-agent parity test in internal/cli
// can assert identical findings for all five agents.
func GenerateContextFixture(sessionID string) string {
	var b []byte
	line := func(format string, args ...any) {
		b = append(b, fmt.Appendf(nil, format, args...)...)
		b = append(b, '\n')
	}
	line(`{"sessionId":%q,"startTime":"2026-08-03T10:00:00Z","lastUpdated":"2026-08-03T10:00:51Z","summary":"Debug flaky build","directories":["/home/dev/app"]}`, sessionID)
	line(`{"type":"user","timestamp":"2026-08-03T10:00:01Z","content":"build is failing, look into it"}`)
	// turn 1: window 1500; Read on main.go
	line(`{"type":"gemini","timestamp":"2026-08-03T10:00:05Z","content":[{"type":"text","text":"checking the build first"}],"toolCalls":[{"id":"t1","name":"Read","args":{"file_path":"/home/dev/app/main.go"},"result":%q,"status":"executed"}],"tokens":{"input":1500,"output":100,"cached":0,"thoughts":0},"model":"fixture-gemini"}`, "package main\nfunc main() {}\n")
	// turn 2: window 2700; first test failure (no status: regex fallback)
	line(`{"type":"gemini","timestamp":"2026-08-03T10:00:10Z","toolCalls":[{"id":"t2","name":"Bash","args":{"command":"go test ./..."},"result":%q}],"tokens":{"input":2700,"output":60,"cached":0,"thoughts":0},"model":"fixture-gemini"}`, ctxFailText)
	// turn 3: window 3100; giant build log (R1)
	line(`{"type":"gemini","timestamp":"2026-08-03T10:00:20Z","toolCalls":[{"id":"t3","name":"Bash","args":{"command":"go build -v ./... 2>&1 | tee /tmp/build.log; cat /tmp/build.log"},"result":%q,"status":"executed"}],"tokens":{"input":3100,"output":50,"cached":0,"thoughts":0},"model":"fixture-gemini"}`, ctxGiantLog)
	// turns 4-6: windows 60000/62000/64000; three identical failures (R3, R5)
	line(`{"type":"gemini","timestamp":"2026-08-03T10:00:30Z","toolCalls":[{"id":"t4","name":"Bash","args":{"command":"go test ./..."},"result":%q,"status":"error"}],"tokens":{"input":60000,"output":40,"cached":0,"thoughts":0},"model":"fixture-gemini"}`, ctxFailText)
	line(`{"type":"gemini","timestamp":"2026-08-03T10:00:35Z","toolCalls":[{"id":"t5","name":"Bash","args":{"command":"go test ./..."},"result":%q,"status":"error"}],"tokens":{"input":62000,"output":40,"cached":0,"thoughts":0},"model":"fixture-gemini"}`, ctxFailText)
	line(`{"type":"gemini","timestamp":"2026-08-03T10:00:38Z","toolCalls":[{"id":"t6","name":"Bash","args":{"command":"go test ./..."},"result":%q,"status":"error"}],"tokens":{"input":64000,"output":30,"cached":0,"thoughts":0},"model":"fixture-gemini"}`, ctxFailText)
	// compaction checkpoint, then usage drops (R4)
	line(`{"type":"user","timestamp":"2026-08-03T10:00:40Z","$set":{"messages":[{"type":"user","content":"(snapshot)"}]}}`)
	line(`{"type":"gemini","timestamp":"2026-08-03T10:00:45Z","content":[{"type":"text","text":"context compacted; continuing with the fix"}],"tokens":{"input":18000,"output":30,"cached":0,"thoughts":0},"model":"fixture-gemini"}`)
	// re-read of main.go after edits (R2: turn gap between t1 and t9)
	line(`{"type":"gemini","timestamp":"2026-08-03T10:00:50Z","toolCalls":[{"id":"t9","name":"Read","args":{"file_path":"/home/dev/app/main.go"},"result":%q,"status":"executed"}],"tokens":{"input":19000,"output":40,"cached":0,"thoughts":0},"model":"fixture-gemini"}`, "package main\nfunc main() { config := Config() }\n")
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
