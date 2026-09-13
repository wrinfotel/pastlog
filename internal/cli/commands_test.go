package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/pastlog/pastlog/internal/render"
)

// TestMain pins colors off so golden output is deterministic regardless of
// the host terminal.
func TestMain(m *testing.M) {
	color.NoColor = true
	m.Run()
}

const sessionAContent = `{"type":"summary","summary":"Fix jwt refresh token rotation","sessionId":"aaaa1111-1111-4111-8111-111111111111"}
{"type":"user","sessionId":"aaaa1111-1111-4111-8111-111111111111","cwd":"/home/dev/myapp","timestamp":"2026-08-02T14:03:22Z","message":{"role":"user","content":"the refresh token is stored in localStorage"}}
{"type":"assistant","sessionId":"aaaa1111-1111-4111-8111-111111111111","cwd":"/home/dev/myapp","timestamp":"2026-08-02T14:03:25Z","message":{"role":"assistant","content":"moving it to an httpOnly cookie"}}
`

const sessionBContent = `{"type":"user","sessionId":"bbbb2222-2222-4222-8222-222222222222","cwd":"/home/dev/other","timestamp":"2026-07-01T10:00:00Z","message":{"role":"user","content":"second project session"}}
`

// isolateDataHome points the per-OS XDG data-dir override at a neutral temp
// dir so host OpenCode data cannot leak into the goldens (M3: the opencode
// adapter resolves its storage root via LOCALAPPDATA / XDG_DATA_HOME).
func isolateDataHome(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Setenv("LOCALAPPDATA", t.TempDir())
	} else {
		t.Setenv("XDG_DATA_HOME", t.TempDir())
	}
}

