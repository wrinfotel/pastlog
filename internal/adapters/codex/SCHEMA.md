# codex storage schema (observed + community notes)

The Codex CLI keeps one JSONL "rollout" file per session. pastlog reads only;
nothing here is written back. This document describes the schema the adapter
is built against. It was originally compiled from publicly documented
community observations; since 2026-10 it is confirmed against **real
rollouts** written by the 0.5x-era CLI (fixtures under `testdata/` stay
synthetic and anonymized). Whenever the real format diverges, the adapter is
defensive: unknown shapes are skipped and counted, never fatal, never a
panic.

Two format generations are supported side by side — a rollout file can even
mix them (a session resumed across a CLI upgrade appends new-generation
lines to an old file):

- **pre-0.5 (legacy)**: usage rides top-level `token_count` records; tool
  calls are `function_call` / `function_call_output`.
- **0.5x (current)**: usage rides `event_msg` envelopes
  (`payload.type:"token_count"`) and top-level `token_usage_record` lines;
  tool calls are `custom_tool_call` / `custom_tool_call_output` (with
  `function_call*` still appearing); inter-agent traffic, tool definitions
  and compaction payloads appear as extra `response_item` payload types;
  `world_state` and `inter_agent_communication_metadata` records interleave.

## Location

```
~/.codex/sessions/YYYY/MM/DD/rollout-YYYY-MM-DDThh-mm-ss-<uuid>.jsonl[.zst]
~/.codex/archived_sessions/rollout-YYYY-MM-DDThh-mm-ss-<uuid>.jsonl[.zst]
```

- Auto-discovery uses `CODEX_HOME` when set. An explicit CLI or Desktop
  home takes precedence.
- Active rollouts use date directories. Archived rollouts live directly
  under `archived_sessions/`. Both layouts accept plain JSONL and Zstandard.
  If the same rollout exists in both storage trees, the active copy wins.

## Record shapes

Each line is a JSON object with a top-level `timestamp` (ISO 8601) and `type`
(current rollouts add an `ordinal` between them; ignored). Fields the
adapter uses:

| Field | Meaning |
|---|---|
| `type` | record types include `session_meta`, `response_item` and the recognized-silent `token_count`, `event_msg`, `token_usage_record`, `turn_context`, `world_state`, `inter_agent_communication_metadata` and `compacted` |
| `timestamp` | ISO 8601, e.g. `2026-10-24T10:28:08.144Z` → session start/end bounds |
| `payload` | type-specific object, see below |

### `session_meta` payload

One per file, typically the first line. Fields: `id` (session id),
`cwd` (project working dir), `cli_version`, `instructions` (optional; not used
by pastlog). The payload is the only source for `Session.ID` and
`Session.Project`. (The search fast path still lists from this first line.)
For spawned subagents, `source.subagent.thread_spawn.parent_thread_id` supplies
the parent ID used by `related`.

### `response_item` payloads

| payload type | pastlog entry |
|---|---|
| `{"type":"message","role":"user"\|"assistant",...,"content":[...]}` | one `Message` per content item `{"type":"input_text"\|"output_text","text":...}`; role = payload role |
| `{"type":"function_call","name":...,"arguments":"<json>"}` | `ToolCall`, role `assistant`, text = raw `arguments` JSON (falls back to `input`, then `name` when empty) |
| `{"type":"custom_tool_call","name":...,"input":"<raw text>"}` (current) | `ToolCall`, role `assistant`, text = raw `input` string (a JS orchestration snippet in the observed rollouts; falls back to `name`) |
| `{"type":"function_call_output","call_id":...,"output":...}` | `ToolResult`, role `tool`, text = `output` (a JSON string unquotes; any other JSON value is kept raw) |
| `{"type":"custom_tool_call_output","call_id":...,"output":[{"type":"input_text","text":...},…]}` (current) | `ToolResult`, role `tool`, the output array's item texts joined with newlines (a plain string output works too) |
| `{"type":"reasoning","summary":[{"type":"summary_text","text":...}]}` | one `Summary` with the item texts joined by newlines (best effort) |
| `agent_message`, `additional_tools`, `compaction`, `configuration_update` | recognized silently. `compaction` marks a Compact event in the context flow. |
| anything else | skipped **and counted** |

