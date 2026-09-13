# opencode storage schema (observed: real database probed read-only, 2026-09)

OpenCode keeps one SQLite database with `-wal`/`-shm` side files. pastlog
reads only, and strictly read-only: the DSN is `file:<db>?mode=ro` — any
write attempt is refused by the driver with SQLITE_READONLY, enforced by
`TestNoWritesToDatabase`. The schema below was probed on a real database
(6 sessions, 3450 messages, 11986 parts) with modernc.org/sqlite; all fixture
databases are generated synthetically at test time (`gen.go`, no binary .db
is committed, no real user data in fixtures).

## Location

```
<storage-root>/opencode.db        (+ opencode.db-wal, opencode.db-shm)
```

- `<storage-root>` is resolved by `internal/discovery.OpenCodeDir`:
  `%LOCALAPPDATA%/opencode` then `~/.local/share/opencode` on Windows
  (real installs may resolve XDG themselves even on Windows);
  `$XDG_DATA_HOME/opencode` then `~/.local/share/opencode` on unix.
  First existing candidate wins (controller ruling).
- Detection = the resolved root contains a regular file `opencode.db`.

## Observed schema

Tables pastlog reads (there are others — `account`, `event`, `permission`, …
— not needed for v0.1; `session_message` exists but is empty/legacy):

```sql
CREATE TABLE `session` (
  `id` text PRIMARY KEY,             -- "ses_…"
  `project_id` text NOT NULL,
  `workspace_id` text, `parent_id` text,   -- child (subagent) sessions
  `slug` text NOT NULL,
  `directory` text NOT NULL,         -- project cwd, forward slashes ("C:/Users/x/…")
  `path` text, `title` text NOT NULL, `version` text NOT NULL,
  `share_url` text,                  -- summary_*/tokens_*/cost columns exist
  … , `agent` text, `model` text,
  `time_created` integer NOT NULL,   -- epoch MILLISECONDS
  `time_updated`  integer NOT NULL,  -- epoch MILLISECONDS
  `time_compacting` integer, `time_archived` integer
);
CREATE TABLE `message` (
  `id` text PRIMARY KEY,             -- "msg_…"
  `session_id` text NOT NULL,
  `time_created` integer NOT NULL,   -- epoch ms
  `time_updated`  integer NOT NULL,  -- epoch ms
  `data` text NOT NULL               -- JSON: {"role":"user"|"assistant",
                                     --   "time":{created,completed?},"agent",
                                     --   "model":{modelID,providerID},
                                     --   "path":{cwd,root},"tokens":…,"cost":…,
                                     --   "parentID":…,"mode":…,"finish":…}
);
CREATE TABLE `part` (
  `id` text PRIMARY KEY,             -- "prt_…"
  `message_id` text NOT NULL,
  `session_id` text NOT NULL,
  `time_created` integer NOT NULL,   -- epoch ms
  `time_updated`  integer NOT NULL,  -- epoch ms
  `data` text NOT NULL               -- JSON, see part types below
);
CREATE INDEX `message_session_time_created_id_idx` ON message (session_id, time_created, id);
CREATE INDEX `part_session_idx` ON part (session_id);
CREATE INDEX `part_message_id_id_idx` ON part (message_id, id);
```

Indexes on `session_id` exist, so per-session filtering is index-driven
(verified with `PRAGMA index_list`); session listing itself scans the primary
key. SQLite's JSON1 functions (`json_valid`, `json_extract`, `json_type`) are
available in the pure-Go modernc.org/sqlite build (verified).

### Observed part types (counts from the real database)

