package claudecode

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pastlog/pastlog/internal/agentlog"
)

// buildHome copies named fixtures from testdata/ into a synthetic home tree:
// <home>/.claude/projects/<dir>/<file>. The adapter must never touch these
// source files; everything is copied into a temp dir first.
func buildHome(t *testing.T, files map[string]string) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude", "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	for rel, fixture := range files {
		dst := filepath.Join(home, ".claude", "projects", rel)
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

func TestDetect(t *testing.T) {
	home := t.TempDir()
	if New(home).Detect() {
		t.Fatal("Detect should be false without storage")
	}
	empty := newTestAdapter(t, nil)
	if !empty.Detect() {
		t.Fatal("Detect should be true when .claude/projects exists")
	}
}

func TestSessionsRealisticFixture(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"weird-escaped-name/3f9c81a2-1111-4222-8333-cccccccccccc.jsonl": "realistic.jsonl",
	})
	if !a.Detect() {
		t.Fatal("storage should be detected")
	}
	metas := listMetas(t, a)
	if len(metas) != 1 {
		t.Fatalf("got %d sessions, want 1", len(metas))
	}
	s, messages := metas[0].Session, metas[0].Messages
	if s.ID != "3f9c81a2-1111-4222-8333-cccccccccccc" {
		t.Errorf("ID = %q", s.ID)
	}
	if s.Agent != "claude-code" {
		t.Errorf("Agent = %q", s.Agent)
	}
	// project cwd comes from the records' cwd field, never from the dir name.
	if s.Project != "/home/dev/myapp" {
		t.Errorf("Project = %q, want /home/dev/myapp", s.Project)
	}
	if s.Title != "Fix jwt refresh token rotation" {
		t.Errorf("Title = %q", s.Title)
	}
	wantStart := time.Date(2026, 8, 2, 14, 3, 22, int(150*time.Millisecond), time.UTC)
	if !s.StartedAt.Equal(wantStart) {
		t.Errorf("StartedAt = %v, want %v", s.StartedAt, wantStart)
	}
	// the trailing unknown-type record's timestamp must be ignored
	wantEnd := time.Date(2026, 8, 2, 14, 4, 5, 0, time.UTC)
	if !s.EndedAt.Equal(wantEnd) {
		t.Errorf("EndedAt = %v, want %v", s.EndedAt, wantEnd)
	}
	if messages != 4 { // 2 user strings + 1 assistant text + 1 system
		t.Errorf("Messages = %d, want 4", messages)
	}
	info, err := os.Stat(filepath.Join(a.home, ".claude", "projects",
		"weird-escaped-name", "3f9c81a2-1111-4222-8333-cccccccccccc.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if s.SizeBytes != info.Size() {
		t.Errorf("SizeBytes = %d, want %d", s.SizeBytes, info.Size())
	}
}

