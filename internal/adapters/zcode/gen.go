package zcode

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	// the generator builds fixture databases with the same pure-Go driver the
	// adapter reads with (CGO off)
	_ "modernc.org/sqlite"
)

// GenerateTestDB creates a synthetic ZCode database at <dir>/db.sqlite using
// the schema observed on a real database (see SCHEMA.md) and anonymized
// fixture data. It is a test helper (spec §9: no binary .db is committed, and
// no real user data ever enters fixtures). It creates <dir> when needed and
// returns the created database path.
func GenerateTestDB(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "db.sqlite")
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

// schema replicates the observed ZCode tables pastlog reads (the full
// observed column lists; see SCHEMA.md), with the observed indexes on
// session_id.
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
  ` + "`revert`" + ` text,
  ` + "`permission`" + ` text,
  ` + "`time_created`" + ` integer NOT NULL,
  ` + "`time_updated`" + ` integer NOT NULL,
  ` + "`time_compacting`" + ` integer,
  ` + "`time_archived`" + ` integer,
  ` + "`task_type`" + ` text NOT NULL,
  ` + "`title_source`" + ` text NOT NULL,
  ` + "`title_message_id`" + ` text,
  ` + "`time_title_updated`" + ` integer,
  ` + "`trace_id`" + ` text
);
CREATE TABLE ` + "`message`" + ` (
  ` + "`id`" + ` text PRIMARY KEY,
  ` + "`session_id`" + ` text NOT NULL,
  ` + "`time_created`" + ` integer NOT NULL,
  ` + "`time_updated`" + ` integer NOT NULL,
  ` + "`data`" + ` text NOT NULL,
  ` + "`sequence`" + ` integer
);
CREATE TABLE ` + "`part`" + ` (
  ` + "`id`" + ` text PRIMARY KEY,
  ` + "`message_id`" + ` text NOT NULL,
  ` + "`session_id`" + ` text NOT NULL,
  ` + "`time_created`" + ` integer NOT NULL,
  ` + "`time_updated`" + ` integer NOT NULL,
  ` + "`data`" + ` text NOT NULL,
  ` + "`sequence`" + ` integer
);
CREATE TABLE ` + "`model_usage`" + ` (
  ` + "`id`" + ` text PRIMARY KEY,
  ` + "`logical_request_id`" + ` text NOT NULL,
  ` + "`attempt_index`" + ` integer NOT NULL,
  ` + "`session_id`" + ` text NOT NULL,
  ` + "`turn_id`" + ` text,
  ` + "`trace_id`" + ` text,
  ` + "`span_id`" + ` text,
  ` + "`assistant_message_id`" + ` text,
  ` + "`parent_user_message_id`" + ` text,
  ` + "`query_source`" + ` text NOT NULL,
  ` + "`provider_id`" + ` text NOT NULL,
  ` + "`model_id`" + ` text NOT NULL,
  ` + "`variant`" + ` text,
  ` + "`agent`" + ` text,
  ` + "`mode`" + ` text,
  ` + "`task_type`" + ` text,
  ` + "`status`" + ` text NOT NULL,
  ` + "`started_at`" + ` integer NOT NULL,
  ` + "`first_token_at`" + ` integer,
  ` + "`completed_at`" + ` integer,
  ` + "`duration_ms`" + ` integer,
  ` + "`time_to_first_token_ms`" + ` integer,
  ` + "`finish_reason`" + ` text,
  ` + "`tool_call_count`" + ` integer NOT NULL,
  ` + "`input_tokens`" + ` integer NOT NULL,
  ` + "`output_tokens`" + ` integer NOT NULL,
  ` + "`reasoning_tokens`" + ` integer NOT NULL,
  ` + "`cache_creation_input_tokens`" + ` integer NOT NULL,
  ` + "`cache_read_input_tokens`" + ` integer NOT NULL,
  ` + "`provider_total_tokens`" + ` integer,
  ` + "`computed_total_tokens`" + ` integer NOT NULL,
  ` + "`retry_count`" + ` integer NOT NULL,
  ` + "`retryable`" + ` integer NOT NULL,
  ` + "`cancelled_by_user`" + ` integer NOT NULL,
  ` + "`context_exceeded`" + ` integer NOT NULL,
  ` + "`error_type`" + ` text,
  ` + "`error_code`" + ` text,
  ` + "`error_message`" + ` text,
  ` + "`raw_usage_json`" + ` text,
  ` + "`provider_metadata_json`" + ` text
);
CREATE INDEX ` + "`message_session_time_created_id_idx`" + ` ON ` + "`message`" + ` (` + "`session_id`" + `, ` + "`time_created`" + `, ` + "`id`" + `);
CREATE INDEX ` + "`part_session_idx`" + ` ON ` + "`part`" + ` (` + "`session_id`" + `);
CREATE INDEX ` + "`part_message_id_id_idx`" + ` ON ` + "`part`" + ` (` + "`message_id`" + `, ` + "`id`" + `);
CREATE INDEX ` + "`model_usage_session_idx`" + ` ON ` + "`model_usage`" + ` (` + "`session_id`" + `);
`

// seed inserts two sessions (an interactive parent and a subagent_child child
// with parent_id set — both are listed), user/assistant messages, model_usage
// rows with token splits, and one part of every observed type plus an unknown
// type and a malformed row for the defensive-parsing paths.
func seed(db *sql.DB) error {
	stmts := []string{
		// sessions (times are fixed epoch milliseconds for determinism)
		`INSERT INTO session (id, project_id, parent_id, slug, directory, path, title, version,
			time_created, time_updated, task_type, title_source)
		 VALUES ('sess_fixture000001zc', 'proj_fixture-app', NULL, 'sess_fixture000001zc', 'C:/dev/fixture zcode', 'C:/dev/fixture zcode',
		 	'Fix the widget flux', '0.16.9', 1786480128012, 1786480192255, 'interactive', 'generated')`,
		`INSERT INTO session (id, project_id, parent_id, slug, directory, path, title, version,
			time_created, time_updated, task_type, title_source)
		 VALUES ('sess_fixture000002zc', 'proj_fixture-app', 'sess_fixture000001zc', 'sess_fixture000002zc', 'C:/dev/fixture zcode', 'C:/dev/fixture zcode',
		 	'Child subagent session', '0.16.9', 1786480193000, 1786480194000, 'subagent_child', 'first_input')`,
		// messages: user, assistant (parent session); one assistant (child)
		`INSERT INTO message (id, session_id, time_created, time_updated, data, sequence) VALUES
		 ('msg_fixture000001zc', 'sess_fixture000001zc', 1786480128145, 1786480128145,
		  '{"role":"user","time":{"created":1786480128145},"agent":"zcode-agent"}', 0)`,
		`INSERT INTO message (id, session_id, time_created, time_updated, data, sequence) VALUES
		 ('msg_fixture000002zc', 'sess_fixture000001zc', 1786480129200, 1786480192000,
		  '{"role":"assistant","time":{"created":1786480129200,"completed":1786480192000},"modelId":"fixture-model-a","finish":"stop","tokens":{"total":42}}', 1)`,
		`INSERT INTO message (id, session_id, time_created, time_updated, data, sequence) VALUES
		 ('msg_fixture000003zc', 'sess_fixture000001zc', 1786480192300, 1786480192400,
		  '{"role":"assistant","finish":"aborted"}', 2)`,
		`INSERT INTO message (id, session_id, time_created, time_updated, data, sequence) VALUES
		 ('msg_fixture000004zc', 'sess_fixture000002zc', 1786480193100, 1786480193900,
		  '{"role":"assistant","agent":"general"}', 0)`,
		// unparseable message.data (M4): the role cannot be extracted, the
		// row counts as skipped, and its parts still stream with an empty
		// best-effort role (content is never dropped)
		`INSERT INTO message (id, session_id, time_created, time_updated, data, sequence) VALUES
		 ('msg_fixture000005zc', 'sess_fixture000002zc', 1786480193950, 1786480193950,
		  'not-json{{{', 1)`,
		// empty-text part under the broken message: nothing to record, no
		// extra message count (the stats query needs non-empty text)
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data, sequence) VALUES
		 ('prt_fixture000005zc', 'msg_fixture000005zc', 'sess_fixture000002zc', 1786480193960, 1786480193960,
		  '{"type":"text","text":""}', 0)`,
		// message without any parts (e.g. aborted turn): its LEFT JOIN row
		// carries a NULL part — recognized structure, must not count as skipped
		// (controller ruling on spec §8, same as opencode)
		// → msg_fixture000003zc has no parts
		// parent session parts, in transcript order
		// user text
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data, sequence) VALUES
		 ('prt_fixture000001zc', 'msg_fixture000001zc', 'sess_fixture000001zc', 1786480128154, 1786480128154,
		  '{"type":"text","text":"the widget flux calibration keeps drifting","time":{"start":1786480128154}}', 0)`,
		// assistant: step-start, text, reasoning, tool with output, tool
		// without output, file (dropped silently), timeline (dropped
		// silently), compaction (dropped silently), unknown type (skipped,
		// counted), malformed json (skipped, counted)
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data, sequence) VALUES
		 ('prt_fixture000002zc', 'msg_fixture000002zc', 'sess_fixture000001zc', 1786480129201, 1786480129201,
		  '{"type":"step-start"}', 0)`,
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data, sequence) VALUES
		 ('prt_fixture000003zc', 'msg_fixture000002zc', 'sess_fixture000001zc', 1786480129210, 1786480129210,
		  '{"type":"text","text":"recalibrating the flux capacitor now"}', 1)`,
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data, sequence) VALUES
		 ('prt_fixture000004zc', 'msg_fixture000002zc', 'sess_fixture000001zc', 1786480129220, 1786480129220,
		  '{"type":"reasoning","text":"The drift matches the ambient humidity.","time":{"start":1786480129220}}', 2)`,
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data, sequence) VALUES
		 ('prt_fixture000006zc', 'msg_fixture000002zc', 'sess_fixture000001zc', 1786480129230, 1786480129230,
		  '{"type":"tool","callID":"call_fixture1","tool":"Read","state":{"status":"completed","input":{"file_path":"C:/dev/fixture app/flux.ts"},"output":"const flux = calibrate(0.42)"}}', 3)`,
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data, sequence) VALUES
		 ('prt_fixture000007zc', 'msg_fixture000002zc', 'sess_fixture000001zc', 1786480129240, 1786480129240,
		  '{"type":"tool","callID":"call_fixture2","tool":"Edit","state":{"status":"pending","input":{"file_path":"flux.ts"}}}', 4)`,
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data, sequence) VALUES
		 ('prt_fixture000008zc', 'msg_fixture000002zc', 'sess_fixture000001zc', 1786480129250, 1786480129250,
		  '{"type":"file","mime":"text/plain","filename":"notes.txt","url":"data:text/plain;base64,Zml4dHVyZQ=="}', 5)`,
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data, sequence) VALUES
		 ('prt_fixture000009zc', 'msg_fixture000002zc', 'sess_fixture000001zc', 1786480129255, 1786480129255,
		  '{"type":"timeline","events":[]}', 6)`,
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data, sequence) VALUES
		 ('prt_fixture000010zc', 'msg_fixture000002zc', 'sess_fixture000001zc', 1786480192100, 1786480192100,
		  '{"type":"step-finish","tokens":42}', 7)`,
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data, sequence) VALUES
		 ('prt_fixture000011zc', 'msg_fixture000002zc', 'sess_fixture000001zc', 1786480192200, 1786480192200,
		  '{"type":"compaction"}', 8)`,
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data, sequence) VALUES
		 ('prt_fixture000012zc', 'msg_fixture000002zc', 'sess_fixture000001zc', 1786480192210, 1786480192210,
		  '{"type":"hologram","depth":7}', 9)`,
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data, sequence) VALUES
		 ('prt_fixture000013zc', 'msg_fixture000002zc', 'sess_fixture000001zc', 1786480192220, 1786480192220,
		  '{"type":"text","text":truncated', 10)`,
		// child session: one assistant text
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data, sequence) VALUES
		 ('prt_fixture000014zc', 'msg_fixture000004zc', 'sess_fixture000002zc', 1786480193200, 1786480193200,
		  '{"type":"text","text":"child session report: flux stable"}', 0)`,
		// model_usage rows: two requests in the parent session with different
		// models (the later one wins as the session model), one in the child
		`INSERT INTO model_usage (id, logical_request_id, attempt_index, session_id, query_source, provider_id, model_id,
			status, started_at, tool_call_count, input_tokens, output_tokens, reasoning_tokens,
			cache_creation_input_tokens, cache_read_input_tokens, computed_total_tokens, retry_count, retryable, cancelled_by_user, context_exceeded)
		 VALUES ('mru_fixture000001zc', 'lrq_1', 0, 'sess_fixture000001zc', 'interactive', 'fixture-provider',
		 	'fixture-model-a', 'ok', 1786480129000, 2, 100, 20, 5, 30, 200, 355, 0, 0, 0, 0)`,
		`INSERT INTO model_usage (id, logical_request_id, attempt_index, session_id, query_source, provider_id, model_id,
			status, started_at, tool_call_count, input_tokens, output_tokens, reasoning_tokens,
			cache_creation_input_tokens, cache_read_input_tokens, computed_total_tokens, retry_count, retryable, cancelled_by_user, context_exceeded)
		 VALUES ('mru_fixture000002zc', 'lrq_2', 0, 'sess_fixture000001zc', 'interactive', 'fixture-provider',
		 	'fixture-model-b', 'ok', 1786480180000, 0, 50, 10, 0, 0, 80, 140, 0, 0, 0, 0)`,
		`INSERT INTO model_usage (id, logical_request_id, attempt_index, session_id, query_source, provider_id, model_id,
			status, started_at, tool_call_count, input_tokens, output_tokens, reasoning_tokens,
			cache_creation_input_tokens, cache_read_input_tokens, computed_total_tokens, retry_count, retryable, cancelled_by_user, context_exceeded)
		 VALUES ('mru_fixture000003zc', 'lrq_3', 0, 'sess_fixture000002zc', 'subagent', 'fixture-provider',
		 	'fixture-model-c', 'ok', 1786480193500, 0, 10, 2, 1, 4, 16, 33, 0, 0, 0, 0)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}
