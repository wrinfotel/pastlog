package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// corruptOpencodeDB writes a garbage file where the opencode database would
// live: the adapter detects storage, then fails to read it — a deterministic,
// portable "storage unreadable" trigger for the stderr-note tests (M4-B1..B4).
func corruptOpencodeDB(t *testing.T, home string) {
	t.Helper()
	p := filepath.Join(home, ".local", "share", "opencode")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "opencode.db"), []byte("this is not a database"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSessionsNoteUnreadableStorage pins M4-B1: a failing adapter must not
// render as a silent empty result — one lowercase stderr note distinguishes
// "storage unreadable" from "no sessions", while other agents keep working
// and the exit code stays 0 (spec §8 best effort).
func TestSessionsNoteUnreadableStorage(t *testing.T) {
	home := fixtureHome(t)
	corruptOpencodeDB(t, home)

	code, out, errOut := run(t, "--home", home, "sessions", "--json")
	if code != 0 {
		t.Fatalf("sessions exit = %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "aaaa1111") {
		t.Errorf("other agents must be unaffected, got:\n%s", out)
	}
	if !strings.Contains(errOut, "opencode: storage unreadable") {
		t.Errorf("stderr should note the unreadable storage, got %q", errOut)
	}
	if strings.Count(errOut, "opencode: storage unreadable") != 1 {
		t.Errorf("exactly one note per failing adapter expected, got %q", errOut)
	}
}

// TestSearchNoteUnreadableStorage pins M4-B2: the search engine notes a
// failing adapter once on stderr instead of silently searching a partial
// corpus.
func TestSearchNoteUnreadableStorage(t *testing.T) {
	home := searchShowHome(t)
	corruptOpencodeDB(t, home)

	code, out, errOut := run(t, "--home", home, "search", "jwt refresh")
	if code != 0 {
		t.Fatalf("search exit = %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "aaaa1111") {
		t.Errorf("hits from other agents must be unaffected, got:\n%s", out)
	}
	if strings.Count(errOut, "opencode: storage unreadable") != 1 {
		t.Errorf("exactly one note per failing adapter expected, got %q", errOut)
	}
}

// TestShowNoMatchDistinguishesUnreadableStorage pins M4-B3: when an adapter's
// storage fails while resolving a session, the not-found message must say so
// instead of claiming a plain miss (exit stays 2).
func TestShowNoMatchDistinguishesUnreadableStorage(t *testing.T) {
	home := searchShowHome(t)
	corruptOpencodeDB(t, home)

	code, _, errOut := run(t, "--home", home, "show", "zzzz")
	if code != 2 {
		t.Fatalf("show exit = %d, want 2", code)
	}
	if !strings.Contains(errOut, `no session matches id prefix "zzzz"`) {
		t.Errorf("stderr should keep the not-found message, got %q", errOut)
	}
	if !strings.Contains(errOut, "storage was unreadable") || !strings.Contains(errOut, "opencode") {
		t.Errorf("stderr should name the unreadable storage, got %q", errOut)
	}
}

// TestShowLockedDBWarnsOnNotFoundPath pins M4-B16: a locked opencode database
// makes `show <opencode-prefix>` fail with the not-found message, and the
// locked-DB warning must be printed on that error path too (it used to run
// only on the success path).
func TestShowLockedDBWarnsOnNotFoundPath(t *testing.T) {
	home := allAgentsHome(t)
	release := lockDB(t, filepath.Join(home, ".local", "share", "opencode", "opencode.db"))
	defer release()

	code, _, errOut := run(t, "--home", home, "show", "ses_fixture100001")
	if code != 2 {
		t.Fatalf("show exit = %d, want 2", code)
	}
	if !strings.Contains(errOut, `no session matches id prefix "ses_fixture100001"`) {
		t.Errorf("stderr should keep the not-found message, got %q", errOut)
	}
	if !strings.Contains(errOut, "opencode: database is locked") {
		t.Errorf("stderr should print the locked-DB warning on the failure path, got %q", errOut)
	}
}

// TestBareInvocationNotesStderr pins M4-B4: a bare `pastlog` routes adapter
// conditions through the same stderr summarizer as agents/sessions/search.
func TestBareInvocationNotesStderr(t *testing.T) {
	home := fixtureHome(t)
	corruptOpencodeDB(t, home)

	code, out, errOut := run(t, "--home", home)
	if code != 0 {
		t.Fatalf("bare exit = %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "claude-code: 2 sessions") {
		t.Errorf("bare invocation should summarize detected agents, got:\n%s", out)
	}
	if !strings.Contains(errOut, "opencode: storage unreadable") {
		t.Errorf("bare invocation should note unreadable storage on stderr, got %q", errOut)
	}
}

// TestAgentsNoteUnreadableStorage pins the same note for `agents`: the
// opencode row stays (storage detected), and stderr explains why its session
// count is empty.
func TestAgentsNoteUnreadableStorage(t *testing.T) {
	home := fixtureHome(t)
	corruptOpencodeDB(t, home)

	code, out, errOut := run(t, "--home", home, "agents")
	if code != 0 {
		t.Fatalf("agents exit = %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "opencode") {
		t.Errorf("opencode row should stay listed (storage detected), got:\n%s", out)
	}
	if !strings.Contains(errOut, "opencode: storage unreadable") {
		t.Errorf("stderr should note the unreadable storage, got %q", errOut)
	}
}
