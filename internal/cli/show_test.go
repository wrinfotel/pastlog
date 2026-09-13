package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pastlog/pastlog/internal/render"
)

func TestShowGolden(t *testing.T) {
	home := searchShowHome(t)
	size := fixtureFileSize(t, home, filepath.Join(".claude", "projects", "C--Users-dev-myapp", "aaaa1111-1111-4111-8111-111111111111.jsonl"))

	code, out, errOut := run(t, "--home", home, "show", "aaaa1111-1111-4111-8111-111111111111")
	if code != 0 {
		t.Fatalf("show exit = %d, stderr: %s", code, errOut)
	}
	// transcript times render in the local zone; compute the expectation the
	// same way so the golden stays machine-independent
	start := mustUTC(t, "2026-08-02T14:03:22Z")
	end := mustUTC(t, "2026-08-02T14:03:25Z")
	want := fmt.Sprintf(`# Fix jwt refresh token rotation
claude-code · aaaa1111-1111-4111-8111-111111111111 · /home/dev/myapp · %s → %s · 2 messages · %d B

       -  summary    Fix jwt refresh token rotation
%s  user       the refresh token is stored in localStorage
%s  assistant  fixing jwt refresh rotation now
`,
		start.Local().Format("2006-01-02 15:04:05"), end.Local().Format("2006-01-02 15:04:05"),
		size,
		start.Local().Format("15:04:05"), end.Local().Format("15:04:05"))
	if out != want {
		t.Errorf("show output:\n%s\nwant:\n%s", out, want)
	}
}

func TestShowUniquePrefix(t *testing.T) {
	home := searchShowHome(t)
	code, out, errOut := run(t, "--home", home, "show", "bbbb")
	if code != 0 {
		t.Fatalf("show by prefix exit = %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "jwt refresh flow broke again") {
		t.Errorf("prefix should resolve to the bbbb session, got:\n%s", out)
	}
}

func TestShowAmbiguousPrefixExit2(t *testing.T) {
	home := searchShowHome(t)
	code, out, errOut := run(t, "--home", home, "show", "aaaa")
	if code != 2 {
		t.Fatalf("ambiguous prefix exit = %d, want 2", code)
	}
	if out != "" {
		t.Errorf("stdout should stay empty, got %q", out)
	}
	msg := errOut
	if !strings.Contains(msg, `ambiguous session id prefix "aaaa"`) {
		t.Errorf("stderr should explain the ambiguity, got %q", msg)
	}
	for _, id := range []string{"aaaa1111-1111-4111-8111-111111111111", "aaaa9999-9999-4999-8999-999999999999"} {
		if !strings.Contains(msg, id) {
			t.Errorf("candidates should list %s, got:\n%s", id, msg)
		}
	}
	// candidates are newest first: aaaa1111 (2026-08-02) before aaaa9999
	if strings.Index(msg, "aaaa9999") < strings.Index(msg, "aaaa1111") {
		t.Errorf("candidates should be newest first, got:\n%s", msg)
	}
}

func TestShowNoMatchExit2(t *testing.T) {
	home := searchShowHome(t)
	code, _, errOut := run(t, "--home", home, "show", "zzzz")
	if code != 2 {
		t.Errorf("no match exit = %d, want 2", code)
	}
	if !strings.Contains(errOut, `no session matches id prefix "zzzz"`) {
		t.Errorf("stderr should explain the miss, got %q", errOut)
	}
}

func TestShowJSON(t *testing.T) {
	home := searchShowHome(t)
	size := fixtureFileSize(t, home, filepath.Join(".claude", "projects", "C--Users-dev-myapp", "aaaa1111-1111-4111-8111-111111111111.jsonl"))

	code, out, errOut := run(t, "--home", home, "show", "aaaa1111", "--json")
	if code != 0 {
		t.Fatalf("show --json exit = %d, stderr: %s", code, errOut)
	}
	want := fmt.Sprintf(`{
  "id": "aaaa1111-1111-4111-8111-111111111111",
  "agent": "claude-code",
  "project": "/home/dev/myapp",
  "title": "Fix jwt refresh token rotation",
  "started_at": "2026-08-02T14:03:22Z",
  "ended_at": "2026-08-02T14:03:25Z",
  "messages": 2,
  "size_bytes": %d,
  "entries": [
    {
      "kind": "summary",
      "role": "",
      "text": "Fix jwt refresh token rotation",
      "timestamp": null
    },
    {
      "kind": "message",
      "role": "user",
      "text": "the refresh token is stored in localStorage",
      "timestamp": "2026-08-02T14:03:22Z"
    },
    {
      "kind": "message",
      "role": "assistant",
      "text": "fixing jwt refresh rotation now",
      "timestamp": "2026-08-02T14:03:25Z"
    }
  ]
}
`, size)
	if out != want {
		t.Errorf("show json output:\n%s\nwant:\n%s", out, want)
	}
}

func TestShowExportMarkdown(t *testing.T) {
	home := searchShowHome(t)
	size := fixtureFileSize(t, home, filepath.Join(".claude", "projects", "C--Users-dev-myapp", "aaaa1111-1111-4111-8111-111111111111.jsonl"))
	code, out, errOut := run(t, "--home", home, "show", "aaaa1111", "--export", "md")
	if code != 0 {
		t.Fatalf("show --export md exit = %d, stderr: %s", code, errOut)
	}
	start := mustUTC(t, "2026-08-02T14:03:22Z")
	end := mustUTC(t, "2026-08-02T14:03:25Z")
	want := fmt.Sprintf(`# Fix jwt refresh token rotation

- **agent:** claude-code
- **session:** aaaa1111-1111-4111-8111-111111111111
- **project:** /home/dev/myapp
- **started:** %s
- **ended:** %s
- **messages:** 2
- **size:** %s

## summary

Fix jwt refresh token rotation

## user · %s

the refresh token is stored in localStorage

## assistant · %s

fixing jwt refresh rotation now

`,
		start.Local().Format("2006-01-02 15:04:05"), end.Local().Format("2006-01-02 15:04:05"),
		render.HumanBytes(size),
		start.Local().Format("2006-01-02 15:04:05"), end.Local().Format("2006-01-02 15:04:05"))
	if out != want {
		t.Errorf("show markdown output:\n%s\nwant:\n%s", out, want)
	}
}

func TestShowExportErrors(t *testing.T) {
	home := searchShowHome(t)

	t.Run("unsupported format", func(t *testing.T) {
		code, _, errOut := run(t, "--home", home, "show", "aaaa1111", "--export", "html")
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
		if !strings.Contains(errOut, `unsupported --export format "html"`) {
			t.Errorf("stderr should name the format, got %q", errOut)
		}
	})
	t.Run("json and export conflict", func(t *testing.T) {
		code, _, errOut := run(t, "--home", home, "show", "aaaa1111", "--json", "--export", "md")
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
		if !strings.Contains(errOut, "--json and --export cannot be combined") {
			t.Errorf("stderr should explain the conflict, got %q", errOut)
		}
	})
}

func mustUTC(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}
