package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrinfotel/pastlog/internal/adapters/codex"
	"github.com/wrinfotel/pastlog/internal/adapters/geminicli"
	"github.com/wrinfotel/pastlog/internal/adapters/opencode"
	"github.com/wrinfotel/pastlog/internal/adapters/zcode"
)

// SPEC-context-analysis §6.6: the golden session rendered in all five native
// formats must yield identical findings and advice — agent differences show
// up only as the profile header (id, agent name) and the precision note,
// never as a different rule set. Every extractor reports exact tokens on
// this fixture, so even the byte counts inside the finding texts must match.

const (
	parityCodexID    = "dddd4444-4444-4444-4444-dddddddddddd"
	parityGeminiID   = "eeee6666-6666-6666-6666-eeeeeeeeeeee"
	parityOpencodeID = "ses_ctxparity0000000000000000a1"
	parityZcodeID    = "sess_ctxparity0000000000000000a1"
)

// parityHome builds a fixture home holding the golden session for all five
// agents (the claude-code fixture comes from context_test.go).
func parityHome(t *testing.T) string {
	t.Helper()
	isolateDataHome(t)
	home := t.TempDir()

	write := func(rel, content string) {
		p := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write(".claude/projects/-home-dev-app/cccc3333-3333-4333-8333-cccccccccccc.jsonl", ctxFixtureContent())
	write(".codex/sessions/2026/08/03/rollout-2026-08-03T10-00-00-"+parityCodexID+".jsonl",
		codex.GenerateContextFixture(parityCodexID))
	write(".gemini/tmp/projhash/chats/session-"+parityGeminiID+".jsonl",
		geminicli.GenerateContextFixture(parityGeminiID))

	// opencode resolves its storage root through the XDG/LOCALAPPDATA
	// overrides isolateDataHome set; generate at every candidate so the
	// OS-specific priority order cannot miss the fixture
	for _, env := range []string{"XDG_DATA_HOME", "LOCALAPPDATA"} {
		if data := os.Getenv(env); data != "" {
			if _, err := opencode.GenerateContextFixtureDB(filepath.Join(data, "opencode"), parityOpencodeID); err != nil {
				t.Fatal(err)
			}
		}
	}
	// zcode resolves under the home directly
	if _, err := zcode.GenerateContextFixtureDB(filepath.Join(home, ".zcode", "cli", "db"), parityZcodeID); err != nil {
		t.Fatal(err)
	}
	return home
}

// findingsSection extracts everything between the findings header and the
// precision note: findings, compactions, advice — the agent-independent part
// of the report.
func findingsSection(out string) string {
	var b strings.Builder
	on := false
	for _, ln := range strings.Split(out, "\n") {
		if strings.HasPrefix(ln, "findings:") {
			on = true
		}
		if strings.HasPrefix(ln, "precision:") {
			on = false
		}
		if on {
			b.WriteString(ln)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func TestContextParityAcrossAgents(t *testing.T) {
	home := parityHome(t)
	cases := []struct{ prefix, agent string }{
		{"cccc3333", "claude-code"},
		{"dddd4444", "codex"},
		{"eeee6666", "gemini-cli"},
		{"ses_ctxparity", "opencode"},
		{"sess_ctxparity", "zcode"},
	}
	want := ""
	for _, tc := range cases {
		code, out, errOut := run(t, "--home", home, "context", tc.prefix)
		if code != 0 {
			t.Fatalf("%s: context exit = %d, stderr: %s", tc.agent, code, errOut)
		}
		if got := findingsSection(out); want == "" {
			want = got
		} else if got != want {
			t.Errorf("%s findings diverge from claude-code:\n--- claude-code ---\n%s\n--- %s ---\n%s",
				tc.agent, want, tc.agent, got)
		}
		if !strings.Contains(out, "precision: exact tokens ("+tc.agent+")") {
			t.Errorf("%s: report should claim exact tokens:\n%s", tc.agent, out)
		}
	}
	// the shared findings section must exercise every rule
	for _, rule := range []string{"R1", "R2", "R3", "R4", "R5"} {
		if !strings.Contains(want, rule) {
			t.Errorf("parity findings missing %s:\n%s", rule, want)
		}
	}
	if !strings.Contains(want, "compactions: 1") {
		t.Errorf("parity findings missing the compaction summary:\n%s", want)
	}
	t.Logf("shared findings section:\n%s", want)
}
