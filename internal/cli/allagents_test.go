package cli

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pastlog/pastlog/internal/adapters/opencode"
	_ "modernc.org/sqlite"
)

// Fixtures for the all-agents integration tests: one session per agent
// sharing the needle "calibration", plus per-agent extra sessions so filters
// and show stay exercised across adapters (M3 acceptance).
const (
	allClaudeContent = `{"type":"user","sessionId":"ccaa1111-1111-4111-8111-111111111111","cwd":"/home/dev/cal","timestamp":"2026-08-02T14:03:22Z","message":{"role":"user","content":"check the calibration log"}}`
	allCodexContent  = `{"timestamp":"2026-08-02T14:10:00Z","type":"session_meta","payload":{"id":"ccbb2222-2222-4222-8222-222222222222","cwd":"/home/dev/cal"}}
{"timestamp":"2026-08-02T14:10:05Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"calibration drift confirmed"}]}}`

	allGeminiContent = `{"sessionId":"gccc3333-3333-4333-8333-333333333333","startTime":"2026-08-02T14:20:00Z","lastUpdated":"2026-08-02T14:21:00Z","kind":"main","directories":["/home/dev/cal"],"summary":"calibration review"}
{"id":"gm1","timestamp":"2026-08-02T14:20:10Z","type":"user","content":"review the calibration constants"}`
)

// allAgentsHome builds a synthetic home with data for all four agents:
// claude-code and codex JSONL, a gemini-cli JSONL session, and an opencode
// database generated into <home>/.local/share/opencode (the storage root
// discovery falls back to once the per-OS env override is neutralized).
func allAgentsHome(t *testing.T) string {
	t.Helper()
	isolateDataHome(t)
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
	write(filepath.Join(".claude", "projects", "C--Users-dev-cal", "ccaa1111-1111-4111-8111-111111111111.jsonl"), allClaudeContent)
	write(filepath.Join(".codex", "sessions", "2026", "08", "02",
		"rollout-2026-08-02T14-10-00-ccbb2222-2222-4222-8222-222222222222.jsonl"), allCodexContent)
	write(filepath.Join(".gemini", "tmp", "ca11bad5foo", "chats", "session-2026-08-02T14-20-gccc3333.jsonl"), allGeminiContent)

	if _, err := opencode.GenerateTestDB(filepath.Join(home, ".local", "share", "opencode")); err != nil {
		t.Fatalf("GenerateTestDB: %v", err)
	}
	return home
}

func TestSessionsAllAgents(t *testing.T) {
	home := allAgentsHome(t)
	code, out, errOut := run(t, "--home", home, "sessions", "--json")
	if code != 0 {
		t.Fatalf("sessions exit = %d, stderr: %s", code, errOut)
	}
	for _, want := range []string{
		`"agent": "claude-code"`,
		`"agent": "codex"`,
		`"agent": "gemini-cli"`,
		`"agent": "opencode"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("sessions --json should include %s, got:\n%s", want, out)
		}
	}
}

func TestSearchAllAgents(t *testing.T) {
	home := allAgentsHome(t)
	code, out, errOut := run(t, "--home", home, "search", "calibration", "--json")
	if code != 0 {
		t.Fatalf("search exit = %d, stderr: %s", code, errOut)
	}
	for _, want := range []string{
		`"agent": "claude-code"`,
		`"agent": "codex"`,
		`"agent": "gemini-cli"`,
		`"agent": "opencode"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("search should hit %s, got:\n%s", want, out)
		}
	}
	// the opencode hit carries the generated fixture text
	if !strings.Contains(out, "calibration keeps drifting") {
		t.Errorf("search should surface the opencode fixture text, got:\n%s", out)
	}
}

func TestShowAllAgents(t *testing.T) {
	home := allAgentsHome(t)
	t.Run("opencode by id prefix", func(t *testing.T) {
		code, out, errOut := run(t, "--home", home, "show", "ses_fixture100001")
		if code != 0 {
			t.Fatalf("show exit = %d, stderr: %s", code, errOut)
		}
		if !strings.Contains(out, "recalibrating the flux capacitor") {
			t.Errorf("show should print the opencode transcript, got:\n%s", out)
		}
	})
	t.Run("gemini-cli by id prefix", func(t *testing.T) {
		code, out, errOut := run(t, "--home", home, "show", "gccc3333")
		if code != 0 {
			t.Fatalf("show exit = %d, stderr: %s", code, errOut)
		}
		if !strings.Contains(out, "review the calibration constants") {
			t.Errorf("show should print the gemini transcript, got:\n%s", out)
		}
	})
}

