package cli

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The stats goldens below run on the multi-agent fixture home (M7): claude-code
// (1 session, 2 messages, input 120, output 45, cache write 30, cache read 200,
// claude-sonnet-4-5), codex (1 session, 1 message, input 300, output 90,
// reasoning 25, cache read 80, gpt-5.3-codex via the last turn_context),
// gemini-cli (1 session, 2 messages, input 200, output 60, reasoning 10,
// cache read 15, gemini-2.5-pro) and opencode (two sessions: 1523/412/87/
// 10240/512 cost 0.42 and 310/95/20/0/128 cost 0.08, qwen3-coder-480b).

func TestStatsByAgentHumanGolden(t *testing.T) {
	home := allAgentsHome(t)
	code, out, errOut := run(t, "--home", home, "stats")
	if code != 0 {
		t.Fatalf("stats exit = %d, stderr: %s", code, errOut)
	}
	want := `agent        sessions  messages  input  output  reasoning  cache read  cache write  total  cost usd
claude-code         1         2    120      45          0         200           30    165         -
codex               1         1    300      90         25          80            0    415         -
gemini-cli          1         2    200      60         10          15            0    270         -
opencode            2         3  1,833     507        107      10,240          640  2,447      0.50
`
	if out != want {
		t.Errorf("stats output:\n%q\nwant:\n%q", out, want)
	}
	if errOut != "" {
		t.Errorf("stderr should stay empty, got %q", errOut)
	}
}

func TestStatsByAgentJSONGolden(t *testing.T) {
	home := allAgentsHome(t)
	code, out, errOut := run(t, "--home", home, "stats", "--json")
	if code != 0 {
		t.Fatalf("stats --json exit = %d, stderr: %s", code, errOut)
	}
	want := `[
  {
    "key": "claude-code",
    "sessions": 1,
    "messages": 2,
    "tokens": {
      "input": 120,
      "output": 45,
      "reasoning": 0,
      "cache_read": 200,
      "cache_write": 30,
      "total": 165
    },
    "cost_usd": null
  },
  {
    "key": "codex",
    "sessions": 1,
    "messages": 1,
    "tokens": {
      "input": 300,
      "output": 90,
      "reasoning": 25,
      "cache_read": 80,
      "cache_write": 0,
      "total": 415
    },
    "cost_usd": null
  },
  {
    "key": "gemini-cli",
    "sessions": 1,
    "messages": 2,
    "tokens": {
      "input": 200,
      "output": 60,
      "reasoning": 10,
      "cache_read": 15,
      "cache_write": 0,
      "total": 270
    },
    "cost_usd": null
  },
  {
    "key": "opencode",
    "sessions": 2,
    "messages": 3,
    "tokens": {
      "input": 1833,
      "output": 507,
      "reasoning": 107,
      "cache_read": 10240,
      "cache_write": 640,
      "total": 2447
    },
    "cost_usd": 0.5
  }
]
`
	if out != want {
		t.Errorf("stats json output:\n%q\nwant:\n%q", out, want)
	}
}

// TestStatsByProjectJSON pins the project grouping keys verbatim (display
// form like SessionsHuman: raw paths here, since the fixture projects do not
// live under the fixture home — the tilde-shortening itself is pinned in the
// render tests), plus the cost column behavior per selection.
func TestStatsByProjectJSON(t *testing.T) {
	home := allAgentsHome(t)
	code, out, errOut := run(t, "--home", home, "stats", "--by", "project", "--json")
	if code != 0 {
		t.Fatalf("stats --by project exit = %d, stderr: %s", code, errOut)
	}
	for _, want := range []string{
		`"key": "C:/dev/fixture app"`,
		`"key": "/home/dev/cal"`,
		`"sessions": 2`,
		`"sessions": 3`,
		`"messages": 5`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stats --by project should contain %s, got:\n%s", want, out)
		}
	}
	if strings.Count(out, `"cost_usd": null`) != 1 {
		t.Errorf("only the non-opencode group should have null cost, got:\n%s", out)
	}

	// excluding opencode drops the cost column entirely (human output)
	code, out, errOut = run(t, "--home", home, "stats", "--by", "project", "--project", "cal")
	if code != 0 {
		t.Fatalf("human exit = %d, stderr: %s", code, errOut)
	}
	if strings.Contains(out, "cost usd") {
		t.Errorf("cost column must disappear when no session carries a cost, got:\n%s", out)
	}
	if !strings.Contains(out, "/home/dev/cal") || strings.Contains(out, "fixture app") {
		t.Errorf("--project cal should keep only the cal group, got:\n%s", out)
	}
}

