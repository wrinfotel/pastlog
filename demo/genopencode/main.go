// Command genopencode regenerates the synthetic OpenCode database for the
// demo fixture home: demo/fixture-home/.local/share/opencode/opencode.db.
// No binary .db is committed (spec §9) — the store is created at demo render
// time from this committed source:
//
//	go run ./demo/genopencode   # from the repo root, before `vhs demo/demo.tape`
//
// The seed is deterministic and anonymized (see demo/README.md): two sessions
// whose rows reproduce exactly the opencode lines of the README quickstart
// blocks (`pastlog`, `agents`, `sessions`). The schema mirrors the observed
// OpenCode tables (internal/adapters/opencode/SCHEMA.md) minus the declared
// perf indexes: the adapter reads this store with grouped scans that need no
// index, and the un-indexed tiny database lands at the 7-page (28 KiB)
// footprint the README `agents` output shows.
package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	// the generator builds the fixture database with the same pure-Go driver
	// the adapter reads with (CGO off)
	_ "modernc.org/sqlite"
)

func main() {
	dir := ""
	if len(os.Args) > 1 {
		dir = os.Args[1]
	} else {
		dir = defaultDir()
	}
	path, err := generate(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "genopencode:", err)
		os.Exit(1)
	}
	fmt.Println(path)
}

// defaultDir resolves the fixture store relative to this file, so the
// generator works from any working directory.
func defaultDir() string {
	_, file, _, ok := runtime.Caller(0) // <repo>/demo/genopencode/main.go
	if !ok {
		return filepath.Join("demo", "fixture-home", ".local", "share", "opencode")
	}
	repo := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	return filepath.Join(repo, "demo", "fixture-home", ".local", "share", "opencode")
}

// generate rebuilds <dir>/opencode.db from scratch (an existing store from a
// previous render is replaced) and returns the database path.
func generate(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "opencode.db")
	for _, name := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		return "", err
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		return "", fmt.Errorf("create schema: %v", err)
	}
	if err := seed(db); err != nil {
		return "", fmt.Errorf("seed: %v", err)
	}
	return path, nil
}

// schema replicates the observed OpenCode tables pastlog reads, minus the
// declared indexes (see the package comment).
const schema = `
CREATE TABLE ` + "`session`" + ` (
  ` + "`id`" + ` text PRIMARY KEY,
  ` + "`project_id`" + ` text NOT NULL,
  ` + "`workspace_id`" + ` text,
  ` + "`parent_id`" + ` text,
  ` + "`slug`" + ` text NOT NULL,
  ` + "`directory`" + ` text NOT NULL,
  ` + "`path`" + ` text,
  ` + "`title`" + ` text NOT NULL,
  ` + "`version`" + ` text NOT NULL,
  ` + "`share_url`" + ` text,
  ` + "`summary_additions`" + ` integer,
  ` + "`summary_deletions`" + ` integer,
  ` + "`summary_files`" + ` integer,
  ` + "`summary_diffs`" + ` text,
  ` + "`metadata`" + ` text,
  ` + "`cost`" + ` real DEFAULT 0 NOT NULL,
  ` + "`tokens_input`" + ` integer DEFAULT 0 NOT NULL,
  ` + "`tokens_output`" + ` integer DEFAULT 0 NOT NULL,
  ` + "`tokens_reasoning`" + ` integer DEFAULT 0 NOT NULL,
  ` + "`tokens_cache_read`" + ` integer DEFAULT 0 NOT NULL,
  ` + "`tokens_cache_write`" + ` integer DEFAULT 0 NOT NULL,
  ` + "`revert`" + ` text,
  ` + "`permission`" + ` text,
  ` + "`agent`" + ` text,
  ` + "`model`" + ` text,
  ` + "`time_created`" + ` integer NOT NULL,
  ` + "`time_updated`" + ` integer NOT NULL,
  ` + "`time_compacting`" + ` integer,
  ` + "`time_archived`" + ` integer,
  CONSTRAINT ` + "`fk_session_project_id_project_id_fk`" + ` FOREIGN KEY (` + "`project_id`" + `) REFERENCES ` + "`project`" + `(` + "`id`" + `) ON DELETE CASCADE
);
CREATE TABLE ` + "`message`" + ` (
  ` + "`id`" + ` text PRIMARY KEY,
  ` + "`session_id`" + ` text NOT NULL,
  ` + "`time_created`" + ` integer NOT NULL,
  ` + "`time_updated`" + ` integer NOT NULL,
  ` + "`data`" + ` text NOT NULL,
  CONSTRAINT ` + "`fk_message_session_id_session_id_fk`" + ` FOREIGN KEY (` + "`session_id`" + `) REFERENCES ` + "`session`" + `(` + "`id`" + `) ON DELETE CASCADE
);
CREATE TABLE ` + "`part`" + ` (
  ` + "`id`" + ` text PRIMARY KEY,
  ` + "`message_id`" + ` text NOT NULL,
  ` + "`session_id`" + ` text NOT NULL,
  ` + "`time_created`" + ` integer NOT NULL,
  ` + "`time_updated`" + ` integer NOT NULL,
  ` + "`data`" + ` text NOT NULL,
  CONSTRAINT ` + "`fk_part_message_id_message_id_fk`" + ` FOREIGN KEY (` + "`message_id`" + `) REFERENCES ` + "`message`" + `(` + "`id`" + `) ON DELETE CASCADE
);
`

