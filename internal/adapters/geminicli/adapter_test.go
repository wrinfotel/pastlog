package geminicli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wrinfotel/pastlog/internal/agentlog"
)

const testHash = "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0"

// buildHome copies named fixtures from testdata/ into a synthetic home tree
// under .gemini/tmp/<hash>/. The adapter must never touch these source files;
// everything is copied into a temp dir first.
func buildHome(t *testing.T, files map[string]string) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".gemini", "tmp", testHash, "chats"), 0o755); err != nil {
		t.Fatal(err)
	}
	for rel, fixture := range files {
		dst := filepath.Join(home, ".gemini", "tmp", testHash, rel)
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

// realisticRel is the on-disk name gemini-cli uses: session-<ts>-<id8>.jsonl.
const realisticRel = "chats/session-2026-08-02T14-03-gabc1111.jsonl"

const (
	realisticID = "gabc1111-1111-4111-8111-111111111111"
	subagentID  = "gsub2222-2222-4222-8222-222222222222"
	legacyID1   = "gold3333-3333-4333-8333-333333333333"
	legacyID2   = "gold4444-4444-4444-8444-444444444444"
)

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

func sessionByID(t *testing.T, sessions []agentlog.Session, id string) agentlog.Session {
	t.Helper()
	for _, s := range sessions {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("session %q not found in %v", id, sessions)
	return agentlog.Session{}
}

func TestDetect(t *testing.T) {
	if New(t.TempDir()).Detect() {
		t.Fatal("Detect should be false without storage")
	}
	a := newTestAdapter(t, nil) // .gemini/tmp exists but is empty
	if !a.Detect() {
		t.Fatal("Detect should be true when .gemini/tmp exists")
	}
	if got := a.StoragePath(); !strings.HasSuffix(filepath.ToSlash(got), "/.gemini/tmp") {
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
	if s.ID != realisticID {
		t.Errorf("ID = %q, want metadata sessionId", s.ID)
	}
	if s.Agent != "gemini-cli" {
		t.Errorf("Agent = %q", s.Agent)
	}
	if s.Project != "/home/dev/myapp/api" {
		t.Errorf("Project = %q, want first directories[] entry", s.Project)
	}
	if s.Title != "move refresh token to an httpOnly cookie" {
		t.Errorf("Title = %q, want the metadata summary", s.Title)
	}
	wantStart := time.Date(2026, 8, 2, 14, 3, 20, 0, time.UTC)
	if !s.StartedAt.Equal(wantStart) {
		t.Errorf("StartedAt = %v, want %v (metadata startTime)", s.StartedAt, wantStart)
	}
	wantEnd := time.Date(2026, 8, 2, 14, 5, 10, 0, time.UTC)
	if !s.EndedAt.Equal(wantEnd) {
		t.Errorf("EndedAt = %v, want %v (metadata lastUpdated)", s.EndedAt, wantEnd)
	}
	// m1 user, m2 gemini, m3 info (system), m4 two text parts, m5 error
	// (system), checkpoint m6 — 7 Message-kind entries (user/assistant/system
	// all count, per the M1 SessionMeta.Messages semantic)
	if messages != 7 {
		t.Errorf("Messages = %d, want 7", messages)
	}
	info, err := os.Stat(filepath.Join(a.home, ".gemini", "tmp", testHash, filepath.FromSlash(realisticRel)))
	if err != nil {
		t.Fatal(err)
	}
	if s.SizeBytes != info.Size() {
		t.Errorf("SizeBytes = %d, want %d", s.SizeBytes, info.Size())
	}
}

func TestSubagentSession(t *testing.T) {
	a := newTestAdapter(t, map[string]string{
		realisticRel: "realistic.jsonl",
		"chats/gparent9999-9999-4999-8999-999999999999/subagent.jsonl": "subagent.jsonl",
	})
	sessions := listSessions(t, a)
	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want 2 (main + subagent)", len(sessions))
	}
	sub := sessionByID(t, sessions, subagentID)
	if sub.Project != "/home/dev/myapp/api" {
		t.Errorf("subagent Project = %q", sub.Project)
	}
	if sub.Title != "" {
		t.Errorf("subagent Title = %q, want empty without summary", sub.Title)
	}
	// Entries by metadata id must find the subagent file
	n := 0
	err := a.Entries(sub, func(e agentlog.Entry) error { n++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("subagent Entries = %d, want 2", n)
	}
}

func TestEntriesStreamRealistic(t *testing.T) {
	a := newTestAdapter(t, map[string]string{realisticRel: "realistic.jsonl"})
	sessions := listSessions(t, a)
	var got []agentlog.Entry
	err := a.Entries(sessionByID(t, sessions, realisticID), func(e agentlog.Entry) error {
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
		{agentlog.Message, "assistant", "No — localStorage is readable by any script on the page."},
		{agentlog.ToolCall, "tool", `{"absolute_path":"/home/dev/myapp/api/src/auth.ts"}`},
		{agentlog.ToolResult, "tool", "export const store = (token) => localStorage.setItem('refresh', token)"},
		{agentlog.ToolCall, "tool", `{"file_path":"/home/dev/myapp/api/src/auth.ts","old_string":"localStorage"}`},
		{agentlog.ToolResult, "tool", "Replaced 1 occurrence."},
		{agentlog.Summary, "assistant", "Security review: Consider where the refresh token is stored and how it rotates."},
		{agentlog.Message, "system", "Using model gemini-2.5-pro"},
		{agentlog.Message, "assistant", "we moved it to an httpOnly cookie"},
		{agentlog.Message, "assistant", "and rotate on every use"},
		{agentlog.Message, "system", "rate limit hit; retrying"},
		// checkpoint {$set:{messages:[...]}} records are applied in place
		{agentlog.Message, "user", "thanks, that fixed it"},
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
	if want := time.Date(2026, 8, 2, 14, 3, 35, 0, time.UTC); !got[2].Timestamp.Equal(want) {
		t.Errorf("toolCall entry timestamp = %v, want the toolCall's own %v", got[2].Timestamp, want)
	}
}

func TestUnknownShapesCountedAsSkipped(t *testing.T) {
	a := newTestAdapter(t, map[string]string{realisticRel: "realistic.jsonl"})
	_ = listSessions(t, a)
	if a.SkippedLines() != 1 {
		t.Errorf("SkippedLines = %d, want 1 (record without type)", a.SkippedLines())
	}
}

// TestScanFileIOErrorNotCountedAsSkipped pins the I/O branch of
// scanner.Err() (aligned with claudecode; final review finding 3): a read
// failure makes the file unreadable (skipped silently, like any unreadable
// file) — it must not inflate the unreadable-content counter, and it must
// never be fatal. Only bufio.ErrTooLong (an oversized line) counts.
func TestScanFileIOErrorNotCountedAsSkipped(t *testing.T) {
	a := newTestAdapter(t, nil)
	// a directory named session-*.jsonl: os.Open succeeds, reading fails with
	// an I/O error that is not bufio.ErrTooLong
	dirFile := filepath.Join(a.home, ".gemini", "tmp", testHash, "chats", "session-iam-a-directory.jsonl")
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

func TestLegacyChatsJSONSessions(t *testing.T) {
	a := newTestAdapter(t, map[string]string{"chats.json": "chats.json"})
	sessions := listSessions(t, a)
	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want 2 (legacy chats.json)", len(sessions))
	}
	s1 := sessionByID(t, sessions, legacyID1)
	if s1.Project != "/home/dev/legacy" {
		t.Errorf("legacy Project = %q", s1.Project)
	}
	if s1.Title != "legacy monolithic session" {
		t.Errorf("legacy Title = %q", s1.Title)
	}
	wantStart := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	if !s1.StartedAt.Equal(wantStart) {
		t.Errorf("legacy StartedAt = %v, want %v", s1.StartedAt, wantStart)
	}
	s2 := sessionByID(t, sessions, legacyID2)
	if s2.Project != "" || s2.Title != "" {
		t.Errorf("second legacy session = %+v, want empty project/title", s2)
	}
	metas := listMetas(t, a)
	if metas[0].Messages != 2 || metas[1].Messages != 1 {
		t.Errorf("legacy message counts = %d, %d; want 2, 1", metas[0].Messages, metas[1].Messages)
	}
	// SizeBytes approximates the session as its raw element length (>0)
	if s1.SizeBytes <= 0 || s2.SizeBytes <= 0 {
		t.Errorf("legacy SizeBytes must be the element length, got %d and %d", s1.SizeBytes, s2.SizeBytes)
	}
}

func TestLegacyEntries(t *testing.T) {
	a := newTestAdapter(t, map[string]string{"chats.json": "chats.json"})
	var texts []string
	err := a.Entries(agentlog.Session{ID: legacyID1, Agent: "gemini-cli"}, func(e agentlog.Entry) error {
		texts = append(texts, e.Text)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) != 2 || texts[0] != "hello from the legacy format" || texts[1] != "Hi! How can I help?" {
		t.Errorf("legacy entries = %q", texts)
	}
}

func TestEntriesUnknownSessionErrors(t *testing.T) {
	a := newTestAdapter(t, nil)
	err := a.Entries(agentlog.Session{ID: "nope"}, func(e agentlog.Entry) error { return nil })
	if err == nil {
		t.Fatal("expected error for unknown session")
	}
	if msg := err.Error(); msg != "" && msg[0] >= 'A' && msg[0] <= 'Z' {
		t.Errorf("error should be lowercase: %q", msg)
	}
}

// TestSessionsGlobMetacharacterHome ports the claudecode lister regression
// (final review, M4 fix): the gemini-cli listers — main JSONL sessions,
// subagent files and the legacy monolithic chats.json — must survive a home
// path containing glob metacharacters, where filepath.Glob silently matches
// nothing, for both listing and per-session file resolution.
func TestSessionsGlobMetacharacterHome(t *testing.T) {
	// Windows forbids * and ? in real filenames, but [ and ] are legal and
	// are exactly the metacharacters that turn a Glob pattern into a broken
	// character class.
	home := filepath.Join(t.TempDir(), "we[ird]home")
	chats := filepath.Join(home, ".gemini", "tmp", testHash, "chats")
	if err := os.MkdirAll(filepath.Join(chats, "gparent9999-9999-4999-8999-999999999999"), 0o755); err != nil {
		t.Fatal(err)
	}
	mainContent := `{"sessionId":"gmet1111-1111-4111-8111-111111111111","startTime":"2026-07-01T10:00:00Z","lastUpdated":"2026-07-01T10:05:00Z","kind":"main","directories":["/home/dev/app"],"summary":"metachar home"}` + "\n" +
		`{"id":"m1","timestamp":"2026-07-01T10:01:00Z","type":"user","content":"one"}`
	subContent := `{"sessionId":"gmet2222-2222-4222-8222-222222222222","kind":"subagent","directories":["/home/dev/app"]}` + "\n" +
		`{"id":"m1","timestamp":"2026-07-01T11:00:00Z","type":"user","content":"two"}`
	legacyContent := `{"version":1,"sessions":[{"sessionId":"gmet3333-3333-4333-8333-333333333333","messages":[{"id":"m1","type":"user","content":"legacy one"}]}]}`
	for name, content := range map[string]string{
		"session-2026-07-01T10-00-gmet1111.jsonl":                mainContent,
		"gparent9999-9999-4999-8999-999999999999/subagent.jsonl": subContent,
	} {
		if err := os.WriteFile(filepath.Join(chats, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// the legacy monolithic store sits in the project dir, next to chats/
	if err := os.WriteFile(filepath.Join(home, ".gemini", "tmp", testHash, "chats.json"), []byte(legacyContent), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New(home)
	got := listSessions(t, a)
	// sorted main JSONL first, sorted subagent JSONL next, legacy last —
	// exactly the listing order the lister had before the metacharacter fix
	wantIDs := []string{"gmet1111-1111-4111-8111-111111111111", "gmet2222-2222-4222-8222-222222222222", "gmet3333-3333-4333-8333-333333333333"}
	if len(got) != len(wantIDs) {
		t.Fatalf("got %d sessions, want %d (lister must survive glob metacharacters)", len(got), len(wantIDs))
	}
	for i, want := range wantIDs {
		if got[i].ID != want {
			t.Errorf("session[%d].ID = %q, want %q", i, got[i].ID, want)
		}
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

func TestSessionsEmptyFileYieldsNothing(t *testing.T) {
	a := newTestAdapter(t, map[string]string{realisticRel: "empty.jsonl"})
	if got := listSessions(t, a); len(got) != 0 {
		t.Errorf("empty file should not yield a session, got %d", len(got))
	}
}

func TestNoTrailingNewline(t *testing.T) {
	a := newTestAdapter(t, map[string]string{"chats/session-2026-07-04T08-00-gntn4444.jsonl": "no-trailing-newline.jsonl"})
	metas := listMetas(t, a)
	if len(metas) != 1 {
		t.Fatalf("got %d sessions, want 1", len(metas))
	}
	if metas[0].Messages != 1 {
		t.Errorf("Messages = %d, want 1", metas[0].Messages)
	}
}

func TestTruncatedLineCountedSkipped(t *testing.T) {
	a := newTestAdapter(t, map[string]string{"chats/session-2026-07-02T11-00-gtrc3333.jsonl": "truncated.jsonl"})
	metas := listMetas(t, a)
	if len(metas) != 1 || metas[0].Messages != 1 {
		t.Fatalf("got %+v, want 1 session with 1 message (valid lines still work)", metas)
	}
	if a.SkippedLines() != 1 {
		t.Errorf("SkippedLines = %d, want 1 (truncated json line)", a.SkippedLines())
	}
}

func TestLongLineOver64KB(t *testing.T) {
	a := newTestAdapter(t, map[string]string{"chats/session-2026-07-05T12-00-glong6666.jsonl": "long-line.jsonl"})
	metas := listMetas(t, a)
	if len(metas) != 1 {
		t.Fatalf("got %d sessions, want 1", len(metas))
	}
	if metas[0].Messages != 2 { // user + gemini reply
		t.Errorf("Messages = %d, want 2", metas[0].Messages)
	}
	if metas[0].SizeBytes < 64*1024 {
		t.Errorf("fixture too small: %d bytes", metas[0].SizeBytes)
	}
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

// TestSessionsMetaFastMatchesFullListing pins the search-flow fast listing
// contract (M4 fix round): a JSONL file opening with a metadata record lists
// fast with ids/projects/titles/timestamps identical to the full listing
// (only message counts are zero); a file whose first line is a plain record
// falls back to the full parse and stays identical.
func TestSessionsMetaFastMatchesFullListing(t *testing.T) {
	home := t.TempDir()
	chats := filepath.Join(home, ".gemini", "tmp", "ca11bad5fix", "chats")
	if err := os.MkdirAll(chats, 0o755); err != nil {
		t.Fatal(err)
	}
	fastContent := `{"sessionId":"51515151-5151-5151-8151-515151515151","startTime":"2026-07-01T10:00:00Z","lastUpdated":"2026-07-01T10:05:00Z","kind":"main","directories":["/home/dev/app"],"summary":"widget review"}` + "\n" +
		`{"id":"m1","timestamp":"2026-07-01T10:01:00Z","type":"user","content":"hello"}` + "\n"
	fallbackContent := `{"id":"m1","timestamp":"2026-07-01T11:00:00Z","type":"user","content":"no metadata first"}` + "\n"
	// metadata record with a sessionId but NO startTime: the fast path cannot
	// know the start timestamp the full parse backfills from the record
	// timestamps, so the file must drop to the full parse (FastMetaSource
	// contract: filters and sort order stay identical to SessionsMeta)
	tslessMetaContent := `{"sessionId":"53535353-5353-5353-8535-535353535353","kind":"main","directories":["/home/dev/app"],"summary":"no start time"}` + "\n" +
		`{"id":"m1","timestamp":"2026-07-01T12:00:00Z","type":"user","content":"timestamp backfill"}` + "\n"
	for name, content := range map[string]string{
		"session-2026-07-01T10-00-51515151.jsonl": fastContent,     // sorted first
		"session-2026-07-01T11-00-52525252.jsonl": fallbackContent, // line 1 is not a metadata record
		"session-2026-07-01T12-00-53535353.jsonl": tslessMetaContent,
	} {
		if err := os.WriteFile(filepath.Join(chats, name), []byte(content), 0o644); err != nil {
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
	if len(fast) != 3 || len(full) != 3 {
		t.Fatalf("fast=%d full=%d sessions, want 3 each", len(fast), len(full))
	}
	for i := range fast {
		if fast[i].ID != full[i].ID || fast[i].Project != full[i].Project || fast[i].Title != full[i].Title ||
			!fast[i].StartedAt.Equal(full[i].StartedAt) || !fast[i].EndedAt.Equal(full[i].EndedAt) {
			t.Errorf("session[%d]: fast %+v differs from full %+v", i, fast[i], full[i])
		}
	}
	// the metadata-first file carries everything but the message count on the
	// fast path
	if fast[0].Messages != 0 {
		t.Errorf("fast meta Messages = %d, want 0", fast[0].Messages)
	}
	if full[0].Messages != 1 {
		t.Errorf("full meta Messages = %d, want 1", full[0].Messages)
	}
	// the fallback file (first line is a plain record) is identical on both
	// paths, including its filename-fallback id
	if fast[1].ID != "session-2026-07-01T11-00-52525252" || fast[1].Messages != full[1].Messages {
		t.Errorf("fallback file must produce the full meta on the fast path too, got %+v vs %+v", fast[1], full[1])
	}
	// the timestamp-less metadata record must fall back to the full parse so
	// the record-backfilled start timestamp (and message count) is identical
	// on both paths — the FastMetaSource filters/sort parity
	if fast[2].ID != "53535353-5353-5353-8535-535353535353" || fast[2].StartedAt.IsZero() || fast[2].Messages != 1 {
		t.Errorf("timestamp-less metadata must drop to the full parse, got %+v", fast[2].Session)
	}
}

func TestUnicodeContent(t *testing.T) {
	a := newTestAdapter(t, map[string]string{"chats/session-2026-07-03T09-15-guni5555.jsonl": "unicode.jsonl"})
	got := listSessions(t, a)
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	if got[0].Project != "/home/dev/unicodeプロジェクト" {
		t.Errorf("Project = %q", got[0].Project)
	}
	var texts []string
	_ = a.Entries(got[0], func(e agentlog.Entry) error { texts = append(texts, e.Text); return nil })
	joined := strings.Join(texts, "\n")
	for _, want := range []string{"こんにちは", "🌍", "café", "naïve", "журнал", "挨拶"} {
		if !strings.Contains(joined, want) {
			t.Errorf("unicode content lost: %q not in entries", want)
		}
	}
}

func TestNeverPanicsOnGarbage(t *testing.T) {
	garbage := []string{
		"{",
		"[]",
		"null",
		"\"string\"",
		"123",
		`{"type":123}`,
		`{"type":"user"}`,              // known type, no content
		`{"type":"user","content":42}`, // content of the wrong shape
		`{"type":"gemini","toolCalls":42}`,
		`{"$set":{"messages":"not-an-array"}}`,
		`{"$set":{"messages":[{"type":"user","content":null}]}}`, // tolerated: empty content
		`{"$set":{"messages":[{"id":"no-type-here"}]}}`,          // unknown shape inside a recognized checkpoint: counted
		"\x00\x01\x02binary",
	}
	home := t.TempDir()
	var b strings.Builder
	for _, g := range garbage {
		b.WriteString(g + "\n")
	}
	dir := filepath.Join(home, ".gemini", "tmp", testHash, "chats")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session-2026-07-01T09-00-ggrb7777.jsonl"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New(home)
	metas := listMetas(t, a)
	// the no-content user record and the null-content checkpoint message are
	// tolerated as readable records with no text; everything else — including
	// unknown shapes inside a recognized checkpoint — is skipped and counted
	if len(metas) != 1 || metas[0].Messages != 0 {
		t.Errorf("got %+v, want 1 session with 0 messages", metas)
	}
	if a.SkippedLines() != 11 {
		t.Errorf("SkippedLines = %d, want 11", a.SkippedLines())
	}
}

func TestLegacyGarbageCountedSkipped(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".gemini", "tmp", testHash)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	broken := `{"version":1,"sessions":[{"sessionId":"gbrk8888-8888-4888-8888-888888888888","messages":[{"id":"x","type":"user","content":"ok"},{"id":"y"}]},`
	if err := os.WriteFile(filepath.Join(dir, "chats.json"), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New(home)
	sessions := listSessions(t, a)
	if len(sessions) != 1 || sessions[0].ID != "gbrk8888-8888-4888-8888-888888888888" {
		t.Fatalf("sessions = %v, want the one complete legacy session", sessions)
	}
	if a.SkippedLines() < 1 {
		t.Errorf("SkippedLines = %d, want >=1 for the unreadable remainder", a.SkippedLines())
	}
}

func TestIterEarlyStop(t *testing.T) {
	a := newTestAdapter(t, map[string]string{realisticRel: "realistic.jsonl"})
	sentinel := errors.New("stop")
	err := a.Sessions(func(s agentlog.Session) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Errorf("Sessions should propagate iter error, got %v", err)
	}
	err = a.Entries(agentlog.Session{ID: realisticID}, func(e agentlog.Entry) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("Entries should propagate iter error, got %v", err)
	}
	err = a.EntriesFiltered(agentlog.Session{ID: realisticID}, nil,
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
	s := agentlog.Session{ID: realisticID}

	keepNone := func(line []byte) bool { return false }
	n := 0
	err := a.EntriesFiltered(s, keepNone, func(e agentlog.Entry) error { n++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("keep=false should yield no entries, got %d", n)
	}

	keepLocal := func(line []byte) bool { return strings.Contains(string(line), "localStorage") }
	var texts []string
	err = a.EntriesFiltered(s, keepLocal, func(e agentlog.Entry) error { texts = append(texts, e.Text); return nil })
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(texts, "\n")
	if !strings.Contains(joined, "the refresh token is stored in localStorage") {
		t.Error("kept line's entries are missing from the result")
	}
	if strings.Contains(joined, "rate limit hit") {
		t.Error("entries from prefilter-rejected lines must not be parsed")
	}
}

// TestSessionForIndexResolvesColdAndWarm pins the M4 fix-round id→store
// index (same treatment as codex/claudecode): Entries resolves sessions O(1)
// through the index, both cold (built in one storage pass by buildIndex) and
// warm (already filled by a listing pass) — by filename-fallback id, by
// metadata sessionId whose filename differs, by subagent filename, and into
// the legacy monolithic chats.json.
func TestSessionForIndexResolvesColdAndWarm(t *testing.T) {
	files := map[string]string{
		realisticRel: "realistic.jsonl",
		"chats/gparent9999-9999-4999-8999-999999999999/subagent.jsonl": "subagent.jsonl",
		"chats.json": "chats.json",
	}
	ids := []string{
		"session-2026-08-02T14-03-gabc1111", // filename-fallback style id
		realisticID,                         // metadata sessionId (filename differs)
		subagentID,                          // subagent file's metadata sessionId
		legacyID1,                           // inside the legacy monolith
		legacyID2,
	}
	count := func(t *testing.T, a *Adapter, id string) int {
		t.Helper()
		n := 0
		err := a.Entries(agentlog.Session{ID: id}, func(agentlog.Entry) error { n++; return nil })
		if err != nil {
			t.Fatalf("Entries(%s): %v", id, err)
		}
		return n
	}

	t.Run("cold", func(t *testing.T) {
		a := newTestAdapter(t, files)
		for _, id := range ids {
			if n := count(t, a, id); n == 0 {
				t.Errorf("cold resolution failed for %s", id)
			}
		}
	})
	t.Run("warm", func(t *testing.T) {
		a := newTestAdapter(t, files)
		_ = listSessions(t, a) // listing fills the index
		for _, id := range ids {
			if n := count(t, a, id); n == 0 {
				t.Errorf("warm resolution failed for %s", id)
			}
		}
	})
}