// TestStatsByDay pins the day bucketing: local 2006-01-02 of StartedAt,
// ascending (= chronological). The three JSONL agents share one start date
// (2026-08-02 UTC); opencode is 2026-08-08 09:48 UTC, six days later, so the
// grouping is stable on every timezone.
func TestStatsByDay(t *testing.T) {
	home := allAgentsHome(t)
	code, out, errOut := run(t, "--home", home, "stats", "--by", "day", "--json")
	if code != 0 {
		t.Fatalf("stats --by day exit = %d, stderr: %s", code, errOut)
	}
	jsonlDay := time.Date(2026, 8, 2, 14, 3, 22, 0, time.UTC).Local().Format("2006-01-02")
	opencodeDay := time.UnixMilli(1786220928012).Local().Format("2006-01-02")
	if jsonlDay == opencodeDay {
		t.Fatalf("fixture assumption broken: both groups landed on %s", jsonlDay)
	}
	// the JSONL-agents day: 3 sessions, 5 messages, input 620 (120+300+200)
	wantJSONL := `"key": "` + jsonlDay + `",
    "sessions": 3,
    "messages": 5,
    "tokens": {
      "input": 620,`
	if !strings.Contains(out, wantJSONL) {
		t.Errorf("day group %s missing or wrong, got:\n%s", jsonlDay, out)
	}
	// the opencode day: 2 sessions, 3 messages, input 1833, cost 0.5
	wantOpen := `"key": "` + opencodeDay + `",
    "sessions": 2,
    "messages": 3,
    "tokens": {
      "input": 1833,`
	if !strings.Contains(out, wantOpen) {
		t.Errorf("day group %s missing or wrong, got:\n%s", opencodeDay, out)
	}
	if strings.Index(out, jsonlDay) > strings.Index(out, opencodeDay) {
		t.Errorf("day groups must be chronological, got:\n%s", out)
	}
}

func TestStatsByModelJSON(t *testing.T) {
	home := allAgentsHome(t)
	code, out, errOut := run(t, "--home", home, "stats", "--by", "model", "--json")
	if code != 0 {
		t.Fatalf("stats --by model exit = %d, stderr: %s", code, errOut)
	}
	// ascending keys; the qwen group aggregates both opencode sessions
	want := []string{
		`"key": "claude-sonnet-4-5"`,
		`"key": "gemini-2.5-pro"`,
		`"key": "gpt-5.3-codex"`,
		`"key": "qwen3-coder-480b",
    "sessions": 2,
    "messages": 3`,
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("stats --by model should contain %q, got:\n%s", w, out)
		}
	}
	if !strings.Contains(out, `"cost_usd": 0.5`) {
		t.Errorf("the qwen group should carry the summed cost, got:\n%s", out)
	}
}

func TestStatsFilters(t *testing.T) {
	home := allAgentsHome(t)

	t.Run("agent filter", func(t *testing.T) {
		code, out, _ := run(t, "--home", home, "stats", "--agent", "codex", "--json")
		if code != 0 {
			t.Fatalf("exit = %d", code)
		}
		if !strings.Contains(out, `"key": "codex"`) || strings.Contains(out, "claude-code") {
			t.Errorf("agent filter should keep only codex, got:\n%s", out)
		}
	})
	t.Run("model filter case-insensitive", func(t *testing.T) {
		code, out, _ := run(t, "--home", home, "stats", "--by", "model", "--model", "GPT", "--json")
		if code != 0 {
			t.Fatalf("exit = %d", code)
		}
		if !strings.Contains(out, `"key": "gpt-5.3-codex"`) || strings.Contains(out, "claude-sonnet") {
			t.Errorf("--model GPT should keep only codex, got:\n%s", out)
		}
	})
	t.Run("project filter drops the cost column", func(t *testing.T) {
		code, out, _ := run(t, "--home", home, "stats", "--project", "cal")
		if code != 0 {
			t.Fatalf("exit = %d", code)
		}
		if strings.Contains(out, "cost usd") {
			t.Errorf("--project cal excludes opencode, so no cost column, got:\n%s", out)
		}
		if !strings.Contains(out, "codex") {
			t.Errorf("--project cal should keep the JSONL agents, got:\n%s", out)
		}
	})
	t.Run("until keeps only the JSONL agents", func(t *testing.T) {
		code, out, _ := run(t, "--home", home, "stats", "--until", "2026-08-03", "--json")
		if code != 0 {
			t.Fatalf("exit = %d", code)
		}
		if strings.Contains(out, "opencode") {
			t.Errorf("--until 2026-08-03 should drop opencode, got:\n%s", out)
		}
	})
	t.Run("since keeps only opencode", func(t *testing.T) {
		code, out, _ := run(t, "--home", home, "stats", "--since", "2026-08-04", "--json")
		if code != 0 {
			t.Fatalf("exit = %d", code)
		}
		if !strings.Contains(out, `"key": "opencode"`) || strings.Contains(out, "claude-code") {
			t.Errorf("--since 2026-08-04 should keep only opencode, got:\n%s", out)
		}
	})
	t.Run("zero-usage sessions still count", func(t *testing.T) {
		// claude-code from the plain fixtureHome has no usage fields at all:
		// two zero-usage sessions with 2+1 messages, still aggregated
		home := fixtureHome(t)
		code, out, errOut := run(t, "--home", home, "stats", "--json")
		if code != 0 {
			t.Fatalf("exit = %d, stderr: %s", code, errOut)
		}
		for _, want := range []string{`"sessions": 2`, `"messages": 3`, `"input": 0`, `"cost_usd": null`} {
			if !strings.Contains(out, want) {
				t.Errorf("zero-usage session should still count (%s), got:\n%s", want, out)
			}
		}
	})
}

