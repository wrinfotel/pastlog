package opencode

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pastlog/pastlog/internal/agentlog"
)

// buildFixtureDB generates the synthetic database into a temp dir.
func buildFixtureDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := GenerateTestDB(dir); err != nil {
		t.Fatalf("GenerateTestDB: %v", err)
	}
	return dir
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

const (
	parentID = "ses_fixture100001fix"
	childID  = "ses_fixture200002fix"
)

func TestDetect(t *testing.T) {
	if NewDir(t.TempDir()).Detect() {
		t.Fatal("Detect should be false without opencode.db")
	}
	a := NewDir(buildFixtureDB(t))
	if !a.Detect() {
		t.Fatal("Detect should be true with a generated database")
	}
	if got, want := filepath.ToSlash(a.StoragePath()), filepath.ToSlash(a.dir); got != want {
		t.Errorf("StoragePath = %q, want %q (the storage root)", got, want)
	}
	// a directory named opencode.db is not a database
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "opencode.db"), 0o755); err != nil {
		t.Fatal(err)
	}
	if NewDir(dir).Detect() {
		t.Error("Detect should be false when opencode.db is a directory")
	}
}

func TestSessionsRealisticFixture(t *testing.T) {
	a := NewDir(buildFixtureDB(t))
	metas := listMetas(t, a)
	if len(metas) != 2 {
		t.Fatalf("got %d sessions, want 2 (parent + child, no filtering)", len(metas))
	}
	parent := metas[0]
	if parent.ID != parentID {
		t.Fatalf("first session = %q, want the oldest one first", parent.ID)
	}
	if parent.Agent != "opencode" {
		t.Errorf("Agent = %q", parent.Agent)
	}
	if parent.Project != "C:/dev/fixture app" {
		t.Errorf("Project = %q, want the directory column verbatim", parent.Project)
	}
	if parent.Title != "Fix the widget flux" {
		t.Errorf("Title = %q, want the title column", parent.Title)
	}
	wantStart := time.UnixMilli(1786220928012)
	if !parent.StartedAt.Equal(wantStart) {
		t.Errorf("StartedAt = %v, want %v (epoch ms)", parent.StartedAt, wantStart)
	}
	wantEnd := time.UnixMilli(1786220992255)
	if !parent.EndedAt.Equal(wantEnd) {
		t.Errorf("EndedAt = %v, want %v", parent.EndedAt, wantEnd)
	}
	// messages = text parts with non-empty text: 1 user + 1 assistant
	if parent.Messages != 2 {
		t.Errorf("Messages = %d, want 2", parent.Messages)
	}
	if metas[1].ID != childID {
		t.Errorf("second session = %q, want the child subagent session (listed, not filtered)", metas[1].ID)
	}
	if metas[1].Messages != 1 {
		t.Errorf("child Messages = %d, want 1", metas[1].Messages)
	}
}

func TestSessionSizeSumsRowData(t *testing.T) {
	dir := buildFixtureDB(t)
	a := NewDir(dir)
	sessions := listSessions(t, a)
	parent := sessionByID(t, sessions, parentID)

	// expected size: summed byte lengths of the parent session's message.data
	// and part.data values ( LENGTH(CAST(data AS BLOB)) )
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, "opencode.db"))+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var want int64
	err = db.QueryRow(`SELECT
		(SELECT COALESCE(SUM(LENGTH(CAST(data AS BLOB))),0) FROM message WHERE session_id=?) +
		(SELECT COALESCE(SUM(LENGTH(CAST(data AS BLOB))),0) FROM part WHERE session_id=?)`,
		parentID, parentID).Scan(&want)
	if err != nil {
		t.Fatal(err)
	}
	if parent.SizeBytes != want {
		t.Errorf("SizeBytes = %d, want %d (sum of row data byte lengths)", parent.SizeBytes, want)
	}
	if parent.SizeBytes <= 0 {
		t.Errorf("SizeBytes = %d, fixture data must be non-empty", parent.SizeBytes)
	}
}