A message `content` that is a plain JSON string (defensive tolerance) is
treated as one text part. Unknown item types inside a readable `content` /
`summary` array are ignored silently; a *malformed* item (invalid JSON) marks
the whole line skipped.

### usage records (recognized-silent, M7 + the 0.2.6 format fix)

Three record shapes carry token usage; the info/usage objects share one
field set (`input_tokens`, `cached_input_tokens`, `output_tokens`,
`reasoning_output_tokens` — all optional):

1. **top-level `token_count`** (legacy): `payload.info` with
   `total_token_usage` (cumulative) and `last_token_usage` (the turn's
   delta).
2. **`event_msg` with `payload.type:"token_count"`** (current): the same
   `payload.info` object. Every other event payload type (`item_completed`,
   `task_started`, `task_complete`, `thread_settings_applied`,
   `user_message`, `agent_message`, …) is recognized-silent — no entries,
   not counted.
3. **top-level `token_usage_record`** (current): `thread_token_usage` (a
   cumulative totals source, observed to track the token_count envelope's
   total within ~2%) and `turn_token_usage` (**not** a summable delta — it
   is per-response gross usage, counting the same window once per API call
   inside a turn; summing it overshoots the totals ~15× on real rollouts —
   so pastlog ignores it).

Semantics:

- Usage records and `compacted` are recognized silently. Their timestamps
  contribute to the session bounds.
- Each usage series is cumulative. Both the session total and the model
  breakdown use the series of the last usable totals snapshot. Changes
  are attributed to the preceding `turn_context` model. Repeated snapshots
  add no usage. A counter correction reduces the latest allocations.
- Missing fields retain their previous value within the same series.
  Rate-limit-only events with `info: null` leave token usage untouched.
- A payload that is missing/null/unparsable (or a wrong-typed `info`) stays
  in the skip+counted class, like every known type with an unusable payload.

### `turn_context` payload (recognized-silent, M7)

`{"cwd":…, "model":"gpt-…", "effort":…}` — **recognized-silent** since M7 (no
entries, not counted). The `model` field feeds `Usage.Model`: the LAST
non-empty `turn_context.model` in the rollout wins (still a top-level record
in current rollouts). `cwd` here does not override the session project (that
stays `session_meta`, first wins).

### `world_state` / `inter_agent_communication_metadata`

Pure metadata records interleaved by current rollouts; nothing is consumed
and nothing validated — recognized-silent (no entries, not counted).

## Mapping to pastlog's model

- `Session.ID` = first `session_meta.payload.id`; fallback: rollout filename
  minus `.jsonl` or `.jsonl.zst`. The two may differ; `Entries` resolves the file through a
  lazily built id→path index covering both spellings (built during listing,
  or on demand in one storage pass — per-session tree walks were quadratic
  and missed the spec §7 budget).
- `Session.Project` = first `session_meta.payload.cwd`.
- `Session.Title` = first user message text, best effort: whitespace runs
  collapsed to single spaces, truncated to 80 runes with an ellipsis.
- `Session.StartedAt` / `EndedAt` = first / last `timestamp` on records the
  adapter actually classified (skipped records contribute nothing;
  recognized-silent ones do).
- `Session.SizeBytes` is the on-disk size, including for compressed rollouts.
- `Messages` (message count, spec §9) = number of `Message`-kind entries
  (user/assistant text parts). `reasoning` → Summary, tool calls and
  unknown records are excluded.
- A file yields a session if it contains at least one non-empty line.

## Token usage (`pastlog stats`, M7)

`SessionsUsage` reads usage facts in the same streaming pass that yields the
sessions (`agentlog.UsageSource`; one walk, same sorted order, same
skip-accounting):

| Usage field | Source from the last usable cumulative snapshot |
|---|---|
| `Input` | `total_token_usage.input_tokens` / `thread_token_usage.input_tokens` |
| `Output` | `…​.output_tokens`, which already includes reasoning |
| `Reasoning` | `…​.reasoning_output_tokens` (0 when absent) |
| `CacheWrite` | — codex reports no separate cache-write total; stays 0 |
| `CacheRead` | `…​.cached_input_tokens` |
| `Model` | LAST non-empty `turn_context.payload.model`; "" when the rollout has none |
| `CostUSD` / `HasCost` | — rollouts carry no per-session cost; `HasCost` stays false |
| `Models` | per-model allocations from the same cumulative series as the session total |