func TestSessionsIDFallbackFromFilename(t *testing.T) {
	// records carry no sessionId field; the ID must fall back to the filename,
	// and the last line has no trailing newline (spec §9 fixture behavior)
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects", "proj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"type":"user","cwd":"/home/dev/app","timestamp":"2026-07-01T10:00:00Z","message":{"role":"user","content":"first"}}` + "\n" +
		`{"type":"assistant","cwd":"/home/dev/app","timestamp":"2026-07-01T10:00:05Z","message":{"role":"assistant","content":"last"}}`
	if err := os.WriteFile(filepath.Join(dir, "aaa2b3c4-0000-4000-8000-000000000002.jsonl"),
		[]byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New(home)
	got := listSessions(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	if got[0].ID != "aaa2b3c4-0000-4000-8000-000000000002" {
		t.Errorf("ID = %q, want filename-based id", got[0].ID)
	}
	wantEnd := time.Date(2026, 7, 1, 10, 0, 5, 0, time.UTC)
	if !got[0].EndedAt.Equal(wantEnd) {
		t.Errorf("EndedAt = %v, want %v (last line has no trailing newline)", got[0].EndedAt, wantEnd)
	}
}

func TestSessionsMultipleProjects(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"proj-a/11111111-1111-4111-8111-111111111111.jsonl": "realistic.jsonl",
		"proj-b/22222222-2222-4222-8222-222222222222.jsonl": "no-trailing-newline.jsonl",
		"proj-b/notes.txt": "realistic.jsonl", // non-jsonl must be ignored
	})
	got := listSessions(t, a)
	if len(got) != 2 {
		t.Fatalf("got %d sessions, want 2 (non-jsonl ignored)", len(got))
	}
}

func TestSessionsEmptyFileYieldsNothing(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"proj/33333333-3333-4333-8333-333333333333.jsonl": "empty.jsonl",
	})
	if got := listSessions(t, a); len(got) != 0 {
		t.Errorf("empty file should not yield a session, got %d", len(got))
	}
}

// TestSessionsGlobMetacharacterHome pins the M4 lister fix: filepath.Glob
// silently matches nothing when the home path contains glob metacharacters
// ([, *, ?); the WalkDir lister must be immune, for both listing and
// per-session file resolution.
func TestSessionsGlobMetacharacterHome(t *testing.T) {
	// Windows forbids * and ? in real filenames, but [ and ] are legal and
	// are exactly the metacharacters that turn a Glob pattern into a broken
	// character class.
	home := filepath.Join(t.TempDir(), "we[ird]home")
	dir := filepath.Join(home, ".claude", "projects", "pr[o]j")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"type":"user","sessionId":"88888888-8888-4888-8888-888888888888","cwd":"/home/dev/app","timestamp":"2026-07-01T10:00:00Z","message":{"role":"user","content":"one"}}`
	if err := os.WriteFile(filepath.Join(dir, "88888888-8888-4888-8888-888888888888.jsonl"), []byte(content), 0o644); err != nil {
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
		t.Errorf("Entries should resolve the file inside a metacharacter home, got %v", texts)
	}
}