func TestStatsEmptySelection(t *testing.T) {
	home := allAgentsHome(t)

	t.Run("human prints nothing and exits 0", func(t *testing.T) {
		code, out, errOut := run(t, "--home", home, "stats", "--model", "zzz-nomatch")
		if code != 0 {
			t.Fatalf("empty stats must exit 0, got %d, stderr: %s", code, errOut)
		}
		if out != "" {
			t.Errorf("empty human output should print nothing, got %q", out)
		}
	})
	t.Run("json prints an empty array", func(t *testing.T) {
		code, out, _ := run(t, "--home", home, "stats", "--model", "zzz-nomatch", "--json")
		if code != 0 {
			t.Fatalf("empty stats must exit 0, got %d", code)
		}
		if out != "[]\n" {
			t.Errorf("empty json should be [], got %q", out)
		}
	})
}

func TestStatsErrors(t *testing.T) {
	home := allAgentsHome(t)

	t.Run("invalid --by", func(t *testing.T) {
		code, out, errOut := run(t, "--home", home, "stats", "--by", "week")
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
		if out != "" {
			t.Errorf("stdout should be empty on error, got %q", out)
		}
		if !strings.Contains(errOut, `invalid --by value "week"`) || !strings.Contains(errOut, "agent, project, day or model") {
			t.Errorf("stderr should explain the valid values, got %q", errOut)
		}
	})
	t.Run("unknown --agent", func(t *testing.T) {
		code, _, errOut := run(t, "--home", home, "stats", "--agent", "not-an-agent")
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
		if !strings.Contains(errOut, `unknown agent "not-an-agent"`) {
			t.Errorf("stderr should name the unknown agent, got %q", errOut)
		}
	})
	t.Run("bad --since", func(t *testing.T) {
		code, _, errOut := run(t, "--home", home, "stats", "--since", "3x")
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
		if !strings.Contains(errOut, `invalid --since value "3x"`) {
			t.Errorf("stderr should explain the format, got %q", errOut)
		}
	})
	t.Run("bad home", func(t *testing.T) {
		code, _, errOut := run(t, "--home", "/no/such/dir", "stats")
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
		if !strings.Contains(errOut, "home directory not found") {
			t.Errorf("stderr should explain --home failure, got %q", errOut)
		}
	})
}

// TestStatsLockedDBWarnsOnce is the spec §4 fallback on the stats path: one
// warning, other agents' numbers unaffected, exit 0 (stats never fails on a
// locked database).
func TestStatsLockedDBWarnsOnce(t *testing.T) {
	home := allAgentsHome(t)
	release := lockDB(t, filepath.Join(home, ".local", "share", "opencode", "opencode.db"))
	defer release()

	code, out, errOut := run(t, "--home", home, "stats", "--json")
	if code != 0 {
		t.Fatalf("stats exit = %d with locked opencode, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, `"key": "claude-code"`) {
		t.Errorf("other agents must be unaffected, got:\n%s", out)
	}
	if strings.Contains(out, `"key": "opencode"`) {
		t.Errorf("locked opencode must yield no group, got:\n%s", out)
	}
	if n := strings.Count(errOut, "opencode: database is locked"); n != 1 {
		t.Errorf("exactly one locked-DB warning expected, got %d in %q", n, errOut)
	}
}

// TestStatsNoteUnreadableStorage pins the noteStderr integration: a corrupt
// opencode database is noted once on stderr and the run stays exit 0.
func TestStatsNoteUnreadableStorage(t *testing.T) {
	home := fixtureHome(t)
	corruptOpencodeDB(t, home)
	code, _, errOut := run(t, "--home", home, "stats")
	if code != 0 {
		t.Fatalf("stats exit = %d, stderr: %s", code, errOut)
	}
	if strings.Count(errOut, "opencode: storage unreadable") != 1 {
		t.Errorf("exactly one unreadable-storage note expected, got %q", errOut)
	}
}
