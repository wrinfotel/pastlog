package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAntigravityCLI(t *testing.T) {
	isolateDataHome(t)
	home := t.TempDir()
	const parent = "aabb1111-1111-4111-8111-111111111111"
	const child = "aabb2222-2222-4222-8222-222222222222"
	root := filepath.Join(home, ".gemini", "antigravity-cli")
	write := func(path, value string) {
		t.Helper()
		path = filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	logs := "brain/" + parent + "/.system_generated/logs/"
	write(logs+"transcript.jsonl", `{"step_index":0,"type":"USER_INPUT","content":"short transcript only"}`)
	write(logs+"transcript_full.jsonl", `{"step_index":0,"type":"USER_INPUT","created_at":"2026-06-28T09:54:31Z","content":"<USER_REQUEST>\ncheck the result\n</USER_REQUEST>"}
{"step_index":1,"type":"PLANNER_RESPONSE","created_at":"2026-06-28T09:54:32Z","content":"Acknowledged","tool_calls":[{"name":"run_command","args":{"CommandLine":"echo hello"}}]}
{"step_index":2,"type":"RUN_COMMAND","status":"DONE","created_at":"2026-06-28T09:54:33Z","content":"unique tool output"}
{"step_index":3,"type":"INVOKE_SUBAGENT","status":"DONE","content":"Created:\n{\"conversationId\":\"`+child+`\"}"}`)
	write("brain/"+child+"/.system_generated/logs/transcript.jsonl", `{"step_index":0,"type":"PLANNER_RESPONSE","created_at":"2026-06-28T09:54:32Z","content":"child reply"}`)
	write("history.jsonl", `{"conversationId":"`+parent+`","display":"Check result","workspace":"C:/project","timestamp":1782640471000,"type":"user"}`)
	for _, tc := range []struct {
		args, includes []string
	}{
		{[]string{"sessions", "--agent", "antigravity", "--json"}, []string{parent, child, `"project": "C:/project"`}},
		{[]string{"show", parent, "--json"}, []string{`"text": "check the result"`, `"text": "unique tool output"`}},
		{[]string{"search", "unique tool output", "--agent", "antigravity", "--json"}, []string{parent, "unique tool output"}},
		{[]string{"related", child, "--json"}, []string{`"parent": {`, parent}},
		{[]string{"stats", "--agent", "antigravity", "--json"}, []string{`"key": "antigravity"`, `"sessions": 2`}},
		{[]string{"context", parent}, []string{"antigravity"}},
	} {
		code, out, errOut := run(t, append([]string{"--home", home}, tc.args...)...)
		if code != 0 || errOut != "" {
			t.Errorf("%v exit=%d stderr=%s", tc.args, code, errOut)
		}
		for _, want := range tc.includes {
			if !strings.Contains(out, want) {
				t.Errorf("%v missing %q: %s", tc.args, want, out)
			}
		}
		if strings.Contains(out, "short transcript only") {
			t.Errorf("%v used the short transcript despite the full one", tc.args)
		}
	}
}