// TestSessionFilesOnlyProjectDirJSONL pins the documented scope (SCHEMA.md):
// only *.jsonl files directly inside project directories are sessions — files
// directly under projects/ or nested deeper are ignored, exactly like the old
// Glob pattern projects/*/*.jsonl.
func TestSessionFilesOnlyProjectDirJSONL(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"proj/11111111-1111-4111-8111-111111111111.jsonl": "realistic.jsonl",
	})
	// a file directly under projects/ and one nested two levels deep must not
	// become sessions
	for _, rel := range []string{
		filepath.Join("stray.jsonl"),
		filepath.Join("proj", "nested", "deeper.jsonl"),
	} {
		dst := filepath.Join(a.home, ".claude", "projects", rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, []byte(`{"type":"user","sessionId":"x","cwd":"/p","message":{"role":"user","content":"s"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := listSessions(t, a); len(got) != 1 {
		t.Fatalf("got %d sessions, want 1 (stray and nested files ignored)", len(got))
	}
}

// TestOversizedLineCountsOneSkipped pins the buffer-overflow branch of
// scanner.Err(): a line beyond maxLineSize leaves the rest of the file
// unreadable and counts exactly one skipped line (SCHEMA.md), while earlier
// lines still parse.
func TestOversizedLineCountsOneSkipped(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects", "proj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	oversized := `{"type":"user","cwd":"/home/dev/app","timestamp":"2026-07-01T10:00:05Z","message":{"role":"user","content":"` +
		strings.Repeat("a", 17*1024*1024) + `"}}`
	content := `{"type":"user","cwd":"/home/dev/app","timestamp":"2026-07-01T10:00:00Z","message":{"role":"user","content":"first"}}` + "\n" + oversized
	if err := os.WriteFile(filepath.Join(dir, "99999999-9999-4999-8999-999999999999.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New(home)
	metas := listMetas(t, a)
	if len(metas) != 1 || metas[0].Messages != 1 {
		t.Fatalf("got %+v, want 1 session with the first line's message", metas)
	}
	if a.SkippedLines() != 1 {
		t.Errorf("SkippedLines = %d, want 1 (one skipped per oversized file)", a.SkippedLines())
	}
}

// TestScanFileIOErrorNotCountedAsSkipped pins the I/O branch of scanner.Err()
// (M4-B7): a read failure makes the file unreadable (skipped silently, like
// any unreadable file) — it must not inflate the unreadable-content counter,
// and it must never be fatal.
func TestScanFileIOErrorNotCountedAsSkipped(t *testing.T) {
	a := newTestAdapter(t, nil)
	// a directory named *.jsonl: os.Open succeeds, reading fails with an I/O
	// error that is not bufio.ErrTooLong
	dirFile := filepath.Join(a.home, ".claude", "projects", "proj", "iam-a-directory.jsonl")
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

// writeHomeLine appends one raw line to a session file inside a fresh home.
func writeHomeLine(t *testing.T, lines ...string) *Adapter {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects", "proj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "12121212-1212-4212-8212-121212121212.jsonl"),
		[]byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return New(home)
}

// TestKnownTypeUnusableContentCountsSkipped pins M4-B8: a user/assistant/
// system record whose message.content is neither a string nor an array (e.g.
// a number) is skipped AND counted, per SCHEMA.md ("known types without a
// usable message are skipped and counted") — previously it was counted as
// neither. Empty string/array content stays readable-with-nothing-to-record.
func TestKnownTypeUnusableContentCountsSkipped(t *testing.T) {
	a := writeHomeLine(t,
		`{"type":"user","sessionId":"12121212-1212-4212-8212-121212121212","cwd":"/home/dev/app","timestamp":"2026-07-01T10:00:00Z","message":{"role":"user","content":42}}`,
		`{"type":"user","sessionId":"12121212-1212-4212-8212-121212121212","cwd":"/home/dev/app","timestamp":"2026-07-01T10:00:05Z","message":{"role":"user","content":"usable text"}}`,
	)
	metas := listMetas(t, a)
	if len(metas) != 1 {
		t.Fatalf("got %d sessions, want 1", len(metas))
	}
	if metas[0].Messages != 1 {
		t.Errorf("Messages = %d, want 1 (only the usable text)", metas[0].Messages)
	}
	if a.SkippedLines() != 1 {
		t.Errorf("SkippedLines = %d, want 1 (number content is a skipped record)", a.SkippedLines())
	}
}

// TestMalformedBlockKeepsEarlierEntriesAndCountsSkipped pins M4-B9's hybrid
// semantics: a shape-mismatched block mid-array stops parsing that array, but
// the entries parsed before it are still emitted and the line is additionally
// counted as skipped (spec §8: corrupt/unknown → skip, count).
func TestMalformedBlockKeepsEarlierEntriesAndCountsSkipped(t *testing.T) {
	a := writeHomeLine(t,
		`{"type":"user","sessionId":"12121212-1212-4212-8212-121212121212","cwd":"/home/dev/app","timestamp":"2026-07-01T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"good before"},{"type":42},{"type":"text","text":"never reached"}]}}`,
		`{"type":"user","sessionId":"12121212-1212-4212-8212-121212121212","cwd":"/home/dev/app","timestamp":"2026-07-01T10:00:05Z","message":{"role":"user","content":"after"}}`,
	)
	metas := listMetas(t, a)
	if len(metas) != 1 {
		t.Fatalf("got %d sessions, want 1", len(metas))
	}
	if metas[0].Messages != 2 {
		t.Errorf("Messages = %d, want 2 (good-before + after)", metas[0].Messages)
	}
	if a.SkippedLines() != 1 {
		t.Errorf("SkippedLines = %d, want 1 (the malformed-block line counts as skipped)", a.SkippedLines())
	}
	// the Entries scan is a second pass: the counter accumulates across scans
	// (see the Adapter doc), so the same line counts once more
	var texts []string
	err := a.Entries(metas[0].Session, func(e agentlog.Entry) error { texts = append(texts, e.Text); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) != 2 || texts[0] != "good before" || texts[1] != "after" {
		t.Errorf("entries = %v, want [good before after] (earlier entries survive)", texts)
	}
	if a.SkippedLines() != 2 {
		t.Errorf("SkippedLines = %d, want 2 after the second scan (counter accumulates)", a.SkippedLines())
	}
}

func TestDefensiveParsingCountsSkipped(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"proj/44444444-4444-4444-8444-444444444444.jsonl": "truncated.jsonl",
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

func TestSkippedLinesAccumulateAcrossScans(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"proj/44444444-4444-4444-8444-444444444444.jsonl": "truncated.jsonl",
	})
	_ = listSessions(t, a)
	_ = listSessions(t, a)
	if a.SkippedLines() != 2 {
		t.Errorf("SkippedLines = %d, want 2 after two scans", a.SkippedLines())
	}
}

func TestLongLineOver64KB(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"proj/55555555-5555-4555-8555-555555555555.jsonl": "long-line.jsonl",
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

func TestEntriesStreamRealistic(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"proj/3f9c81a2-1111-4222-8333-cccccccccccc.jsonl": "realistic.jsonl",
	})
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
		{agentlog.Summary, "", "Fix jwt refresh token rotation"},
		{agentlog.Message, "user", "the refresh token is stored in localStorage, is that safe?"},
		{agentlog.Message, "assistant", "No — localStorage is readable by any script on the page. Let me move it to an httpOnly cookie."},
		{agentlog.ToolCall, "assistant", `Read {"file_path":"/home/dev/myapp/src/auth.ts"}`},
		{agentlog.ToolResult, "tool", "import { jwtSign } from './jwt'\n// token rotation: TODO"},
		{agentlog.Message, "system", "hook: PostToolUse Read"},
		{agentlog.ToolCall, "assistant", `Edit {"file_path":"/home/dev/myapp/src/auth.ts","old_string":"localStorage.setItem('refresh', token)","new_string":"res.cookie('refresh', token, { httpOnly: true, secure: true })"}`},
		{agentlog.ToolResult, "tool", "The file has been updated."},
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
	if got[1].Timestamp.IsZero() {
		t.Error("message entries should carry the record timestamp")
	}
	if !got[0].Timestamp.IsZero() {
		t.Error("summary entry has no timestamp in fixture")
	}
}

func TestEntriesFindsSessionBySessionIDField(t *testing.T) {
	// filename deliberately differs from the sessionId carried in the records
	a := newTestAdapter(t, map[string]string{
		"proj/mismatched-name.jsonl": "realistic.jsonl",
	})
	s := agentlog.Session{ID: "3f9c81a2-1111-4222-8333-cccccccccccc", Agent: "claude-code"}
	n := 0
	err := a.Entries(s, func(e agentlog.Entry) error { n++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Error("Entries should locate the file by sessionId when the filename differs")
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

func TestIterEarlyStop(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"proj/11111111-1111-4111-8111-111111111111.jsonl": "realistic.jsonl",
		"proj/22222222-2222-4222-8222-222222222222.jsonl": "no-trailing-newline.jsonl",
	})
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
}

func TestUnicodeContent(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"proj/66666666-6666-4666-8666-666666666666.jsonl": "unicode.jsonl",
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
	dir := filepath.Join(home, ".claude", "projects", "garbage")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	garbage := []string{
		"{",
		"[]",
		"null",
		"\"string\"",
		"123",
		"{\"type\":123}",
		"{\"type\":\"user\",\"message\":null}",
		"{\"type\":\"user\",\"message\":{\"content\":123}}",
		"{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":42}]}}",
		"{\"type\":\"assistant\",\"message\":{\"content\":[\"plain string in array\"]}}",
		"\x00\x01\x02binary",
	}
	var b strings.Builder
	for _, g := range garbage {
		fmt.Fprintf(&b, "%s\n", g)
	}
	if err := os.WriteFile(filepath.Join(dir, "77777777-7777-4777-8777-777777777777.jsonl"),
		[]byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New(home)
	metas := listMetas(t, a)
	// the "plain string in array" assistant block is a text block -> message
	if len(metas) != 1 || metas[0].Messages != 1 {
		t.Errorf("got %+v, want 1 session with 1 message", metas)
	}
}

// TestEntriesFilteredPrefilter verifies the search hot-path hook: lines the
// prefilter rejects are never parsed (a garbage line rejected by keep must
// not increment the skipped counter) and matching lines still yield entries.
func TestEntriesFilteredPrefilter(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		"proj/3f9c81a2-1111-4222-8333-cccccccccccc.jsonl": "realistic.jsonl",
	})
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

	keepToken := func(line []byte) bool { return strings.Contains(string(line), "localStorage") }
	var texts []string
	err = a.EntriesFiltered(s, keepToken, func(e agentlog.Entry) error { texts = append(texts, e.Text); return nil })
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(texts, "\n")
	if !strings.Contains(joined, "the refresh token is stored in localStorage") {
		t.Error("kept line's entries are missing from the result")
	}
	if strings.Contains(joined, "rotate on every use") {
		t.Error("entries from prefilter-rejected lines must not be parsed")
	}
}