| type | count | payload | pastlog entry |
|---|---|---|---|
| `step-start` | 3021 | — | skipped, counted |
| `step-finish` | 2891 | `tokens`, `cost` | skipped, counted |
| `tool` | 2490 | `{type, tool, callID, state:{status, input, output?, title?, time, metadata}}` — `input` a JSON object, `output` a plain string (observed up to ~300 KB) | `ToolCall` role `tool`, text = compact JSON of `state.input` (falls back to the tool name), plus a `ToolResult` role `tool` with the output text when non-empty |
| `text` | 2214 | `{type, text, time?}` | `Message` with the message row's role |
| `reasoning` | 1311 | `{type, text, time}` | `Summary` role `assistant` |
| `file` | 28 | `{type, mime, filename, url}` — url is a `data:` URL up to ~200 KB | skipped, **uncounted** (deliberate: recognized type with no searchable text) |
| `patch` | 16 | `{type, hash, files[]}` | skipped, counted |
| `compaction` | 15 | — | skipped, counted |
| unknown | 0 | — | skipped, counted |

## Mapping to pastlog's model

- `Session.ID` = `session.id`.
- `Session.Project` = `session.directory` (kept verbatim, forward slashes).
- `Session.Title` = `session.title`.
- `Session.StartedAt` / `EndedAt` = `time_created` / `time_updated`
  (epoch milliseconds, UTC).
- `Session.SizeBytes` ≈ sum over the session's rows of the byte length of
  `message.data` + `part.data`, computed as `LENGTH(CAST(data AS BLOB))`
  (byte-accurate; plain `LENGTH` would count UTF-8 characters and understate
  non-ASCII content). Listing therefore scans row payloads once per grouped
  aggregate — the accepted cost of the mandated accounting.
- `Messages` = text parts with non-empty text (computed with SQLite JSON
  functions, guarded by `json_valid`; mirrors what `Entries` yields —
  malformed part.data counts as no message).
- Child sessions (`parent_id` set) are listed like any other session — no
  filtering (controller brief).
- Entries iteration: one ordered cursor per session —
  `message LEFT JOIN part` ordered by `(message.time_created, message.id)`
  then `(part.time_created, part.id)`; verified on the real database that
  this yields a coherent transcript (no timestamp inversions within a
  session). Row payloads are handed to the callback incrementally; the
  `*sql.Rows` cursor is closed on every path, including early callback
  errors. `Entries` scopes every query by `session_id`.
- Entry timestamps = `part.time_created`.

## `agents` totals (controller ruling)

- `agents` total size = the on-disk sizes of `opencode.db` + `opencode.db-wal`
  + `opencode.db-shm` (`TotalBytes` via `agentlog.TotalSizer`), not the sum
  of session sizes (the sessions share one database file).

## Locked database (spec §4)

The DSN uses a short `_pragma=busy_timeout(200)` so a momentarily blocked
file is not misread as a locked database. On SQLITE_BUSY / SQLITE_LOCKED
(either at open, first probe, or mid-scan) the adapter:

- marks itself unavailable (`Warning()` returns one lowercase, actionable
  stderr line, printed once per CLI run via `agentlog.WarningSource`),
- yields zero sessions/entries and returns no error, so listing, search and
  exit codes continue unaffected with the other agents,
- stays `Detect()`-true (the storage exists; it is merely locked).

Covered by `TestLockedDBWarnsAndContinues` / `TestLockedDBEntriesNoError`
(second connection holding `BEGIN EXCLUSIVE`) and the CLI-level test.

## Defensive behavior

- Malformed `part.data` / `message.data` JSON → skipped, counted
  (`SkippedLines()`; the CLI summarizes as "N unreadable lines skipped" —
  kept from M2 for all adapters).
- Unknown part types and patch/step-*/compaction records → skipped, counted.
- `file` parts → skipped, uncounted (deliberate, see table above).
- Unreadable rows (unexpected NULLs) are skipped silently.
- Iteration callbacks may abort the stream by returning an error; the error
  is propagated unchanged and the cursor is closed.

## Streaming

`Sessions`/`SessionsMeta` stream one `SELECT … ORDER BY` cursor over
`session` plus three grouped aggregate cursors (streamed into per-session
maps, never whole tables). `Entries` streams one joined cursor per session.
Nothing materializes a full result set — spec §5/§7. The M2 raw-line search
prefilter does not apply to SQL rows (no `LineFilteredAdapter` here); SQL
LIKE is not used, matching stays in Go via the existing matcher path
(controller ruling).
