package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Fixture home shared by the search and show command tests: two claude-code
// projects plus a codex rollout, so search must cross adapters.
const (
	claudeAContent = `{"type":"summary","summary":"Fix jwt refresh token rotation","sessionId":"aaaa1111-1111-4111-8111-111111111111"}
{"type":"user","sessionId":"aaaa1111-1111-4111-8111-111111111111","cwd":"/home/dev/myapp","timestamp":"2026-08-02T14:03:22Z","message":{"role":"user","content":"the refresh token is stored in localStorage"}}
{"type":"assistant","sessionId":"aaaa1111-1111-4111-8111-111111111111","cwd":"/home/dev/myapp","timestamp":"2026-08-02T14:03:25Z","message":{"role":"assistant","content":"fixing jwt refresh rotation now"}}
`

	claudeBContent = `{"type":"user","sessionId":"bbbb2222-2222-4222-8222-222222222222","cwd":"/home/dev/other","timestamp":"2026-07-01T10:00:00Z","message":{"role":"user","content":"jwt refresh flow broke again"}}
{"type":"assistant","sessionId":"bbbb2222-2222-4222-8222-222222222222","cwd":"/home/dev/other","timestamp":"2026-07-01T10:00:10Z","message":{"role":"assistant","content":"please check JWT refresh handling"}}
`

	// aaaa9999 never matches "jwt refresh"; it exists so that the prefix
	// "aaaa" is ambiguous for `show`. Its date keeps it older than aaaa1111
	// so the search --limit test is stable.
	claudeCContent = `{"type":"user","sessionId":"aaaa9999-9999-4999-8999-999999999999","cwd":"/home/dev/myapp","timestamp":"2026-06-15T09:00:00Z","message":{"role":"user","content":"unrelated session about databases"}}
`

	// dddd4444 carries a quoted needle ("jwt) for the prefilter-disabled
	// end-to-end test but never matches "jwt refresh", keeping the main
	// goldens stable.
	claudeDContent = `{"type":"user","sessionId":"dddd4444-4444-4444-8444-444444444444","cwd":"/home/dev/other","timestamp":"2026-06-01T08:00:00Z","message":{"role":"user","content":"the parameter \"jwt secret\" is set"}}
`

	codexContent = `{"timestamp":"2026-08-02T14:10:00Z","type":"session_meta","payload":{"id":"3f9c81a2-1111-4222-8333-cccccccccccc","cwd":"/home/dev/myapp/api","cli_version":"0.42.1"}}
{"timestamp":"2026-08-02T14:10:05Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"the issue was the auth module"},{"type":"output_text","text":"we fixed jwt refresh rotation"}]}}
{"timestamp":"2026-08-02T14:10:10Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"tests cover the café flow now"},{"type":"output_text","text":"jwt refresh is verified"}]}}
`
)

// searchShowHome builds the combined two-adapter fixture home.
func searchShowHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(".claude", "projects", "C--Users-dev-myapp", "aaaa1111-1111-4111-8111-111111111111.jsonl"), claudeAContent)
	write(filepath.Join(".claude", "projects", "-home-dev-other", "bbbb2222-2222-4222-8222-222222222222.jsonl"), claudeBContent)
	write(filepath.Join(".claude", "projects", "C--Users-dev-myapp", "aaaa9999-9999-4999-8999-999999999999.jsonl"), claudeCContent)
	write(filepath.Join(".claude", "projects", "-home-dev-other", "dddd4444-4444-4444-8444-444444444444.jsonl"), claudeDContent)
	write(filepath.Join(".codex", "sessions", "2026", "08", "02",
		"rollout-2026-08-02T14-10-00-3f9c81a2-1111-4222-8333-cccccccccccc.jsonl"), codexContent)
	return home
}