- `Messages` keeps the `SessionsMeta` semantic, so sessions and messages of
  zero-usage rollouts still count in the aggregates.
- `SessionsMetaFast` (the search fast path) reads only line 1 and carries no
  usage — the stats flow always uses the full `SessionsUsage` pass.

## Defensive behavior

- Corrupt / truncated / non-object lines → skipped, counted
  (`SkippedLines()`); the CLI summarizes as "N unreadable lines skipped".
- Unknown top-level `type` and unknown `response_item`
  payload types → skipped and counted.
- `token_count`, `event_msg`, `token_usage_record`, `turn_context`,
  `world_state` and `inter_agent_communication_metadata` →
  recognized-silent (M7 controller ruling + the 0.2.6 format fix): no
  entries, NOT counted; unusable payloads of these types stay
  skip+counted, like every known type. (`world_state` /
  `inter_agent_communication_metadata` consume nothing, so their payloads
  are not validated at all.)
- Known types with a missing/null/unparsable payload → skipped and counted.
- Unknown item types inside a recognized content/summary array are ignored
  silently (the line itself was readable).
- Lines longer than 16 MiB exceed the scanner buffer: the rest of that file
  is skipped and counted (1 per oversized file). Real-world lines are well
  under 1 MiB; the enlarged buffer (64 KiB initial, 16 MiB max) handles all
  plausible lines >64 KiB per spec §5.
- Session listing walks the storage tree (`filepath.WalkDir`) instead of
  using `filepath.Glob`: Glob silently matches nothing when the home path
  contains glob metacharacters (`[`, `*`, `?`), while a walk is immune.
  Unreadable storage during listing is surfaced as a whole-adapter error
  (the CLI notes the agent as storage-unreadable), never silently truncated;
  absent storage simply lists zero sessions.
- Other read failures mid-file (I/O errors) make the file unreadable: the
  file is skipped silently and NOT counted — nothing was parsed wrong, the
  storage itself became unreadable (same class as an unopenable file).
- Unreadable files (permission errors) are skipped silently.
- Iteration callbacks may abort the stream by returning an error; the error
  is propagated unchanged.

## Streaming

`Sessions`/`Entries` never load a whole file: `bufio.Scanner` with
`Scanner.Buffer` (spec §5) walks line by line, parsing only the fields listed
above. `SessionsMeta` provides message counts in the same pass.
`EntriesFiltered` additionally accepts a raw-line predicate so the search
engine can skip JSON parsing entirely for lines that cannot contain the query
(spec §7 hot path); prefilter-rejected lines are not counted as skipped.

## Context analysis (`pastlog context`, SPEC-context-analysis)

The adapter implements `agentlog.CtxSource` (`ctxextract.go`): one streaming
pass over the rollout JSONL maps records onto the normalized `CtxEvent` IR.

- one TurnStart per usable token_count change — the legacy
  top-level spelling and the current `event_msg` envelope both map (a mixed
  rollout emits each turn once); `token_usage_record` lines emit nothing
  (their per-turn usage repeats the envelope's `last_token_usage` —
  double-sourcing would double every turn). `last_token_usage` is the
  per-request usage. Codex includes cached tokens in `input_tokens`,
  so the normalized input field excludes the portion reported as cache reads.
  Records missing `last_token_usage` fall back to the cumulative totals' diff (negatives
  clamp to 0 after compaction resets).
- a Compact event per compaction boundary — the legacy top-level `compacted`
  record and the current `response_item` `compaction` payload (encrypted)
  both map.
- `function_call`/`custom_tool_call` and `function_call_output`/
  `custom_tool_call_output` → ToolCall/ToolResult, correlated via `call_id`
  (FIFO fallback for id-less pairs). A `custom_tool_call`'s `input` is a raw
  string, not JSON — it passes through the args canonicalizer trimmed
  (ctxcanon.go), so repeated identical snippets still share an ArgsKey.
  Codex exposes no error flag, so R3 runs on the uniform error-shape regex
  (SPEC §2.2).