// The two sessions, fixed for reproducibility. Their ids keep the ID prefixes
// the README quickstart shows; the epochs reproduce the README dates.
const (
	sessionAPI   = "7c1e9a0f-7777-4777-8777-777777777777"
	sessionApp   = "2f8d6b3a-8888-4888-8888-888888888888"
	msgA1        = "msg_demoa1"
	msgA2        = "msg_demoa2"
	msgB1        = "msg_demob1"
	partA1       = "prt_demoa1"
	partA2       = "prt_demoa2"
	partB1       = "prt_demob1"
	msgDataUser  = `{"role":"user"}`
	msgDataAsst  = `{"role":"assistant"}`
	textA1       = "the api key rotation shipped to production"
	textA2       = "rotated the signing keys in each region; the previous key keeps verifying old tokens until the grace window closes this week"
	textB1       = "the deploy finished clean, error rate back to its baseline"
	titleA       = "api key rotation"
	titleB       = "deploy error rate"
	slugA        = "demo-api"
	slugB        = "demo-app"
	versionV     = "1.18.0"
	directoryAPI = "/home/dev/api"
	directoryApp = "/home/dev/myapp"
)

// textPart builds a text part payload; its byte length feeds the per-session
// size the `sessions` output shows, so the texts above are length-pinned
// (a part payload is 25 bytes + text: session A rows sum to 251 bytes,
// session B to 98 — the README numbers).
func textPart(text string) string {
	return `{"type":"text","text":"` + text + `"}`
}

func seed(db *sql.DB) error {
	startA := time.Date(2026, 7, 29, 16, 48, 0, 0, time.UTC)
	startB := time.Date(2026, 7, 28, 16, 35, 0, 0, time.UTC)
	endA := startA.Add(90 * time.Second)
	endB := startB.Add(60 * time.Second)

	stmts := []struct {
		query string
		args  []any
	}{
		{
			`INSERT INTO session (id, project_id, workspace_id, parent_id, slug, directory, title, version, time_created, time_updated)
		  VALUES (?, 'prj_demo', NULL, NULL, ?, ?, ?, ?, ?, ?)`,
			[]any{sessionAPI, slugA, directoryAPI, titleA, versionV, startA.UnixMilli(), endA.UnixMilli()},
		},
		{
			`INSERT INTO session (id, project_id, workspace_id, parent_id, slug, directory, title, version, time_created, time_updated)
		  VALUES (?, 'prj_demo', NULL, NULL, ?, ?, ?, ?, ?, ?)`,
			[]any{sessionApp, slugB, directoryApp, titleB, versionV, startB.UnixMilli(), endB.UnixMilli()},
		},
		// session A: two messages (user + assistant)
		{
			`INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?)`,
			[]any{msgA1, sessionAPI, startA.UnixMilli(), startA.UnixMilli(), msgDataUser},
		},
		{
			`INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?)`,
			[]any{msgA2, sessionAPI, startA.Add(30 * time.Second).UnixMilli(), startA.Add(40 * time.Second).UnixMilli(), msgDataAsst},
		},
		{
			`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?, ?)`,
			[]any{partA1, msgA1, sessionAPI, startA.Add(time.Second).UnixMilli(), startA.Add(time.Second).UnixMilli(), textPart(textA1)},
		},
		{
			`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?, ?)`,
			[]any{partA2, msgA2, sessionAPI, startA.Add(35 * time.Second).UnixMilli(), startA.Add(35 * time.Second).UnixMilli(), textPart(textA2)},
		},
		// session B: one user message
		{
			`INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?)`,
			[]any{msgB1, sessionApp, startB.UnixMilli(), startB.UnixMilli(), msgDataUser},
		},
		{
			`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?, ?)`,
			[]any{partB1, msgB1, sessionApp, startB.Add(5 * time.Second).UnixMilli(), startB.Add(5 * time.Second).UnixMilli(), textPart(textB1)},
		},
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt.query, stmt.args...); err != nil {
			return err
		}
	}
	return nil
}
