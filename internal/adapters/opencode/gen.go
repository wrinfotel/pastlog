package opencode

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	// the generator builds fixture databases with the same pure-Go driver the
	// adapter reads with (CGO off)
	_ "modernc.org/sqlite"
)

// GenerateTestDB creates a synthetic OpenCode database at
// <dir>/opencode.db using the schema observed on a real database (see
// SCHEMA.md) and anonymized fixture data. It is a test helper (spec §9: no
// binary .db is committed, and no real user data ever enters fixtures). It
// creates <dir> when needed and returns the created database path.
func GenerateTestDB(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
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
// unknown type and a malformed row for the defensive-parsing paths. Both
// sessions carry non-zero aggregate token columns, a cost and a model (M7
// token statistics); the child's zero cache_read column exercises a 0 value.
func seed(db *sql.DB) error {
	stmts := []string{
		// sessions (times are fixed epoch milliseconds for determinism)
		`INSERT INTO session (id, project_id, workspace_id, parent_id, slug, directory, title, version, time_created, time_updated,
			tokens_input, tokens_output, tokens_reasoning, tokens_cache_read, tokens_cache_write, cost, model)
		 VALUES ('ses_fixture100001fix', 'prj_fixture', NULL, NULL, 'fixture-one', 'C:/dev/fixture app', 'Fix the widget flux', '1.18.0', 1786220928012, 1786220992255,
			1523, 412, 87, 10240, 512, 0.42, 'qwen3-coder-480b')`,
		`INSERT INTO session (id, project_id, workspace_id, parent_id, slug, directory, title, version, time_created, time_updated,
			tokens_input, tokens_output, tokens_reasoning, tokens_cache_read, tokens_cache_write, cost, model)
		 VALUES ('ses_fixture200002fix', 'prj_fixture', NULL, 'ses_fixture100001fix', 'fixture-two', 'C:/dev/fixture app', 'Child subagent session', '1.18.0', 1786220993000, 1786220994000,
			310, 95, 20, 0, 128, 0.08, 'qwen3-coder-480b')`,
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
		// unparseable message.data (M4): the role cannot be extracted, the
		// row counts as skipped, and its parts still stream with an empty
		// best-effort role (content is never dropped)
		`INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES
		 ('msg_fixture200002fi', 'ses_fixture200002fix', 1786220993950, 1786220993950,
		  'not-json{{{')`,
		// empty-text part under the broken message: nothing to record, no
		// extra message count (the stats query needs non-empty text)
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
		 ('prt_fixture200002fi', 'msg_fixture200002fi', 'ses_fixture200002fix', 1786220993960, 1786220993960,
		  '{"type":"text","text":""}')`,
		// message without any parts (e.g. aborted turn): its LEFT JOIN row
		// carries a NULL part — recognized structure, must not count as skipped
		`INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES
		 ('msg_fixture100003fi', 'ses_fixture100001fix', 1786220992300, 1786220992400,
		  '{"role":"assistant","finish":"aborted"}')`,
		// parent session parts, in transcript order
		// user text
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
		 ('prt_fixture100001fi', 'msg_fixture100001fi', 'ses_fixture100001fix', 1786220928154, 1786220928154,
		  '{"type":"text","text":"the widget flux calibration keeps drifting"}')`,
		// assistant: step-start, text, reasoning, tool with output, tool
		// without output, file (dropped silently), patch (dropped silently),
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
		// compaction record (dropped silently)
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

// GenerateContextFixtureDB creates a database holding the SPEC §6.6 golden
// context session under the given id: exact per-turn usage, a repeated read
// (R2), an oversized tool result (R1), an error loop (R3), a compaction
// boundary (R4), and a big-turn jump (R5). The per-turn window sums mirror
// the claude-code fixture in internal/cli/context_test.go (1500, 2700,
// 3100, 60000, 62000, 64000, 18000, 19000) so the cross-agent parity test
// in internal/cli can assert identical findings for all five agents. It
// creates <dir> when needed and returns the created database path.
func GenerateContextFixtureDB(dir, sessionID string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, dbName)
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		return "", err
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		return "", fmt.Errorf("create schema: %w", err)
	}
	stmt := func(q string) error {
		_, err := db.Exec(q)
		return err
	}
	step := func(input int64) string {
		return fmt.Sprintf(`{"type":"step-finish","tokens":{"input":%d,"output":40,"cache":{"read":0,"write":0}}}`, input)
	}
	tool := func(name, inputJSON, output, status string) string {
		return fmt.Sprintf(`{"type":"tool","tool":"%s","state":{"status":"%s","input":%s,"output":%s}}`,
			name, status, inputJSON, quoteJSONString(output))
	}
	giant := strings.Repeat("building package github.com/dev/app/internal ...\n", 400)
	fail := "FAIL github.com/dev/app 0.5s\nbuild failed: undefined: Config"

	ms := 1786220990000
	next := func() int { ms += 500; return ms }
	partN := 0
	msg := func(id, role string, parts ...string) error {
		next()
		if err := stmt(fmt.Sprintf(`INSERT INTO message (id, session_id, time_created, time_updated, data)
			VALUES ('%s', '%s', %d, %d, '{"role":"%s"}')`, id, sessionID, ms, ms, role)); err != nil {
			return err
		}
		for _, data := range parts {
			next()
			partN++
			if err := stmt(fmt.Sprintf(`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data)
				VALUES ('prt_ctx%03d', '%s', '%s', %d, %d, '%s')`, partN, id, sessionID, ms, ms, data)); err != nil {
				return err
			}
		}
		return nil
	}

	if err := stmt(`INSERT INTO session (id, project_id, slug, directory, title, version, time_created, time_updated)
		VALUES ('` + sessionID + `', 'proj_ctx', 'ctx-fixture', '/home/dev/app', 'Debug flaky build', '1.0', 1786220990000, 1786220995000)`); err != nil {
		return "", err
	}
	turns := []struct {
		msg   string
		tool  string
		input string
		out   string
		st    string
		step  int64
	}{
		{"msg_ctx02", "Read", `{"file_path":"/home/dev/app/main.go"}`, "package main\nfunc main() {}\n", "completed", 1500},
		{"msg_ctx03", "Bash", `{"command":"go test ./..."}`, fail, "completed", 2700},
		{"msg_ctx04", "Bash", `{"command":"go build -v ./... 2>&1 | tee /tmp/build.log; cat /tmp/build.log"}`, giant, "completed", 3100},
		{"msg_ctx05", "Bash", `{"command":"go test ./..."}`, fail, "error", 60000},
		{"msg_ctx06", "Bash", `{"command":"go test ./..."}`, fail, "error", 62000},
		{"msg_ctx07", "Bash", `{"command":"go test ./..."}`, fail, "error", 64000},
		{"msg_ctx09", "Read", `{"file_path":"/home/dev/app/main.go"}`, "package main\nfunc main() { config := Config() }\n", "completed", 19000},
	}
	if err := msg("msg_ctx01", "user", `{"type":"text","text":"build is failing, look into it"}`); err != nil {
		return "", err
	}
	for _, t := range turns[:6] {
		if err := msg(t.msg, "assistant", tool(t.tool, t.input, t.out, t.st), step(t.step)); err != nil {
			return "", err
		}
	}
	// compaction part, then usage drops (R4)
	if err := msg("msg_ctx08", "assistant", `{"type":"compaction"}`, step(18000)); err != nil {
		return "", err
	}
	// re-read of main.go after edits (R2: turn gap between t1 and t9)
	if err := msg("msg_ctx09", "assistant",
		tool("Read", `{"file_path":"/home/dev/app/main.go"}`, "package main\nfunc main() { config := Config() }\n", "completed"),
		step(19000)); err != nil {
		return "", err
	}
	return path, nil
}

// quoteJSONString encodes s as one JSON string literal.
func quoteJSONString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}