func fixtureFileSize(t *testing.T, home, rel string) int64 {
	t.Helper()
	info, err := os.Stat(filepath.Join(home, rel))
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

func TestSearchGolden(t *testing.T) {
	home := searchShowHome(t)
	code, out, errOut := run(t, "--home", home, "search", "jwt refresh")
	if code != 0 {
		t.Fatalf("search exit = %d, stderr: %s", code, errOut)
	}
	want := `codex · myapp/api · 2026-08-02 · sess 3f9c81a2
  the issue was the auth module
  → we fixed jwt refresh rotation
  tests cover the café flow now
  → jwt refresh is verified

claude-code · dev/myapp · 2026-08-02 · sess aaaa1111
  → Fix jwt refresh token rotation
  → fixing jwt refresh rotation now

claude-code · dev/other · 2026-07-01 · sess bbbb2222
  → jwt refresh flow broke again
  → please check JWT refresh handling
`
	if out != want {
		t.Errorf("search output:\n%s\nwant:\n%s", out, want)
	}
	if errOut != "" {
		t.Errorf("stderr should stay empty, got %q", errOut)
	}
}

func TestSearchNoMatchesExit1(t *testing.T) {
	home := searchShowHome(t)
	code, out, errOut := run(t, "--home", home, "search", "nonexistent-needle")
	if code != 1 {
		t.Errorf("search without matches exit = %d, want 1", code)
	}
	if out != "" || errOut != "" {
		t.Errorf("no matches should print nothing, got stdout %q stderr %q", out, errOut)
	}
}

func TestSearchJSONGolden(t *testing.T) {
	home := searchShowHome(t)
	sizeCodex := fixtureFileSize(t, home, filepath.Join(".codex", "sessions", "2026", "08", "02",
		"rollout-2026-08-02T14-10-00-3f9c81a2-1111-4222-8333-cccccccccccc.jsonl"))
	sizeA := fixtureFileSize(t, home, filepath.Join(".claude", "projects", "C--Users-dev-myapp", "aaaa1111-1111-4111-8111-111111111111.jsonl"))
	sizeB := fixtureFileSize(t, home, filepath.Join(".claude", "projects", "-home-dev-other", "bbbb2222-2222-4222-8222-222222222222.jsonl"))

	code, out, errOut := run(t, "--home", home, "search", "jwt refresh", "--json")
	if code != 0 {
		t.Fatalf("search --json exit = %d, stderr: %s", code, errOut)
	}
	want := fmt.Sprintf(`[
  {
    "session": {
      "id": "3f9c81a2-1111-4222-8333-cccccccccccc",
      "agent": "codex",
      "project": "/home/dev/myapp/api",
      "title": "",
      "started_at": "2026-08-02T14:10:00Z",
      "ended_at": "2026-08-02T14:10:10Z",
      "messages": 4,
      "size_bytes": %d
    },
    "hits": [
      {
        "kind": "message",
        "role": "assistant",
        "timestamp": "2026-08-02T14:10:05Z",
        "context": "the issue was the auth module",
        "line": "we fixed jwt refresh rotation",
        "match_start": 9,
        "match_end": 20
      },
      {
        "kind": "message",
        "role": "assistant",
        "timestamp": "2026-08-02T14:10:10Z",
        "context": "tests cover the café flow now",
        "line": "jwt refresh is verified",
        "match_start": 0,
        "match_end": 11
      }
    ]
  },
  {
    "session": {
      "id": "aaaa1111-1111-4111-8111-111111111111",
      "agent": "claude-code",
      "project": "/home/dev/myapp",
      "title": "Fix jwt refresh token rotation",
      "started_at": "2026-08-02T14:03:22Z",
      "ended_at": "2026-08-02T14:03:25Z",
      "messages": 2,
      "size_bytes": %d
    },
    "hits": [
      {
        "kind": "summary",
        "role": "",
        "timestamp": null,
        "context": "",
        "line": "Fix jwt refresh token rotation",
        "match_start": 4,
        "match_end": 15
      },
      {
        "kind": "message",
        "role": "assistant",
        "timestamp": "2026-08-02T14:03:25Z",
        "context": "Fix jwt refresh token rotation",
        "line": "fixing jwt refresh rotation now",
        "match_start": 7,
        "match_end": 18
      }
    ]
  },
  {
    "session": {
      "id": "bbbb2222-2222-4222-8222-222222222222",
      "agent": "claude-code",
      "project": "/home/dev/other",
      "title": "",
      "started_at": "2026-07-01T10:00:00Z",
      "ended_at": "2026-07-01T10:00:10Z",
      "messages": 2,
      "size_bytes": %d
    },
    "hits": [
      {
        "kind": "message",
        "role": "user",
        "timestamp": "2026-07-01T10:00:00Z",
        "context": "",
        "line": "jwt refresh flow broke again",
        "match_start": 0,
        "match_end": 11
      },
      {
        "kind": "message",
        "role": "assistant",
        "timestamp": "2026-07-01T10:00:10Z",
        "context": "jwt refresh flow broke again",
        "line": "please check JWT refresh handling",
        "match_start": 13,
        "match_end": 24
      }
    ]
  }
]
`, sizeCodex, sizeA, sizeB)
	if out != want {
		t.Errorf("search json output:\n%s\nwant:\n%s", out, want)
	}
}

func TestSearchMaxHitsCapsTotal(t *testing.T) {
	home := searchShowHome(t)
	code, out, _ := run(t, "--home", home, "search", "jwt refresh", "--max-hits", "2")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if n := strings.Count(out, "→"); n != 2 {
		t.Errorf("printed hits = %d, want 2", n)
	}
	if strings.Contains(out, "aaaa1111") {
		t.Error("scanning must stop once the hit cap is reached")
	}
}

func TestSearchLimitCapsSessions(t *testing.T) {
	home := searchShowHome(t)
	code, out, _ := run(t, "--home", home, "search", "jwt refresh", "--limit", "1")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "3f9c81a2") {
		t.Error("the newest session should be searched")
	}
	if strings.Contains(out, "aaaa1111") || strings.Contains(out, "bbbb2222") {
		t.Error("--limit 1 must keep older sessions out")
	}
}