func TestEntriesStreamRealistic(t *testing.T) {
	a := NewDir(buildFixtureDB(t))
	sessions := listSessions(t, a)
	var got []agentlog.Entry
	err := a.Entries(sessionByID(t, sessions, parentID), func(e agentlog.Entry) error {
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
		{agentlog.Message, "user", "the widget flux calibration keeps drifting"},
		{agentlog.Message, "assistant", "recalibrating the flux capacitor now"},
		{agentlog.Summary, "assistant", "The drift matches the ambient humidity."},
		{agentlog.ToolCall, "tool", `{"filePath":"C:/dev/fixture app/flux.ts"}`},
		{agentlog.ToolResult, "tool", "const flux = calibrate(0.42)"},
		{agentlog.ToolCall, "tool", `{"filePath":"flux.ts"}`}, // pending tool: no result yet
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
		t.Error("part entries should carry part.time_created")
	}
	if want := time.UnixMilli(1786220928154); !got[0].Timestamp.Equal(want) {
		t.Errorf("entry[0].Timestamp = %v, want %v", got[0].Timestamp, want)
	}
}

func TestEntriesSkippedRecordsCounted(t *testing.T) {
	a := NewDir(buildFixtureDB(t))
	sessions := listSessions(t, a)
	parent := sessionByID(t, sessions, parentID)
	_ = a.Entries(parent, func(e agentlog.Entry) error { return nil })
	// only genuinely unknown shapes count: the unknown "hologram" part type
	// plus the malformed json row. The recognized-but-unmapped records
	// (file, patch, step-start, step-finish, compaction) are dropped
	// silently — spec §8 reserves the counter for corrupt/unknown shapes
	// (controller ruling).
	if a.SkippedLines() != 2 {
		t.Errorf("SkippedLines = %d, want 2", a.SkippedLines())
	}
	// counts accumulate across scans
	_ = a.Entries(parent, func(e agentlog.Entry) error { return nil })
	if a.SkippedLines() != 4 {
		t.Errorf("SkippedLines = %d, want 4 after two scans", a.SkippedLines())
	}
}

// TestEntriesUnparseableMessageDataCountsSkipped pins the M4-B17 ruling: a
// message row whose data is not valid JSON counts as skipped (aligned with
// the spec §8 letter), while its parts still stream — content is never
// dropped, the role just degrades to "".
func TestEntriesUnparseableMessageDataCountsSkipped(t *testing.T) {
	a := NewDir(buildFixtureDB(t))
	var got []agentlog.Entry
	err := a.Entries(agentlog.Session{ID: childID}, func(e agentlog.Entry) error {
		got = append(got, e)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// the child session's readable message still yields its text part
	if len(got) != 1 || got[0].Text != "child session report: flux stable" {
		t.Fatalf("entries = %+v, want the readable part only", got)
	}
	if a.SkippedLines() != 1 {
		t.Errorf("SkippedLines = %d, want 1 (the unparseable message.data row)", a.SkippedLines())
	}
}

func TestEntriesUnknownSessionErrors(t *testing.T) {
	a := NewDir(buildFixtureDB(t))
	err := a.Entries(agentlog.Session{ID: "ses_nope"}, func(e agentlog.Entry) error { return nil })
	if err == nil {
		t.Fatal("expected error for unknown session")
	}
	if msg := err.Error(); msg != "" && msg[0] >= 'A' && msg[0] <= 'Z' {
		t.Errorf("error should be lowercase: %q", msg)
	}
}

func TestIterEarlyStop(t *testing.T) {
	a := NewDir(buildFixtureDB(t))
	sentinel := errors.New("stop")
	err := a.Sessions(func(s agentlog.Session) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Errorf("Sessions should propagate iter error, got %v", err)
	}
	err = a.Entries(agentlog.Session{ID: parentID}, func(e agentlog.Entry) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("Entries should propagate iter error, got %v", err)
	}
}

func TestTotalBytesSumsDBFiles(t *testing.T) {
	dir := buildFixtureDB(t)
	a := NewDir(dir)
	want := int64(0)
	for _, name := range []string{"opencode.db", "opencode.db-wal", "opencode.db-shm"} {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil {
			want += info.Size()
		}
	}
	if got := a.TotalBytes(); got != want {
		t.Errorf("TotalBytes = %d, want %d (db + wal + shm)", got, want)
	}
	if want == 0 {
		t.Fatal("fixture database must be non-empty")
	}
}

// TestNoWritesToDatabase pins the read-only guarantee: opening the adapter
// must not modify the database file or create side files.
func TestNoWritesToDatabase(t *testing.T) {
	dir := buildFixtureDB(t)
	before := statAll(t, dir)

	a := NewDir(dir)
	if err := a.Sessions(func(agentlog.Session) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := a.Entries(agentlog.Session{ID: parentID}, func(agentlog.Entry) error { return nil }); err != nil {
		t.Fatal(err)
	}
	after := statAll(t, dir)
	if len(before) != len(after) {
		t.Fatalf("file set changed: before %v, after %v", before, after)
	}
	for name, size := range before {
		if after[name] != size {
			t.Errorf("file %s changed size: %d -> %d", name, size, after[name])
		}
	}
}

func statAll(t *testing.T, dir string) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		info, err := e.Info()
		if err == nil {
			out[e.Name()] = info.Size()
		}
	}
	return out
}

// TestLockedDBWarnsAndContinues is the spec §4 fallback: a second connection
// holds a write lock (BEGIN EXCLUSIVE) and the adapter degrades to
// unavailable — no sessions, no error, one warning — while remaining healthy
// otherwise.
func TestLockedDBWarnsAndContinues(t *testing.T) {
	dir := buildFixtureDB(t)
	release := lockDatabase(t, dir)
	defer release()

	a := NewDir(dir)
	if !a.Detect() {
		t.Fatal("Detect should stay true: the storage exists, it is merely locked")
	}
	if a.Warning() != "" {
		t.Error("no warning before the locked condition is discovered")
	}

	var sessions int
	err := a.Sessions(func(agentlog.Session) error { sessions++; return nil })
	if err != nil {
		t.Errorf("Sessions on a locked DB must not fail: %v", err)
	}
	if sessions != 0 {
		t.Errorf("locked DB should yield no sessions, got %d", sessions)
	}
	if msg := a.Warning(); msg == "" {
		t.Error("locked DB should produce a warning line")
	} else if msg[0] >= 'A' && msg[0] <= 'Z' {
		t.Errorf("warning should be lowercase: %q", msg)
	}

	// repeated scans keep the adapter consistent without double-reporting
	err = a.SessionsMeta(func(agentlog.SessionMeta) error { return nil })
	if err != nil {
		t.Errorf("SessionsMeta on a locked DB must not fail: %v", err)
	}
	if a.Warning() == "" {
		t.Error("warning should persist while the DB stays locked")
	}
}

func TestLockedDBEntriesNoError(t *testing.T) {
	dir := buildFixtureDB(t)
	release := lockDatabase(t, dir)
	defer release()

	a := NewDir(dir)
	n := 0
	if err := a.Entries(agentlog.Session{ID: parentID}, func(agentlog.Entry) error { n++; return nil }); err != nil {
		t.Errorf("Entries on a locked DB must not fail: %v", err)
	}
	if n != 0 {
		t.Errorf("locked DB should yield no entries, got %d", n)
	}
	if a.Warning() == "" {
		t.Error("Entries should record the locked condition too")
	}
}

// lockDatabase opens a second (read-write) connection and holds an EXCLUSIVE
// lock for the duration of the test, so the adapter's read-only connection
// runs into SQLITE_BUSY after its short busy timeout.
func lockDatabase(t *testing.T, dir string) func() {
	t.Helper()
	dsn := "file:" + filepath.ToSlash(filepath.Join(dir, "opencode.db"))
	db, err := sql.Open("sqlite", dsn)
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
	return func() {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		_ = conn.Close()
		_ = db.Close()
	}
}