// TestAgentsOpencodeTotals pins the controller ruling for the opencode
// `agents` row: the size column is the database footprint (db + wal + shm),
// not the sum of session sizes.
func TestAgentsOpencodeTotals(t *testing.T) {
	home := allAgentsHome(t)
	code, out, errOut := run(t, "--home", home, "agents")
	if code != 0 {
		t.Fatalf("agents exit = %d, stderr: %s", code, errOut)
	}
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "opencode") {
			// the generated db is a few KiB — human bytes end in "KB"
			if !strings.Contains(line, "sessions") || !strings.Contains(line, "KB") {
				t.Errorf("opencode agents row should show the db footprint, got %q", line)
			}
		}
	}
	if !strings.Contains(out, "~/.local/share/opencode") {
		t.Errorf("opencode row should show the tilde-shortened storage root, got:\n%s", out)
	}
}

// TestAgentsLockedDBWarnsOnce is the spec §4 fallback end-to-end: a second
// connection holds a write lock, `agents` prints ONE warning on stderr, keeps
// listing the other agents and exits 0.
func TestAgentsLockedDBWarnsOnce(t *testing.T) {
	home := allAgentsHome(t)
	release := lockDB(t, filepath.Join(home, ".local", "share", "opencode", "opencode.db"))
	defer release()

	code, out, errOut := run(t, "--home", home, "agents")
	if code != 0 {
		t.Fatalf("agents exit = %d with locked opencode, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "claude-code  1 session") {
		t.Errorf("other agents must be unaffected, got:\n%s", out)
	}
	if n := strings.Count(errOut, "opencode: database is locked"); n != 1 {
		t.Errorf("exactly one locked-DB warning expected, got %d in %q", n, errOut)
	}
}

// TestSearchLockedDBExitCode pins that a locked opencode database neither
// breaks search results from other agents (exit 0 with hits) nor turns a
// miss into an error (still the grep-style exit 1).
func TestSearchLockedDBExitCode(t *testing.T) {
	home := allAgentsHome(t)
	release := lockDB(t, filepath.Join(home, ".local", "share", "opencode", "opencode.db"))
	defer release()

	t.Run("hits from other agents still exit 0", func(t *testing.T) {
		code, out, errOut := run(t, "--home", home, "search", "calibration")
		if code != 0 {
			t.Fatalf("search exit = %d, stderr: %s", code, errOut)
		}
		if !strings.Contains(out, "claude-code") {
			t.Errorf("claude-code hits expected, got:\n%s", out)
		}
		if n := strings.Count(errOut, "opencode: database is locked"); n != 1 {
			t.Errorf("exactly one locked-DB warning expected, got %d in %q", n, errOut)
		}
	})
	t.Run("no matches still exits 1", func(t *testing.T) {
		code, _, errOut := run(t, "--home", home, "search", "xyzzynothing")
		if code != 1 {
			t.Fatalf("search exit = %d, want 1 (no matches)", code)
		}
		if n := strings.Count(errOut, "opencode: database is locked"); n != 1 {
			t.Errorf("exactly one locked-DB warning expected, got %d in %q", n, errOut)
		}
	})
}

// lockDB holds an exclusive write lock on the database file for the duration
// of the test, mimicking a running OpenCode instance.
func lockDB(t *testing.T, path string) func() {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("lock connection: %v", err)
	}
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("lock conn: %v", err)
	}
	ctx := context.Background()
	if _, err := conn.ExecContext(ctx, "BEGIN EXCLUSIVE"); err != nil {
		t.Fatalf("BEGIN EXCLUSIVE: %v", err)
	}
	// sanity: the lock must actually block another connection's reads
	probe, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro&_pragma=busy_timeout(1)")
	if err != nil {
		t.Fatalf("probe connection: %v", err)
	}
	defer probe.Close()
	if _, err := probe.Query("SELECT count(*) FROM session"); err == nil {
		t.Fatal("expected the probe read to fail while the lock is held")
	}
	return func() {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		_ = conn.Close()
		_ = db.Close()
	}
}