func TestSearchCaseSensitive(t *testing.T) {
	home := searchShowHome(t)

	code, out, _ := run(t, "--home", home, "search", "JWT", "--case-sensitive", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	// only the assistant line of session bbbb carries the uppercase form
	if !strings.Contains(out, "bbbb2222") || strings.Contains(out, "aaaa1111") || strings.Contains(out, "3f9c81a2") {
		t.Errorf("case-sensitive JWT should only hit session bbbb, got:\n%s", out)
	}

	code, out, _ = run(t, "--home", home, "search", "JWT", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "aaaa1111") {
		t.Error("default search should fold case and hit every session")
	}
}

func TestSearchRegex(t *testing.T) {
	home := searchShowHome(t)
	code, out, _ := run(t, "--home", home, "search", `jwt (refresh|token)`, "--regex", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "3f9c81a2") || !strings.Contains(out, "aaaa1111") {
		t.Errorf("regex alternation should hit both sessions, got:\n%s", out)
	}
}

func TestSearchUnicodeNeedleDisablesPrefilterStillMatches(t *testing.T) {
	home := searchShowHome(t)
	code, out, _ := run(t, "--home", home, "search", "café", "--json")
	if code != 0 {
		t.Fatalf("non-ASCII needle exit = %d, want 0", code)
	}
	if !strings.Contains(out, "tests cover the café flow now") {
		t.Errorf("unicode needle should match parsed text, got:\n%s", out)
	}
}

func TestSearchQuotedNeedleMatches(t *testing.T) {
	// needles with JSON-escaped chars disable the raw-line prefilter but the
	// match itself must still work end to end
	home := searchShowHome(t)
	code, out, _ := run(t, "--home", home, "search", `"jwt`, "--json")
	if code != 0 {
		t.Fatalf("quoted needle exit = %d, want 0", code)
	}
	if !strings.Contains(out, "dddd4444") || strings.Contains(out, "aaaa1111") {
		t.Errorf("quoted needle should hit only the quoted text, got:\n%s", out)
	}
}

func TestSearchErrorsExit2(t *testing.T) {
	home := searchShowHome(t)

	t.Run("invalid regex", func(t *testing.T) {
		code, _, errOut := run(t, "--home", home, "search", "([", "--regex")
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
		if !strings.Contains(errOut, "invalid --regex pattern") {
			t.Errorf("stderr should name the bad pattern, got %q", errOut)
		}
	})
	t.Run("unknown agent", func(t *testing.T) {
		code, _, errOut := run(t, "--home", home, "search", "jwt", "--agent", "gemini-cli")
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
		if !strings.Contains(errOut, `unknown agent "gemini-cli"`) {
			t.Errorf("stderr should name the unknown agent, got %q", errOut)
		}
	})
	t.Run("bad since", func(t *testing.T) {
		code, _, errOut := run(t, "--home", home, "search", "jwt", "--since", "3x")
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
		if !strings.Contains(errOut, "invalid --since value") {
			t.Errorf("stderr should explain the format, got %q", errOut)
		}
	})
	t.Run("no args", func(t *testing.T) {
		code, _, _ := run(t, "--home", home, "search")
		if code != 2 {
			t.Errorf("missing query exit = %d, want 2", code)
		}
	})
}

func TestSearchAgentAndProjectFilters(t *testing.T) {
	home := searchShowHome(t)

	code, out, _ := run(t, "--home", home, "search", "jwt refresh", "--agent", "codex", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "3f9c81a2") || strings.Contains(out, "aaaa1111") {
		t.Errorf("--agent codex should restrict results, got:\n%s", out)
	}

	code, out, _ = run(t, "--home", home, "search", "jwt refresh", "--project", "OTHER", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "bbbb2222") || strings.Contains(out, "aaaa1111") {
		t.Errorf("--project filter (case-insensitive) should restrict results, got:\n%s", out)
	}
}
