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
| `step-start` | 3021 | — | dropped silently (recognized, no entry) |
| `step-finish` | 2891 | `tokens`, `cost` | dropped silently (recognized, no entry) |
| `tool` | 2490 | `{type, tool, callID, state:{status, input, output?, title?, time, metadata}}` — `input` a JSON object, `output` a plain string (observed up to ~300 KB) | `ToolCall` role `tool`, text = compact JSON of `state.input` (falls back to the tool name), plus a `ToolResult` role `tool` with the output text when non-empty |
| `text` | 2214 | `{type, text, time?}` | `Message` with the message row's role |
| `reasoning` | 1311 | `{type, text, time}` | `Summary` role `assistant` |
| `file` | 28 | `{type, mime, filename, url}` — url is a `data:` URL up to ~200 KB | dropped silently (recognized, no entry — its url is a data: URL, not searchable text) |
| `patch` | 16 | `{type, hash, files[]}` | dropped silently (recognized, no entry) |
| `compaction` | 15 | — | dropped silently (recognized, no entry) |
| unknown | 0 | — | skipped, **counted** |

**Skipped-record accounting (controller ruling on spec §8):** the counter
reserves itself for corrupt/UNKNOWN shapes — malformed `part.data` JSON and
part types outside the recognized set above. Recognized-but-unmapped types
(`file`, `patch`, `step-start`, `step-finish`, `compaction`) are dropped
SILENTLY, so a full scan of healthy data reports zero skipped records.
Recognized types: `text`, `reasoning`, `tool`, `file`, `patch`,
`step-start`, `step-finish`, `compaction`.

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

## Token usage (`pastlog stats`, M7)

OpenCode is the one agent whose storage carries the usage aggregates
directly: the `session` table has `tokens_input`, `tokens_output`,
`tokens_reasoning`, `tokens_cache_read`, `tokens_cache_write` (integers,
NOT NULL DEFAULT 0), `cost` (real, NOT NULL DEFAULT 0) and `model` (text,
nullable). `SessionsUsage` maps the columns 1:1:

| Usage field | Source |
|---|---|
| `Input` | `session.tokens_input` |
| `Output` | `session.tokens_output` |
| `Reasoning` | `session.tokens_reasoning` |
| `CacheWrite` | `session.tokens_cache_write` |
| `CacheRead` | `session.tokens_cache_read` |
| `Model` | `session.model`, extracted: the column may carry a plain model id (`"gpt-5.3-codex"`) or a model-OBJECT JSON string (`{"id":"…","providerID":"…","variant":"…"}` — observed on real databases); when the value parses as a JSON object with a non-empty string `id`, that id is used, everything else (plain strings, objects without a usable id, malformed JSON) passes through verbatim |
| `CostUSD` / `HasCost` | `session.cost` with `HasCost=true` — the column is NOT NULL, so every opencode session provides a cost (even 0); opencode is the only agent with cost data (M7 ruling 5) |

- The usage columns ride the SAME walk as `SessionsMeta` (`walkUsage`): one
  connection, one `SELECT … ORDER BY time_created, id` cursor over `session`
  (extended by the token/cost/model columns), plus the existing grouped
  aggregate cursors for sizes and message counts.
- `Messages` keeps the `SessionsMeta` semantic, so sessions and messages of
  zero-usage sessions still count in the aggregates.
- The stored per-message `tokens`/`cost` payloads inside `message.data` are
  NOT read — the session-level columns are the agent's own aggregate and are
  the authoritative source (M7 ruling 3: stored totals like claude/codex
  `total` fields stay unused for the total column; pastlog computes
  total = input+output+reasoning itself).
- Locked-DB handling is identical to `SessionsMeta`: the usage view yields
  zero rows, no error, one warning.

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

### Read-only scope

`mode=ro` makes writes to `opencode.db` and `opencode.db-wal` impossible
(verified against the real database: sizes and mtimes unchanged after full
agents/sessions/search/show runs). Reading a WAL database requires SQLite to
coordinate through the `-shm` file; read-only readers update their read-mark
slots in it — standard SQLite behavior for every read-only client, affecting
transient coordination state only, never session data.

## Defensive behavior

- Malformed `part.data` JSON → skipped, counted (`SkippedLines()`; the CLI
  summarizes as "N unreadable lines skipped" — kept from M2 for all
  adapters). A full scan of the healthy real database counts **zero**
  records.
- Unknown part types (outside the recognized list above) → skipped, counted.
- Messages without any `part` rows (e.g. aborted turns — visible as NULL
  part columns in the LEFT JOIN cursor) are a recognized structure: nothing
  to record, **not counted**.
- Recognized-but-unmapped part types (`file`, `patch`, `step-start`,
  `step-finish`, `compaction`) → dropped **silently** (controller ruling on
  spec §8).
- Unreadable rows (unexpected NULLs) are skipped silently. An unparseable
  `message.data` counts as skipped (spec §8 letter, M4 ruling) — its parts
  still stream so no content is dropped; the role just degrades to "".
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
