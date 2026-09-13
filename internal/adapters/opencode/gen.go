package opencode

import (
	"database/sql"
	"fmt"
	"path/filepath"

	// the generator builds fixture databases with the same pure-Go driver the
	// adapter reads with (CGO off)
	_ "modernc.org/sqlite"
)

// GenerateTestDB creates a synthetic OpenCode database at
// <dir>/opencode.db using the schema observed on a real database (see
// SCHEMA.md) and anonymized fixture data. It is a test helper (spec §9: no
// binary .db is committed, and no real user data ever enters fixtures). It
// returns the created database path.
func GenerateTestDB(dir string) (string, error) {
	path := filepath.Join(dir, "opencode.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		return "", err
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		return "", fmt.Errorf("create schema: %w", err)
	}
	if err := seed(db); err != nil {
		return "", fmt.Errorf("seed: %w", err)
	}
	return path, nil
}

// GenerateTestDBAt is GenerateTestDB for tests: it fails the test on error.
// (kept minimal so non-test packages can reuse the generator)

// schema replicates the observed OpenCode tables pastlog reads, with the
// observed column names and the observed indexes on session_id.
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
CREATE INDEX ` + "`message_session_time_created_id_idx`" + ` ON ` + "`message`" + ` (` + "`session_id`" + `, ` + "`time_created`" + `, ` + "`id`" + `);
CREATE INDEX ` + "`part_session_idx`" + ` ON ` + "`part`" + ` (` + "`session_id`" + `);
CREATE INDEX ` + "`part_message_id_id_idx`" + ` ON ` + "`part`" + ` (` + "`message_id`" + `, ` + "`id`" + `);
`

// seed inserts two sessions (one child, proving parent sessions are listed
// too), user/assistant messages, and one part of every observed type plus an
// unknown type and a malformed row for the defensive-parsing paths.
func seed(db *sql.DB) error {
	stmts := []string{
		// sessions (times are fixed epoch milliseconds for determinism)
		`INSERT INTO session (id, project_id, workspace_id, parent_id, slug, directory, title, version, time_created, time_updated)
		 VALUES ('ses_fixture100001fix', 'prj_fixture', NULL, NULL, 'fixture-one', 'C:/dev/fixture app', 'Fix the widget flux', '1.18.0', 1786220928012, 1786220992255)`,
		`INSERT INTO session (id, project_id, workspace_id, parent_id, slug, directory, title, version, time_created, time_updated)
		 VALUES ('ses_fixture200002fix', 'prj_fixture', NULL, 'ses_fixture100001fix', 'fixture-two', 'C:/dev/fixture app', 'Child subagent session', '1.18.0', 1786220993000, 1786220994000)`,
		// messages: user, assistant (parent session); one assistant (child)
		`INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES
		 ('msg_fixture100001fi', 'ses_fixture100001fix', 1786220928145, 1786220928145,
		  '{"role":"user","time":{"created":1786220928145},"agent":"build"}')`,
		`INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES
		 ('msg_fixture100002fi', 'ses_fixture100001fix', 1786220929200, 1786220992000,
		  '{"role":"assistant","mode":"build","agent":"build","path":{"cwd":"C:\\\\dev\\\\fixture app"},"tokens":{"total":42},"cost":0}')`,
		`INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES
		 ('msg_fixture200001fi', 'ses_fixture200002fix', 1786220993100, 1786220993900,
		  '{"role":"assistant","agent":"general"}')`,
		// parent session parts, in transcript order
		// user text
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
		 ('prt_fixture100001fi', 'msg_fixture100001fi', 'ses_fixture100001fix', 1786220928154, 1786220928154,
		  '{"type":"text","text":"the widget flux calibration keeps drifting"}')`,
		// assistant: step-start, text, reasoning, tool with output, tool
		// without output, file (skipped, uncounted), patch (skipped, counted),
		// unknown type (skipped, counted), malformed json (skipped, counted)
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
		 ('prt_fixture100002fi', 'msg_fixture100002fi', 'ses_fixture100001fix', 1786220929201, 1786220929201,
		  '{"type":"step-start"}')`,
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
		 ('prt_fixture100003fi', 'msg_fixture100002fi', 'ses_fixture100001fix', 1786220929210, 1786220929210,
		  '{"type":"text","text":"recalibrating the flux capacitor now"}')`,
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
		 ('prt_fixture100004fi', 'msg_fixture100002fi', 'ses_fixture100001fix', 1786220929220, 1786220929220,
		  '{"type":"reasoning","text":"The drift matches the ambient humidity.","time":{"start":1786220929220}}')`,
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
		 ('prt_fixture100005fi', 'msg_fixture100002fi', 'ses_fixture100001fix', 1786220929230, 1786220929230,
		  '{"type":"tool","tool":"read","callID":"call_fixture1","state":{"status":"completed","input":{"filePath":"C:/dev/fixture app/flux.ts"},"output":"const flux = calibrate(0.42)","title":"flux.ts","time":{}}}')`,
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
		 ('prt_fixture100006fi', 'msg_fixture100002fi', 'ses_fixture100001fix', 1786220929240, 1786220929240,
		  '{"type":"tool","tool":"edit","callID":"call_fixture2","state":{"status":"pending","input":{"filePath":"flux.ts"}}}')`,
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
		 ('prt_fixture100007fi', 'msg_fixture100002fi', 'ses_fixture100001fix', 1786220929250, 1786220929250,
		  '{"type":"file","mime":"text/plain","filename":"notes.txt","url":"data:text/plain;base64,Zml4dHVyZQ=="}')`,
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
		 ('prt_fixture100008fi', 'msg_fixture100002fi', 'ses_fixture100001fix', 1786220929260, 1786220929260,
		  '{"type":"patch","hash":"abc123","files":["C:/dev/fixture app/flux.ts"]}')`,
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
		 ('prt_fixture100009fi', 'msg_fixture100002fi', 'ses_fixture100001fix', 1786220929270, 1786220929270,
		  '{"type":"hologram","depth":7}')`,
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
		 ('prt_fixture100010fi', 'msg_fixture100002fi', 'ses_fixture100001fix', 1786220929280, 1786220929280,
		  '{"type":"text","text":truncated')`,
		// assistant step-finish in the parent session
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
		 ('prt_fixture100011fi', 'msg_fixture100002fi', 'ses_fixture100001fix', 1786220992100, 1786220992100,
		  '{"type":"step-finish","tokens":42}')`,
		// compaction record (skipped, counted)
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
		 ('prt_fixture100012fi', 'msg_fixture100002fi', 'ses_fixture100001fix', 1786220992200, 1786220992200,
		  '{"type":"compaction"}')`,
		// child session: one assistant text
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
		 ('prt_fixture200001fi', 'msg_fixture200001fi', 'ses_fixture200002fix', 1786220993200, 1786220993200,
		  '{"type":"text","text":"child session report: flux stable"}')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}
