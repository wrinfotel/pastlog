# ZCode — storage schema

Observed on real ZCode 0.16.5–0.16.9 data (Windows, 2026-09). ZCode is a
Node app with its own fixed home on every OS: `~/.zcode/`. The session store
is a single SQLite database with the observed layout:

```
~/.zcode/
  cli/
    db/
      db.sqlite          ← the session database (adapter reads this)
      db.sqlite-wal      ← side files (counted toward the `agents` footprint)
      db.sqlite-shm
    agents/<sess_id>/agent_<id>/metadata.json, output.txt, task.output
    rollout/model-io-<sess_id>.jsonl   (model I/O debug logs — not read)
```

pastlog opens `db.sqlite` strictly read-only (`?mode=ro` + a 200 ms
`busy_timeout`). A database locked by a running ZCode instance degrades the
adapter to unavailable: no sessions, no error, one stderr warning, other
agents unaffected (spec §4). Version drift is expected: adapters must be
defensive (skip unknown shapes, count skipped lines, never panic).

## Tables pastlog reads

### `session` — one row per session

| column | read as | notes |
|---|---|---|
| `id` | Session.ID | `sess_<uuid>`; subagent children are `sess_subagent_agent_<uuid>` |
| `directory` | Session.Project | working directory, verbatim |
| `title` | Session.Title | first input or generated, `title_source` distinguishes |
| `time_created` / `time_updated` | StartedAt / EndedAt | **epoch milliseconds** |
| `parent_id` | — (not read) | set on `subagent_child` sessions; children are listed, not filtered |
| `task_type` | — (not read) | observed: `interactive`, `subagent_child` |
| `project_id` | — (not read) | internal `proj_*` id; no `project` table exists in the database |

`workspace_id`, `slug`, `path`, `version`, `share_url`, `summary_*`,
`revert`, `permission`, `time_compacting`, `time_archived`, `title_*`,
`trace_id` are ignored.

### `message` — one row per message

`id`, `session_id`, `time_created`/`time_updated` (epoch ms), `sequence`,
and `data` — a JSON payload. Only `data.role` is read
(`user` / `assistant` observed; any other value passes through as the
entries' role). Everything else (`tokens`, `cost`, `modelId`, `path`,
`semantics`, `anchor`, …) is ignored — token facts come from `model_usage`
(below), not from message payloads.

### `part` — one row per message part

`id`, `message_id`, `session_id`, `time_created`/`time_updated` (epoch ms),
`sequence`, and `data` — a JSON payload with a `type` discriminator. Entry
mapping (identical to the opencode adapter):

| part `type` | entry(s) |
|---|---|
| `text` (`.text` non-empty) | Message (role from the message) |
| `reasoning` (`.text` non-empty) | Summary, role `assistant` |
| `tool` | ToolCall (compact JSON of `.state.input`, falling back to `.tool`) and, when `.state.output` is non-empty, ToolResult with the output text |
| `file`, `timeline`, `step-start`, `step-finish`, `compaction` | recognized on real data, mapped to no entry, **dropped silently** |
| anything else | unknown shape → counted as skipped (spec §8) |

Malformed `data` JSON also counts as skipped. Entries stream from one
ordered cursor: `message → part` joined on `message_id`, ordered by
`(message.time_created, message.id)` then `(part.time_created, part.id)`.

### `model_usage` — one row per model request (usage facts)

pastlog sums this table per session (`SessionsUsage`; also powers `stats`):

| column | read as |
|---|---|
| `session_id` | group key |
| `input_tokens`, `output_tokens`, `reasoning_tokens` | summed into Usage.Input/Output/Reasoning |
| `cache_creation_input_tokens`, `cache_read_input_tokens` | summed into Usage.CacheWrite / CacheRead |
| `model_id` | Usage.Model = the model of the **latest** request (rows stream in `started_at` order, last wins) |

Rows with failed/cancelled status keep their token counts (tokens spent are
tokens spent). ZCode exposes **no cost**: Usage.HasCost stays `false` and
`cost usd` never appears for zcode. Databases older than the `model_usage`
table degrade to zero usage instead of failing. All other columns
(`logical_request_id`, `attempt_index`, timings, retry/cancel counters,
error fields, `raw_usage_json`, …) are ignored.

Observed `model_id` values on real data are plain ids such as
`GLM-5.3-Flash` or `nex-agi/nex-n2.5-pro:free` (unlike opencode's
`session.model`, no JSON-object form has been observed, so no extraction is
performed).

## Sizes and message counts

Per-session `SizeBytes` = summed `LENGTH(CAST(data AS BLOB))` of the
session's `message` and `part` rows. `Messages` (SessionMeta /
SessionUsage) = count of `text` parts with non-empty text — mirroring what
`Entries` yields. The `agents` command reports the database footprint
(`db.sqlite` + `-wal` + `-shm`) via `TotalSizer`, not the per-session sums.

## Indexes observed (queries rely on them)

```
message(session_id, time_created, id)
part(session_id)
part(message_id, id)
model_usage(session_id)
```
