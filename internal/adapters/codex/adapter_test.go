package codex

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// buildHome copies named fixtures from testdata/ into a synthetic home tree:
// <home>/.codex/sessions/YYYY/MM/DD/<file>. The adapter must never touch these
// source files; everything is copied into a temp dir first.
func buildHome(t *testing.T, files map[string]string) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex", "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	for rel, fixture := range files {
		dst := filepath.Join(home, ".codex", "sessions", rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		src, err := os.ReadFile(filepath.Join("testdata", fixture))
		if err != nil {
			t.Fatalf("fixture %s: %v", fixture, err)
		}
		if err := os.WriteFile(dst, src, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func newTestAdapter(t *testing.T, files map[string]string) *Adapter {
	t.Helper()
	return New(buildHome(t, files))
}

func listSessions(t *testing.T, a *Adapter) []agentlog.Session {
	t.Helper()
	var out []agentlog.Session
	err := a.Sessions(func(s agentlog.Session) error {
		out = append(out, s)
		return nil
	})
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	return out
}

func listMetas(t *testing.T, a *Adapter) []agentlog.SessionMeta {
	t.Helper()
	var out []agentlog.SessionMeta
	err := a.SessionsMeta(func(m agentlog.SessionMeta) error {
		out = append(out, m)
		return nil
	})
	if err != nil {
		t.Fatalf("SessionsMeta: %v", err)
	}
	return out
}

// realisticRel is a date dir + a rollout filename whose embedded uuid does NOT
// match the session_meta payload id, proving the payload id wins.
const realisticRel = "2026/08/02/rollout-2026-08-02T14-03-20-deadbeef-0000-4000-8000-000000000001.jsonl"

func TestDetect(t *testing.T) {
	home := t.TempDir()
	if New(home).Detect() {
		t.Fatal("Detect should be false without storage")
	}
	empty := newTestAdapter(t, nil)
	if !empty.Detect() {
		t.Fatal("Detect should be true when .codex/sessions exists")
	}
	if got := empty.StoragePath(); filepath.ToSlash(got) != filepath.ToSlash(filepath.Join(empty.home, ".codex", "sessions")) {
		t.Errorf("StoragePath = %q", got)
	}
}

func TestSessionsRealisticFixture(t *testing.T) {
	a := newTestAdapter(t, map[string]string{realisticRel: "realistic.jsonl"})
	if !a.Detect() {
		t.Fatal("storage should be detected")
	}
	metas := listMetas(t, a)
	if len(metas) != 1 {
		t.Fatalf("got %d sessions, want 1", len(metas))
	}
	s, messages := metas[0].Session, metas[0].Messages
	// the payload id must win over the uuid embedded in the filename
	if s.ID != "3f9c81a2-1111-4222-8333-cccccccccccc" {
		t.Errorf("ID = %q", s.ID)
	}
	if s.Agent != "codex" {
		t.Errorf("Agent = %q", s.Agent)
	}
	if s.Project != "/home/dev/myapp/api" {
		t.Errorf("Project = %q, want /home/dev/myapp/api", s.Project)
	}
	if s.Title != "the refresh token is stored in localStorage, is that safe?" {
		t.Errorf("Title = %q", s.Title)
	}
	wantStart := time.Date(2026, 8, 2, 14, 3, 20, 0, time.UTC)
	if !s.StartedAt.Equal(wantStart) {
		t.Errorf("StartedAt = %v, want %v (session_meta timestamp)", s.StartedAt, wantStart)
	}
	// timestamps of skipped records (event_msg etc.) must be ignored
	wantEnd := time.Date(2026, 8, 2, 14, 4, 5, 0, time.UTC)
	if !s.EndedAt.Equal(wantEnd) {
		t.Errorf("EndedAt = %v, want %v", s.EndedAt, wantEnd)
	}
	if messages != 4 { // 1 user + 2 assistant parts + 1 assistant
		t.Errorf("Messages = %d, want 4", messages)
	}
	info, err := os.Stat(filepath.Join(a.home, ".codex", "sessions", filepath.FromSlash(realisticRel)))
	if err != nil {
		t.Fatal(err)
	}
	if s.SizeBytes != info.Size() {
		t.Errorf("SizeBytes = %d, want %d", s.SizeBytes, info.Size())
	}
}

// TestNoTrailingNewlineFixtureIsWired pins the committed
// testdata/no-trailing-newline.jsonl fixture (M4-B10): a rollout file whose
// last line has no trailing newline parses end to end — the session is listed
// from its session_meta payload and the final message is not lost.
// TestSessionsMetaFastMatchesFullListing pins the search-flow fast listing
// contract (M4 fix round): ids, projects and start timestamps are identical
// to the full listing. A rollout whose first line is the session_meta payload
// is listed fast (beyond-line-1 fields zero); one whose first line is a later
// record falls back to the full parse and stays identical.
func TestSessionsMetaFastMatchesFullListing(t *testing.T) {
	home := t.TempDir()
	day := filepath.Join(home, ".codex", "sessions", "2026", "07", "01")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}
	fastContent := `{"timestamp":"2026-07-01T10:00:00Z","type":"session_meta","payload":{"id":"41414141-4141-4141-8141-414141414141","cwd":"/home/dev/app"}}` + "\n" +
		`{"timestamp":"2026-07-01T10:00:05Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}` + "\n"
	fallbackContent := `{"timestamp":"2026-07-01T11:00:05Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"late meta"}]}}` + "\n" +
		`{"timestamp":"2026-07-01T11:00:00Z","type":"session_meta","payload":{"id":"42424242-4242-4242-8242-424242424242","cwd":"/home/dev/app"}}` + "\n"
	for name, content := range map[string]string{
		"rollout-2026-07-01T10-00-00-41414141-4141-4141-8141-414141414141.jsonl": fastContent,     // sorted first
		"rollout-2026-07-01T11-00-00-42424242-4242-4242-8242-424242424242.jsonl": fallbackContent, // line 1 is not the meta
	} {
		if err := os.WriteFile(filepath.Join(day, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	list := func(a *Adapter, fast bool) []agentlog.SessionMeta {
		t.Helper()
		var out []agentlog.SessionMeta
		if fast {
			used, err := a.SessionsMetaFast(func(m agentlog.SessionMeta) error { out = append(out, m); return nil })
			if err != nil || !used {
				t.Fatalf("SessionsMetaFast: used=%v err=%v", used, err)
			}
		} else {
			if err := a.SessionsMeta(func(m agentlog.SessionMeta) error { out = append(out, m); return nil }); err != nil {
				t.Fatal(err)
			}
		}
		return out
	}

	fast, full := list(New(home), true), list(New(home), false)
	if len(fast) != 2 || len(full) != 2 {
		t.Fatalf("fast=%d full=%d sessions, want 2 each", len(fast), len(full))
	}
	for i := range fast {
		if fast[i].ID != full[i].ID || fast[i].Project != full[i].Project || !fast[i].StartedAt.Equal(full[i].StartedAt) {
			t.Errorf("session[%d]: fast %+v differs from full %+v in id/project/startedAt", i, fast[i], full[i])
		}
	}
	if fast[0].Messages != 0 || !fast[0].EndedAt.IsZero() || fast[0].Title != "" {
		t.Errorf("fast meta for a meta-first rollout should carry no beyond-line-1 fields, got %+v", fast[0].Session)
	}
	if full[0].Messages != 1 || full[0].Title != "hello" {
		t.Errorf("full meta = %+v, want 1 message and the first-user-message title", full[0].Session)
	}
	if fast[1].Messages != 1 || fast[1].Title != "late meta" || fast[1].EndedAt.IsZero() {
		t.Errorf("fallback rollout must produce the full meta on the fast path too, got %+v", fast[1].Session)
	}
}

func TestNoTrailingNewlineFixtureIsWired(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"2026/07/01/rollout-2026-07-01T10-00-00-aaa2b3c4-0000-4000-8000-000000000002.jsonl": "no-trailing-newline.jsonl",
	})
	metas := listMetas(t, a)
	if len(metas) != 1 {
		t.Fatalf("got %d sessions, want 1", len(metas))
	}
	if metas[0].ID != "aaa2b3c4-0000-4000-8000-000000000002" {
		t.Errorf("ID = %q, want the session_meta payload id", metas[0].ID)
	}
	if metas[0].Messages != 1 {
		t.Errorf("Messages = %d, want 1", metas[0].Messages)
	}
	var texts []string
	err := a.Entries(metas[0].Session, func(e agentlog.Entry) error { texts = append(texts, e.Text); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) != 1 || texts[0] != "hello without newline" {
		t.Errorf("entries = %v, want the last line's message (no trailing newline)", texts)
	}
}

// TestSessionsGlobMetacharacterHome ports the claudecode lister regression
// (final review, M4 fix): the codex lister must survive a home path containing
// glob metacharacters — filepath.Glob silently matches nothing there — for
// both listing and per-session file resolution.
func TestSessionsGlobMetacharacterHome(t *testing.T) {
	// Windows forbids * and ? in real filenames, but [ and ] are legal and
	// are exactly the metacharacters that turn a Glob pattern into a broken
	// character class.
	home := filepath.Join(t.TempDir(), "we[ird]home")
	day := filepath.Join(home, ".codex", "sessions", "2026", "07", "01")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"timestamp":"2026-07-01T10:00:00Z","type":"session_meta","payload":{"id":"88888888-8888-4888-8888-888888888888","cwd":"/home/dev/app"}}` + "\n" +
		`{"timestamp":"2026-07-01T10:00:05Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"one"}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(day, "rollout-2026-07-01T10-00-00-88888888-8888-4888-8888-888888888888.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New(home)
	got := listSessions(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1 (lister must survive glob metacharacters)", len(got))
	}
	var texts []string
	err := a.Entries(got[0], func(e agentlog.Entry) error { texts = append(texts, e.Text); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) != 1 || texts[0] != "one" {
		t.Errorf("Entries should resolve the rollout inside a metacharacter home, got %v", texts)
	}
}

// TestScanFileIOErrorNotCountedAsSkipped pins the I/O branch of
// scanner.Err() (aligned with claudecode; final review finding 3): a read
// failure makes the file unreadable (skipped silently, like any unreadable
// file) — it must not inflate the unreadable-content counter, and it must
// never be fatal. Only bufio.ErrTooLong (an oversized line) counts.
func TestScanFileIOErrorNotCountedAsSkipped(t *testing.T) {
	a := newTestAdapter(t, nil)
	// a directory named rollout-*.jsonl: os.Open succeeds, reading fails with
	// an I/O error that is not bufio.ErrTooLong
	dirFile := filepath.Join(a.home, ".codex", "sessions", "2026", "07", "01", "rollout-iam-a-directory.jsonl")
	if err := os.MkdirAll(dirFile, 0o755); err != nil {
		t.Fatal(err)
	}
	sum, err := a.scanFile(dirFile, nil, nil)
	if err != nil {
		t.Fatalf("I/O failures stay non-fatal, got %v", err)
	}
	if sum.sawLine {
		t.Error("no line can be read from a directory")
	}
	if a.SkippedLines() != 0 {
		t.Errorf("SkippedLines = %d, want 0 (I/O errors are unreadable files, not unreadable lines)", a.SkippedLines())
	}
}

func TestSessionsIDFallbackFromFilename(t *testing.T) {
	// no session_meta record: the ID must fall back to the rollout filename
	a := newTestAdapter(t, map[string]string{
		"2026/07/01/rollout-2026-07-01T09-00-00-aaa2b3c4-0000-4000-8000-000000000002.jsonl": "truncated.jsonl",
	})
	got := listSessions(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	if got[0].ID != "rollout-2026-07-01T09-00-00-aaa2b3c4-0000-4000-8000-000000000002" {
		t.Errorf("ID = %q, want filename-based id", got[0].ID)
	}
	if got[0].Project != "" {
		t.Errorf("Project = %q, want empty without session_meta", got[0].Project)
	}
}

func TestTitleTruncatedFromFirstUserMessage(t *testing.T) {
	long := strings.Repeat("word ", 30) // 150 runes, no newline
	content := `{"timestamp":"2026-08-02T10:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + long + `"}]}}` + "\n" +
		`{"timestamp":"2026-08-02T10:00:05Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}}` + "\n"
	a := newTestAdapter(t, nil)
	dir := filepath.Join(a.home, ".codex", "sessions", "2026", "08", "02")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rollout-2026-08-02T10-00-00-11111111-1111-4111-8111-111111111111.jsonl"),
		[]byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := listSessions(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	runes := []rune(got[0].Title)
	if len(runes) != 80 || !strings.HasSuffix(got[0].Title, "…") {
		t.Errorf("Title should be 80 runes ending with …, got %d runes: %.90q", len(runes), got[0].Title)
	}
}

func TestTitleCollapsesNewlines(t *testing.T) {
	content := `{"timestamp":"2026-08-02T10:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"fix the bug\n\nin auth"}]}}` + "\n"
	a := newTestAdapter(t, nil)
	dir := filepath.Join(a.home, ".codex", "sessions", "2026", "08", "02")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rollout-2026-08-02T10-00-00-22222222-2222-4222-8222-222222222222.jsonl"),
		[]byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := listSessions(t, a)
	if got[0].Title != "fix the bug in auth" {
		t.Errorf("Title = %q, want single line", got[0].Title)
	}
}

func TestEntriesStreamRealistic(t *testing.T) {
	a := newTestAdapter(t, map[string]string{realisticRel: "realistic.jsonl"})
	sessions := listSessions(t, a)
	var got []agentlog.Entry
	err := a.Entries(sessions[0], func(e agentlog.Entry) error {
		got = append(got, e)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	type want struct {
		kind agentlog.EntryKind
		role string
		text string
	}
	wants := []want{
		{agentlog.Message, "user", "the refresh token is stored in localStorage, is that safe?"},
		{agentlog.Summary, "", "Consider where the refresh token is stored and how it rotates."},
		{agentlog.Message, "assistant", "No — localStorage is readable by any script on the page."},
		{agentlog.Message, "assistant", "Let me move it to an httpOnly cookie."},
		{agentlog.ToolCall, "assistant", `{"command":["grep","-rn","localStorage","/home/dev/myapp/api/src"]}`},
		{agentlog.ToolResult, "tool", `src/auth.ts:12: localStorage.setItem('refresh', token)`},
		{agentlog.Message, "assistant", "we moved it to an httpOnly cookie and rotate on every use"},
	}
	if len(got) != len(wants) {
		for i, e := range got {
			t.Logf("got[%d] kind=%d role=%q text=%.60q", i, e.Kind, e.Role, e.Text)
		}
		t.Fatalf("got %d entries, want %d", len(got), len(wants))
	}
	for i, w := range wants {
		if got[i].Kind != w.kind || got[i].Role != w.role {
			t.Errorf("entry[%d] = kind %d role %q, want kind %d role %q", i, got[i].Kind, got[i].Role, w.kind, w.role)
		}
		if got[i].Text != w.text {
			t.Errorf("entry[%d].Text = %.80q, want %.80q", i, got[i].Text, w.text)
		}
	}
	if got[0].Timestamp.IsZero() {
		t.Error("message entries should carry the record timestamp")
	}
}

func TestEntriesFindsSessionByMetaID(t *testing.T) {
	// filename deliberately differs from the session_meta payload id
	a := newTestAdapter(t, map[string]string{realisticRel: "realistic.jsonl"})
	s := agentlog.Session{ID: "3f9c81a2-1111-4222-8333-cccccccccccc", Agent: "codex"}
	n := 0
	err := a.Entries(s, func(e agentlog.Entry) error { n++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Error("Entries should locate the file by session_meta payload id")
	}
}

func TestEntriesUnknownSessionErrors(t *testing.T) {
	a := newTestAdapter(t, nil)
	err := a.Entries(agentlog.Session{ID: "nope"}, func(e agentlog.Entry) error { return nil })
	if err == nil {
		t.Fatal("expected error for unknown session")
	}
	if msg := err.Error(); strings.ContainsAny(msg[:1], "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		t.Errorf("error should be lowercase: %q", msg)
	}
}

func TestDefensiveParsingCountsSkipped(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"2026/07/02/rollout-2026-07-02T11-00-00-44444444-4444-4444-8444-444444444444.jsonl": "truncated.jsonl",
	})
	metas := listMetas(t, a)
	if len(metas) != 1 {
		t.Fatalf("got %d sessions, want 1 (valid lines still work)", len(metas))
	}
	if metas[0].Messages != 1 {
		t.Errorf("Messages = %d, want 1", metas[0].Messages)
	}
	if a.SkippedLines() != 1 {
		t.Errorf("SkippedLines = %d, want 1 (truncated json line)", a.SkippedLines())
	}
}

func TestUnknownShapesCountedAsSkipped(t *testing.T) {
	// realistic.jsonl carries event_msg and an unknown response_item payload
	// type — both are skipped and counted. turn_context is NOT counted
	// anymore: M7 moved it to recognized-silent (controller ruling 4), so
	// this expectation legitimately changed from 3 to 2.
	a := newTestAdapter(t, map[string]string{realisticRel: "realistic.jsonl"})
	_ = listSessions(t, a)
	if a.SkippedLines() != 2 {
		t.Errorf("SkippedLines = %d, want 2 (event_msg, web_search_call; turn_context is recognized-silent since M7)", a.SkippedLines())
	}
}

func TestSkippedLinesAccumulateAcrossScans(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"2026/07/02/rollout-2026-07-02T11-00-00-44444444-4444-4444-8444-444444444444.jsonl": "truncated.jsonl",
	})
	_ = listSessions(t, a)
	_ = listSessions(t, a)
	if a.SkippedLines() != 2 {
		t.Errorf("SkippedLines = %d, want 2 after two scans", a.SkippedLines())
	}
}

func TestSessionsEmptyFileYieldsNothing(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"2026/07/01/rollout-2026-07-01T09-00-00-33333333-3333-4333-8333-333333333333.jsonl": "empty.jsonl",
	})
	if got := listSessions(t, a); len(got) != 0 {
		t.Errorf("empty file should not yield a session, got %d", len(got))
	}
}

func TestNonRolloutFilesIgnored(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"2026/07/01/rollout-2026-07-01T09-00-00-11111111-1111-4111-8111-111111111111.jsonl": "realistic.jsonl",
		"2026/07/01/notes.txt": "realistic.jsonl", // non-rollout must be ignored
	})
	if got := listSessions(t, a); len(got) != 1 {
		t.Errorf("got %d sessions, want 1 (non-rollout files ignored)", len(got))
	}
}

func TestLongLineOver64KB(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"2026/07/05/rollout-2026-07-05T12-00-00-55555555-5555-4555-8555-555555555555.jsonl": "long-line.jsonl",
	})
	metas := listMetas(t, a)
	if len(metas) != 1 {
		t.Fatalf("got %d sessions, want 1", len(metas))
	}
	if metas[0].Messages != 1 {
		t.Errorf("Messages = %d, want 1", metas[0].Messages)
	}
	if metas[0].SizeBytes < 64*1024 {
		t.Errorf("fixture too small: %d bytes", metas[0].SizeBytes)
	}
	// the long content must survive into Entries
	var texts []string
	err := a.Entries(metas[0].Session, func(e agentlog.Entry) error {
		texts = append(texts, e.Text)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tx := range texts {
		if len(tx) > 64*1024 {
			found = true
		}
	}
	if !found {
		t.Error("no entry carried the >64KB text")
	}
}

func TestUnicodeContent(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"2026/07/03/rollout-2026-07-03T09-15-00-66666666-6666-4666-8666-666666666666.jsonl": "unicode.jsonl",
	})
	got := listSessions(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	var texts []string
	_ = a.Entries(got[0], func(e agentlog.Entry) error { texts = append(texts, e.Text); return nil })
	joined := strings.Join(texts, "\n")
	for _, want := range []string{"こんにちは", "🌍", "café", "naïve", "журнал"} {
		if !strings.Contains(joined, want) {
			t.Errorf("unicode content lost: %q not in entries", want)
		}
	}
}

func TestNeverPanicsOnGarbage(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".codex", "sessions", "2026", "07", "01")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	garbage := []string{
		"{",
		"[]",
		"null",
		"\"string\"",
		"123",
		`{"type":123}`,
		`{"type":"session_meta"}`,              // known type, no payload
		`{"type":"session_meta","payload":42}`, // payload not an object
		`{"type":"response_item","payload":null}`, // payload missing
		`{"type":"response_item","payload":{"type":"message","content":[{"type":42}]}}`,
		`{"type":"response_item","payload":{"type":"message","content":"plain string"}}`, // tolerated: string content
		`{"type":"response_item","payload":{"type":"reasoning","summary":[{"type":42}]}}`,
		"\x00\x01\x02binary",
	}
	var b strings.Builder
	for _, g := range garbage {
		fmt.Fprintf(&b, "%s\n", g)
	}
	if err := os.WriteFile(filepath.Join(dir, "rollout-2026-07-01T09-00-00-77777777-7777-4777-8777-777777777777.jsonl"),
		[]byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New(home)
	metas := listMetas(t, a)
	// the "plain string" message content is tolerated as one text part
	if len(metas) != 1 || metas[0].Messages != 1 {
		t.Errorf("got %+v, want 1 session with 1 message", metas)
	}
	if a.SkippedLines() != 12 {
		t.Errorf("SkippedLines = %d, want 12", a.SkippedLines())
	}
}

func TestIterEarlyStop(t *testing.T) {
	a := newTestAdapter(t, map[string]string{realisticRel: "realistic.jsonl"})
	sentinel := errors.New("stop")
	err := a.Sessions(func(s agentlog.Session) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Errorf("Sessions should propagate iter error, got %v", err)
	}
	err = a.Entries(agentlog.Session{ID: "3f9c81a2-1111-4222-8333-cccccccccccc"}, func(e agentlog.Entry) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("Entries should propagate iter error, got %v", err)
	}
	err = a.EntriesFiltered(agentlog.Session{ID: "3f9c81a2-1111-4222-8333-cccccccccccc"}, nil,
		func(e agentlog.Entry) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Errorf("EntriesFiltered(nil keep) should behave like Entries, got %v", err)
	}
}

// TestEntriesFilteredPrefilter verifies the search hot-path hook: lines the
// prefilter rejects are never parsed (a garbage line rejected by keep must not
// increment the skipped counter) and matching lines still yield entries.
func TestEntriesFilteredPrefilter(t *testing.T) {
	a := newTestAdapter(t, map[string]string{realisticRel: "realistic.jsonl"})
	s := agentlog.Session{ID: "3f9c81a2-1111-4222-8333-cccccccccccc"}

	keepNone := func(line []byte) bool { return false }
	n := 0
	err := a.EntriesFiltered(s, keepNone, func(e agentlog.Entry) error { n++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("keep=false should yield no entries, got %d", n)
	}
	if a.SkippedLines() != 0 {
		t.Errorf("prefilter-rejected lines must not count as skipped, got %d", a.SkippedLines())
	}

	keepLocal := func(line []byte) bool { return strings.Contains(string(line), "localStorage") }
	var texts []string
	err = a.EntriesFiltered(s, keepLocal, func(e agentlog.Entry) error { texts = append(texts, e.Text); return nil })
	if err != nil {
		t.Fatal(err)
	}
	// the prefilter works at line granularity: entries on kept lines are
	// yielded, lines without the needle are never parsed at all
	joined := strings.Join(texts, "\n")
	if !strings.Contains(joined, "the refresh token is stored in localStorage") {
		t.Error("kept line's entries are missing from the result")
	}
	if strings.Contains(joined, "rotate on every use") {
		t.Error("entries from prefilter-rejected lines must not be parsed")
	}
}
