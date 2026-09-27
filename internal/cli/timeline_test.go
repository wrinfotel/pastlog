package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTimelineSessionsChronological(t *testing.T) {
	home := searchShowHome(t)
	code, out, errOut := run(t, "--home", home, "timeline")
	if code != 0 {
		t.Fatalf("timeline exit = %d, stderr: %s", code, errOut)
	}
	// oldest first: dddd (06-01) → aaaa9999 (06-15) → bbbb (07-01) →
	// aaaa1111 (08-02 14:03) → codex (08-02 14:10)
	order := []string{"dddd4444", "aaaa9999", "bbbb2222", "aaaa1111", "3f9c81a2"}
	last := -1
	for _, id := range order {
		i := strings.Index(out, id)
		if i < 0 {
			t.Fatalf("timeline should contain %s, got:\n%s", id, out)
		}
		if i < last {
			t.Errorf("%s appears out of chronological order, got:\n%s", id, out)
		}
		last = i
	}
	if !strings.Contains(out, mustUTC(t, "2026-06-01T08:00:00Z").Local().Format("2006-01-02 15:04")) {
		t.Errorf("session rows should carry date and time, got:\n%s", out)
	}
}

func TestTimelineMessagesGolden(t *testing.T) {
	home := searchShowHome(t)
	code, out, errOut := run(t, "--home", home, "timeline", "--messages", "--project", "myapp")
	if code != 0 {
		t.Fatalf("timeline --messages exit = %d, stderr: %s", code, errOut)
	}
	s0 := mustUTC(t, "2026-06-15T09:00:00Z").Local().Format("2006-01-02 15:04:05")
	s1 := mustUTC(t, "2026-08-02T14:03:22Z").Local().Format("2006-01-02 15:04:05")
	s2 := mustUTC(t, "2026-08-02T14:03:25Z").Local().Format("2006-01-02 15:04:05")
	c1 := mustUTC(t, "2026-08-02T14:10:05Z").Local().Format("2006-01-02 15:04:05")
	c2 := mustUTC(t, "2026-08-02T14:10:10Z").Local().Format("2006-01-02 15:04:05")
	// aaaa9999 shares /home/dev/myapp; codex emits one record per output_text
	want := strings.Join([]string{
		s0 + "  claude-code  user       unrelated session about databases            aaaa9999",
		s1 + "  claude-code  user       the refresh token is stored in localStorage  aaaa1111",
		s2 + "  claude-code  assistant  fixing jwt refresh rotation now              aaaa1111",
		c1 + "  codex        assistant  the issue was the auth module                3f9c81a2",
		c1 + "  codex        assistant  we fixed jwt refresh rotation                3f9c81a2",
		c2 + "  codex        assistant  tests cover the café flow now                3f9c81a2",
		c2 + "  codex        assistant  jwt refresh is verified                      3f9c81a2",
		"",
	}, "\n")
	if out != want {
		t.Errorf("timeline --messages output:\n%s\nwant:\n%s", out, want)
	}
}

func TestTimelineMessagesMaxRowsKeepsNewest(t *testing.T) {
	home := searchShowHome(t)
	code, out, errOut := run(t, "--home", home, "timeline", "--messages", "--project", "myapp", "--max-rows", "2")
	if code != 0 {
		t.Fatalf("timeline --messages --max-rows exit = %d, stderr: %s", code, errOut)
	}
	if strings.Contains(out, "refresh token") || strings.Contains(out, "auth module") {
		t.Errorf("--max-rows 2 should keep the newest events only, got:\n%s", out)
	}
	if !strings.Contains(out, "café flow") || !strings.Contains(out, "jwt refresh is verified") {
		t.Errorf("--max-rows 2 should keep the two newest codex events, got:\n%s", out)
	}
}

func TestTimelineMessagesMasksSecrets(t *testing.T) {
	home := maskHome(t)
	code, out, errOut := run(t, "--home", home, "timeline", "--messages", "--project", "keys")
	if code != 0 {
		t.Fatalf("timeline --messages exit = %d, stderr: %s", code, errOut)
	}
	if strings.Contains(out, rawSecret) {
		t.Errorf("timeline events must be masked, got:\n%s", out)
	}
	if !strings.Contains(out, maskedSecret) {
		t.Errorf("timeline events should show the masked token, got:\n%s", out)
	}
}

func TestTimelineMessagesJSONVerbatim(t *testing.T) {
	home := maskHome(t)
	code, out, errOut := run(t, "--home", home, "timeline", "--messages", "--project", "keys", "--json")
	if code != 0 {
		t.Fatalf("timeline --messages --json exit = %d, stderr: %s", code, errOut)
	}
	var events []struct {
		Timestamp string `json:"timestamp"`
		Agent     string `json:"agent"`
		Role      string `json:"role"`
		Text      string `json:"text"`
		Session   string `json:"session"`
	}
	if err := json.Unmarshal([]byte(out), &events); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	e := events[0]
	if e.Agent != "claude-code" || e.Role != "user" || e.Session != "eeee5555-5555-4555-8555-555555555555" {
		t.Errorf("event = %+v", e)
	}
	if e.Timestamp != "2026-08-05T09:00:00Z" {
		t.Errorf("timestamp = %q, want the entry time not the session start", e.Timestamp)
	}
	if !strings.Contains(e.Text, rawSecret) {
		t.Errorf("--json is the machine channel and stays verbatim, got %q", e.Text)
	}
}

func TestTimelineSessionsJSON(t *testing.T) {
	home := searchShowHome(t)
	code, out, errOut := run(t, "--home", home, "timeline", "--json")
	if code != 0 {
		t.Fatalf("timeline --json exit = %d, stderr: %s", code, errOut)
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out)
	}
	if len(rows) != 5 {
		t.Fatalf("rows = %d, want 5 sessions", len(rows))
	}
	if rows[0].ID != "dddd4444-4444-4444-8444-444444444444" || rows[4].ID != "3f9c81a2-1111-4222-8333-cccccccccccc" {
		t.Errorf("rows should be oldest first, got %v … %v", rows[0].ID, rows[4].ID)
	}
}

func TestTimelineFlagErrors(t *testing.T) {
	home := searchShowHome(t)
	t.Run("negative max-rows", func(t *testing.T) {
		code, _, errOut := run(t, "--home", home, "timeline", "--messages", "--max-rows", "-1")
		if code != 2 || !strings.Contains(errOut, "invalid --max-rows value") {
			t.Errorf("exit = %d stderr = %q", code, errOut)
		}
	})
	t.Run("unknown agent", func(t *testing.T) {
		code, _, errOut := run(t, "--home", home, "timeline", "--agent", "nope")
		if code != 2 || !strings.Contains(errOut, `unknown agent "nope"`) {
			t.Errorf("exit = %d stderr = %q", code, errOut)
		}
	})
}
