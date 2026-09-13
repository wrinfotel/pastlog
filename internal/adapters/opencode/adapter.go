// Package opencode adapts OpenCode's SQLite storage: a single
// opencode.db (with -wal/-shm side files) under the resolved storage root
// (see SCHEMA.md and internal/discovery.OpenCodeDir). The database is opened
// strictly read-only; iteration streams per-session SQL cursors, never
// materializing whole result sets. If the database is locked by a running
// OpenCode instance the adapter degrades to unavailable with one warning
// while other agents continue (spec §4).
package opencode

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pastlog/pastlog/internal/agentlog"
	"github.com/pastlog/pastlog/internal/discovery"

	// pure-Go SQLite driver, CGO off (spec §6); registered for its side
	// effects
	_ "modernc.org/sqlite"
)

const (
	agentName     = "opencode"
	dbName        = "opencode.db"
	busyTimeoutMS = 200 // short: fail fast to the locked-DB fallback (spec §4)
)

// Adapter reads OpenCode sessions from the storage root's opencode.db.
type Adapter struct {
	dir     string // resolved storage root, "" when absent
	locked  bool   // storage present but unreadable: locked by a running instance
	skipped int    // unreadable/unknown records, counted across Entries scans
}

// New builds an adapter for the given user home directory, resolving the
// OpenCode storage root via discovery (XDG/LOCALAPPDATA, first existing wins).
func New(home string) *Adapter {
	return &Adapter{dir: discovery.OpenCodeDir(home)}
}

// NewDir builds an adapter for an explicit storage root (tests, exotic setups).
func NewDir(dir string) *Adapter { return &Adapter{dir: dir} }

func (a *Adapter) Name() string { return agentName }

func (a *Adapter) Detect() bool {
	if a.dir == "" {
		return false
	}
	info, err := os.Stat(a.dbPath())
	return err == nil && info.Mode().IsRegular()
}

// SkippedLines implements agentlog.SkipCounter (spec §8 stderr summary).
func (a *Adapter) SkippedLines() int { return a.skipped }

// StoragePath implements agentlog.PathSource: the storage root scanned for
// sessions, for display in `agents`.
func (a *Adapter) StoragePath() string { return a.dir }

// Warning implements agentlog.WarningSource: one stderr line while the
// database is locked (spec §4: warn once, continue with other agents).
func (a *Adapter) Warning() string {
	if !a.locked {
		return ""
	}
	return "opencode: database is locked by a running opencode instance — skipping it; " +
		"other agents are unaffected (retry after closing opencode)"
}

// TotalBytes implements agentlog.TotalSizer (controller ruling): the `agents`
// total size is the on-disk footprint of the database and its side files, not
// the sum of per-session data sizes.
func (a *Adapter) TotalBytes() int64 {
	var total int64
	for _, name := range []string{dbName, dbName + "-wal", dbName + "-shm"} {
		if info, err := os.Stat(filepath.Join(a.dir, name)); err == nil {
			total += info.Size()
		}
	}
	return total
}

func (a *Adapter) dbPath() string { return filepath.Join(a.dir, dbName) }

// dsn is the strictly read-only DSN. mode=ro makes every write impossible
// (the driver refuses with SQLITE_READONLY), and a short busy_timeout avoids
// false-positive lock detection on momentarily blocked files (controller
// ruling; the pragma's parentheses are legal query characters).
func (a *Adapter) dsn() string {
	return "file:" + filepath.ToSlash(a.dbPath()) +
		fmt.Sprintf("?mode=ro&_pragma=busy_timeout(%d)", busyTimeoutMS)
}