// fixtureHome builds a synthetic home: two projects, two sessions.
func fixtureHome(t *testing.T) string {
	t.Helper()
	isolateDataHome(t)
	home := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(home, ".claude", "projects", rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("C--Users-dev-myapp/aaaa1111-1111-4111-8111-111111111111.jsonl", sessionAContent)
	write("-home-dev-other/bbbb2222-2222-4222-8222-222222222222.jsonl", sessionBContent)
	return home
}

func fixtureSize(t *testing.T, home, rel string) int64 {
	t.Helper()
	info, err := os.Stat(filepath.Join(home, ".claude", "projects", rel))
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := Execute(args, out, errOut)
	return code, out.String(), errOut.String()
}

func TestAgentsHumanGolden(t *testing.T) {
	home := fixtureHome(t)
	sizeA := fixtureSize(t, home, "C--Users-dev-myapp/aaaa1111-1111-4111-8111-111111111111.jsonl")
	sizeB := fixtureSize(t, home, "-home-dev-other/bbbb2222-2222-4222-8222-222222222222.jsonl")

	code, out, errOut := run(t, "--home", home, "agents")
	if code != 0 {
		t.Fatalf("agents exit = %d, stderr: %s", code, errOut)
	}
	want := fmt.Sprintf("claude-code  2 sessions  %s  ~/.claude/projects\n"+
		"codex        0 sessions  (not found)\n"+
		"gemini-cli   0 sessions  (not found)\n"+
		"opencode     0 sessions  (not found)\n",
		render.HumanBytes(sizeA+sizeB))
	if out != want {
		t.Errorf("agents output:\n%q\nwant:\n%q", out, want)
	}
}

func TestAgentsNotFound(t *testing.T) {
	isolateDataHome(t)
	home := t.TempDir() // no agent data at all
	code, out, errOut := run(t, "--home", home, "agents")
	if code != 0 {
		t.Fatalf("agents exit = %d, stderr: %s", code, errOut)
	}
	want := "claude-code  0 sessions  (not found)\n" +
		"codex        0 sessions  (not found)\n" +
		"gemini-cli   0 sessions  (not found)\n" +
		"opencode     0 sessions  (not found)\n" +
		"nothing found — install an agent or pass --home <dir>\n"
	if out != want {
		t.Errorf("agents output:\n%q\nwant:\n%q", out, want)
	}
}

func TestAgentsJSONGolden(t *testing.T) {
	home := fixtureHome(t)
	sizeA := fixtureSize(t, home, "C--Users-dev-myapp/aaaa1111-1111-4111-8111-111111111111.jsonl")
	sizeB := fixtureSize(t, home, "-home-dev-other/bbbb2222-2222-4222-8222-222222222222.jsonl")

	code, out, errOut := run(t, "--home", home, "agents", "--json")
	if code != 0 {
		t.Fatalf("agents --json exit = %d, stderr: %s", code, errOut)
	}
	want := fmt.Sprintf(`[
  {
    "name": "claude-code",
    "detected": true,
    "path": %q,
    "sessions": 2,
    "bytes": %d
  },
  {
    "name": "codex",
    "detected": false,
    "path": null,
    "sessions": 0,
    "bytes": 0
  },
  {
    "name": "gemini-cli",
    "detected": false,
    "path": null,
    "sessions": 0,
    "bytes": 0
  },
  {
    "name": "opencode",
    "detected": false,
    "path": null,
    "sessions": 0,
    "bytes": 0
  }
]
`, filepath.Join(home, ".claude", "projects"), sizeA+sizeB)
	if out != want {
		t.Errorf("agents json output:\n%q\nwant:\n%q", out, want)
	}
}

func TestSessionsJSONGolden(t *testing.T) {
	home := fixtureHome(t)
	sizeA := fixtureSize(t, home, "C--Users-dev-myapp/aaaa1111-1111-4111-8111-111111111111.jsonl")
	sizeB := fixtureSize(t, home, "-home-dev-other/bbbb2222-2222-4222-8222-222222222222.jsonl")

	code, out, errOut := run(t, "--home", home, "sessions", "--json")
	if code != 0 {
		t.Fatalf("sessions --json exit = %d, stderr: %s", code, errOut)
	}
	want := fmt.Sprintf(`[
  {
    "id": "aaaa1111-1111-4111-8111-111111111111",
    "agent": "claude-code",
    "project": "/home/dev/myapp",
    "title": "Fix jwt refresh token rotation",
    "started_at": "2026-08-02T14:03:22Z",
    "ended_at": "2026-08-02T14:03:25Z",
    "messages": 2,
    "size_bytes": %d
  },
  {
    "id": "bbbb2222-2222-4222-8222-222222222222",
    "agent": "claude-code",
    "project": "/home/dev/other",
    "title": "",
    "started_at": "2026-07-01T10:00:00Z",
    "ended_at": "2026-07-01T10:00:00Z",
    "messages": 1,
    "size_bytes": %d
  }
]
`, sizeA, sizeB)
	if out != want {
		t.Errorf("sessions json output:\n%q\nwant:\n%q", out, want)
	}
}

// TestSessionsHumanGolden pins the command→render composition for the human
// `sessions` output at the CLI layer (M4-B12): column alignment, pluralization,
// date rendering and the id prefix must stay stable end to end, not only in
// the render unit test.
func TestSessionsHumanGolden(t *testing.T) {
	home := fixtureHome(t)
	sizeA := fixtureSize(t, home, "C--Users-dev-myapp/aaaa1111-1111-4111-8111-111111111111.jsonl")
	sizeB := fixtureSize(t, home, "-home-dev-other/bbbb2222-2222-4222-8222-222222222222.jsonl")

	code, out, errOut := run(t, "--home", home, "sessions")
	if code != 0 {
		t.Fatalf("sessions exit = %d, stderr: %s", code, errOut)
	}
	startA := mustUTC(t, "2026-08-02T14:03:22Z")
	startB := mustUTC(t, "2026-07-01T10:00:00Z")
	want := fmt.Sprintf("claude-code  /home/dev/myapp  %s  2 messages  %s  aaaa1111\n"+
		"claude-code  /home/dev/other  %s  1 message   %s  bbbb2222\n",
		startA.Local().Format("2006-01-02 15:04"), render.HumanBytes(sizeA),
		startB.Local().Format("2006-01-02 15:04"), render.HumanBytes(sizeB))
	if out != want {
		t.Errorf("sessions output:\n%q\nwant:\n%q", out, want)
	}
	if errOut != "" {
		t.Errorf("stderr should stay empty, got %q", errOut)
	}
}

func TestSessionsFilters(t *testing.T) {
	home := fixtureHome(t)

	t.Run("limit", func(t *testing.T) {
		code, out, errOut := run(t, "--home", home, "sessions", "--json", "--limit", "1")
		if code != 0 {
			t.Fatalf("exit = %d, stderr: %s", code, errOut)
		}
		if !strings.Contains(out, "aaaa1111") || strings.Contains(out, "bbbb2222") {
			t.Errorf("limit 1 should keep only the newest session, got:\n%s", out)
		}
	})
	t.Run("project substring", func(t *testing.T) {
		code, out, _ := run(t, "--home", home, "sessions", "--json", "--project", "OTHER")
		if code != 0 {
			t.Fatalf("exit = %d", code)
		}
		if !strings.Contains(out, "bbbb2222") || strings.Contains(out, "aaaa1111") {
			t.Errorf("project filter (case-insensitive) failed, got:\n%s", out)
		}
	})
	t.Run("since date", func(t *testing.T) {
		code, out, _ := run(t, "--home", home, "sessions", "--json", "--since", "2026-07-15")
		if code != 0 {
			t.Fatalf("exit = %d", code)
		}
		if !strings.Contains(out, "aaaa1111") || strings.Contains(out, "bbbb2222") {
			t.Errorf("--since 2026-07-15 should keep only july-2nd-onwards, got:\n%s", out)
		}
	})
	t.Run("until date", func(t *testing.T) {
		code, out, _ := run(t, "--home", home, "sessions", "--json", "--until", "2026-07-02")
		if code != 0 {
			t.Fatalf("exit = %d", code)
		}
		if !strings.Contains(out, "bbbb2222") || strings.Contains(out, "aaaa1111") {
			t.Errorf("--until 2026-07-02 should keep only the july session, got:\n%s", out)
		}
	})
	t.Run("skipped lines noted on stderr", func(t *testing.T) {
		broken := filepath.Join(home, ".claude", "projects", "C--Users-dev-myapp", "cccc3333-3333-4333-8333-333333333333.jsonl")
		if err := os.WriteFile(broken, []byte("{\"type\":\"user\",\"sessionId\":\"cccc\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Remove(broken) })

		code, _, errOut := run(t, "--home", home, "sessions", "--json")
		if code != 0 {
			t.Fatalf("exit = %d", code)
		}
		if !strings.Contains(errOut, "1 unreadable lines skipped") {
			t.Errorf("stderr should note skipped lines, got %q", errOut)
		}
	})
}

func TestSessionsFilterErrors(t *testing.T) {
	home := fixtureHome(t)

	t.Run("bad since format", func(t *testing.T) {
		code, out, errOut := run(t, "--home", home, "sessions", "--since", "3x")
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
		if out != "" {
			t.Errorf("stdout should be empty on error, got %q", out)
		}
		if !strings.Contains(errOut, "invalid --since value \"3x\"") {
			t.Errorf("stderr should explain the format, got %q", errOut)
		}
	})
	t.Run("bad limit", func(t *testing.T) {
		code, _, errOut := run(t, "--home", home, "sessions", "--limit", "-1")
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
		if !strings.Contains(errOut, "--limit") {
			t.Errorf("stderr should mention --limit, got %q", errOut)
		}
	})
	t.Run("unknown agent", func(t *testing.T) {
		code, _, errOut := run(t, "--home", home, "sessions", "--agent", "not-an-agent")
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
		if !strings.Contains(errOut, `unknown agent "not-an-agent"`) {
			t.Errorf("stderr should name the unknown agent, got %q", errOut)
		}
	})
	t.Run("bad home", func(t *testing.T) {
		code, _, errOut := run(t, "--home", "/no/such/dir", "sessions")
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
		if !strings.Contains(errOut, "home directory not found") {
			t.Errorf("stderr should explain --home failure, got %q", errOut)
		}
	})
}

func TestBareInvocationShowsHelpAndSummary(t *testing.T) {
	home := fixtureHome(t)
	code, out, errOut := run(t, "--home", home)
	if code != 0 {
		t.Fatalf("bare exit = %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("bare invocation should print help, got:\n%s", out)
	}
	if !strings.Contains(out, "claude-code: 2 sessions") {
		t.Errorf("bare invocation should summarize detected agents, got:\n%s", out)
	}
}

func TestBareInvocationNothingFound(t *testing.T) {
	isolateDataHome(t)
	code, out, _ := run(t, "--home", t.TempDir())
	if code != 0 {
		t.Fatalf("bare exit = %d", code)
	}
	if !strings.Contains(out, "no agent data found") {
		t.Errorf("bare invocation should print friendly hint, got:\n%s", out)
	}
}