// open prepares a single-connection read-only database handle.
func (a *Adapter) open() (*sql.DB, error) {
	db, err := sql.Open("sqlite", a.dsn())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// markLocked classifies a failure: locked databases flip the adapter into the
// unavailable state (one warning, no sessions, no error, spec §4) and report
// true. Other failures report false and surface as errors.
func (a *Adapter) markLocked(err error) bool {
	if isLockedErr(err) {
		a.locked = true
		return true
	}
	return false
}

// sqlite result codes of interest: 5 SQLITE_BUSY, 6 SQLITE_LOCKED.
const (
	sqliteBusy   = 5
	sqliteLocked = 6
)

// isLockedErr reports whether err is (or wraps) a SQLITE_BUSY/SQLITE_LOCKED
// failure, via the driver's error code when available, with a message
// fallback for other drivers.
func isLockedErr(err error) bool {
	if err == nil {
		return false
	}
	var coder interface{ Code() int }
	if errors.As(err, &coder) {
		switch coder.Code() {
		case sqliteBusy, sqliteLocked:
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "locked") || strings.Contains(msg, "busy")
}

func (a *Adapter) Sessions(iter func(agentlog.Session) error) error {
	return a.walk(func(m agentlog.SessionMeta) error {
		return iter(m.Session)
	})
}

// SessionsMeta implements agentlog.MetaSource: message counts come from the
// same streaming pass that yields sessions.
func (a *Adapter) SessionsMeta(iter func(agentlog.SessionMeta) error) error {
	return a.walk(iter)
}

// walk streams every session (parent and child alike — no filtering) in
// time_created order through iter, with per-session sizes and message counts
// from grouped aggregate cursors.
func (a *Adapter) walk(iter func(agentlog.SessionMeta) error) error {
	if a.dir == "" {
		return nil // storage root absent: no sessions, not an error (spec §8)
	}
	db, err := a.open()
	if err != nil {
		if a.markLocked(err) {
			return nil
		}
		return fmt.Errorf("cannot open opencode database: %v", err)
	}
	defer db.Close()

	sizes, counts, err := a.stats(db)
	if err != nil {
		if a.markLocked(err) {
			return nil
		}
		return fmt.Errorf("cannot read opencode database: %v", err)
	}

	rows, err := db.Query(`SELECT id, directory, title, time_created, time_updated
		FROM session ORDER BY time_created, id`)
	if err != nil {
		if a.markLocked(err) {
			return nil
		}
		return fmt.Errorf("cannot list opencode sessions: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, directory, title string
		var created, updated int64
		if err := rows.Scan(&id, &directory, &title, &created, &updated); err != nil {
			continue // unreadable row: skip
		}
		m := agentlog.SessionMeta{
			Session: agentlog.Session{
				ID:        id,
				Agent:     agentName,
				Project:   directory,
				Title:     title,
				StartedAt: time.UnixMilli(created),
				EndedAt:   time.UnixMilli(updated),
				SizeBytes: sizes[id],
			},
			Messages: int(counts[id]),
		}
		if err := iter(m); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		if a.markLocked(err) {
			return nil
		}
		return fmt.Errorf("cannot list opencode sessions: %v", err)
	}
	return nil
}

// stats aggregates per-session sizes (byte lengths of the row JSON) and
// message counts (text parts with non-empty text) with three grouped queries.
// The cursors stream group rows into small per-session maps; the grouped
// scans are index-driven (SCHEMA.md) and bounded by the number of sessions.
func (a *Adapter) stats(db *sql.DB) (sizes, counts map[string]int64, err error) {
	sizes = map[string]int64{}
	counts = map[string]int64{}
	if err = scanGroups(db,
		`SELECT session_id, COALESCE(SUM(LENGTH(CAST(data AS BLOB))),0)
		 FROM message GROUP BY session_id`, sizes); err != nil {
		return nil, nil, err
	}
	if err = scanGroups(db,
		`SELECT session_id, COALESCE(SUM(LENGTH(CAST(data AS BLOB))),0)
		 FROM part GROUP BY session_id`, sizes); err != nil {
		return nil, nil, err
	}
	// message counts mirror what Entries yields: text parts with non-empty
	// text. Malformed part.data counts as no message (json_valid guard).
	if err = scanGroups(db,
		`SELECT session_id, COUNT(*)
		 FROM part
		 WHERE json_valid(data)
		   AND json_extract(data,'$.type')='text'
		   AND COALESCE(json_extract(data,'$.text'),'') <> ''
		 GROUP BY session_id`, counts); err != nil {
		return nil, nil, err
	}
	return sizes, counts, nil
}

// scanGroups streams one grouped (session_id, number) query into the map.
func scanGroups(db *sql.DB, query string, into map[string]int64) error {
	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int64
		if err := rows.Scan(&id, &n); err != nil {
			continue // unreadable row: skip
		}
		into[id] += n
	}
	return rows.Err()
}

// Entries streams one session's entries from a single ordered cursor: rows
// join message → part, ordered by (message.time_created, message.id) and
// (part.time_created, part.id) within each message (SCHEMA.md). The cursor
// is closed on every path, including early callback errors.
func (a *Adapter) Entries(s agentlog.Session, iter func(agentlog.Entry) error) error {
	if a.dir == "" {
		return nil // storage root absent: nothing to read, not an error
	}
	db, err := a.open()
	if err != nil {
		if a.markLocked(err) {
			return nil
		}
		return fmt.Errorf("cannot open opencode database: %v", err)
	}
	defer db.Close()

	var exists int
	if err := db.QueryRow(`SELECT 1 FROM session WHERE id = ?`, s.ID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("session %s not found in opencode storage", s.ID)
		}
		if a.markLocked(err) {
			return nil
		}
		return fmt.Errorf("cannot read opencode session %s: %v", s.ID, err)
	}

	rows, err := db.Query(`SELECT m.id, m.data, p.data, p.time_created
		FROM message m LEFT JOIN part p ON p.message_id = m.id
		WHERE m.session_id = ?
		ORDER BY m.time_created, m.id, p.time_created, p.id`, s.ID)
	if err != nil {
		if a.markLocked(err) {
			return nil
		}
		return fmt.Errorf("cannot read opencode session %s: %v", s.ID, err)
	}
	defer rows.Close()

	var lastMsgID string
	var role string
	seen := false
	for rows.Next() {
		var msgID string
		var msgData, partData sql.NullString
		var partCreated sql.NullInt64
		if err := rows.Scan(&msgID, &msgData, &partData, &partCreated); err != nil {
			continue // unreadable row: skip
		}
		if !seen || msgID != lastMsgID {
			var ok bool
			role, ok = messageRole(msgData.String)
			if !ok {
				// unparseable message.data: counts as skipped (spec §8
				// letter, M4-B17); its parts still stream below, so no
				// content is dropped — only the role degrades to ""
				a.skipped++
			}
			lastMsgID = msgID
			seen = true
		}
		if !partData.Valid {
			// message without any parts (LEFT JOIN null row): recognized
			// structure, nothing to record — never counted as skipped
			// (controller ruling on spec §8)
			continue
		}
		var ts time.Time
		if partCreated.Valid {
			ts = time.UnixMilli(partCreated.Int64)
		}
		entries, skip := partToEntries(role, partData.String, ts)
		if skip {
			a.skipped++
		}
		for _, e := range entries {
			if err := iter(e); err != nil {
				return err
			}
		}
	}
	if err := rows.Err(); err != nil {
		if a.markLocked(err) {
			return nil
		}
		return fmt.Errorf("cannot read opencode session %s: %v", s.ID, err)
	}
	return nil
}

// messageRole extracts the role from a message.data JSON payload. ok=false
// means data is not valid JSON: the row counts as skipped (M4-B17). A
// parseable payload without a role field yields ok=true with "" — Entry.Role
// is best effort per spec §5.
func messageRole(data string) (role string, ok bool) {
	if strings.TrimSpace(data) == "" {
		return "", true // empty payload: readable, nothing to extract
	}
	var md struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal([]byte(data), &md); err != nil {
		return "", false
	}
	return md.Role, true
}
