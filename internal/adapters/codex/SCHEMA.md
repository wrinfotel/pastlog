# codex storage schema (assumed — no real data on this machine)

The Codex CLI keeps one JSONL "rollout" file per session. pastlog reads only;
nothing here is written back. This document describes the schema the adapter is
built against. It was compiled from publicly documented community observations,
not from live data — **no real Codex data existed on the build machine** (all
fixtures under `testdata/` are synthetic and anonymized). Whenever the real
format diverges, the adapter is defensive: unknown shapes are skipped and
counted, never fatal, never a panic.

## Location

```
~/.codex/sessions/YYYY/MM/DD/rollout-YYYY-MM-DDThh-mm-ss-<uuid>.jsonl
```

- One file per session, nested under the date directories the file was created.
- Only `rollout-*.jsonl` files exactly three levels below `sessions/` are
  considered; anything else is ignored.
- Detection = `~/.codex/sessions` exists and is a directory.

## Record shapes

Each line is a JSON object with a top-level `timestamp` (ISO 8601) and `type`.
Fields the adapter uses:

| Field | Meaning |
|---|---|
| `type` | record type: `session_meta` or `response_item`; anything else (`event_msg`, `turn_context`, `compacted`, …) is skipped **and counted** |
| `timestamp` | ISO 8601, e.g. `2026-08-02T14:03:22.150Z` → session start/end bounds |
| `payload` | type-specific object, see below |

### `session_meta` payload

One per file, typically the first line. Fields: `id` (session id),
`cwd` (project working dir), `cli_version`, `instructions` (optional; not used
by pastlog). The payload is the only source for `Session.ID` and
`Session.Project`.

### `response_item` payloads

| payload type | pastlog entry |
|---|---|
| `{"type":"message","role":"user"\|"assistant",...,"content":[...]}` | one `Message` per content item `{"type":"input_text"\|"output_text","text":...}`; role = payload role |
| `{"type":"function_call","name":...,"arguments":"..."}` | `ToolCall`, role `assistant`, text = raw `arguments` JSON (falls back to `name` when arguments are empty) |
| `{"type":"function_call_output","call_id":...,"output":...}` | `ToolResult`, role `tool`, text = `output` (a JSON string unquotes; any other JSON value is kept raw) |
| `{"type":"reasoning","summary":[{"type":"summary_text","text":...}]}` | one `Summary` with the item texts joined by newlines (best effort) |
| anything else | skipped **and counted** |

A message `content` that is a plain JSON string (defensive tolerance) is
treated as one text part. Unknown item types inside a readable `content` /
`summary` array are ignored silently; a *malformed* item (invalid JSON) marks
the whole line skipped.

## Mapping to pastlog's model

- `Session.ID` = first `session_meta.payload.id`; fallback: rollout filename
  minus `.jsonl`. The two may differ; `Entries` resolves the file through a
  lazily built id→path index covering both spellings (built during listing,
  or on demand in one storage pass — per-session tree walks were quadratic
  and missed the spec §7 budget).
- `Session.Project` = first `session_meta.payload.cwd`.
- `Session.Title` = first user message text, best effort: whitespace runs
  collapsed to single spaces, truncated to 80 runes with an ellipsis.
- `Session.StartedAt` / `EndedAt` = first / last `timestamp` on records the
  adapter actually classified (skipped records contribute nothing).
- `Session.SizeBytes` = JSONL file size.
- `Messages` (message count, spec §9) = number of `Message`-kind entries
  (user/assistant text parts). `reasoning` → Summary, function calls and
  unknown records are excluded.
- A file yields a session if it contains at least one non-empty line.

## Defensive behavior

- Corrupt / truncated / non-object lines → skipped, counted
  (`SkippedLines()`); the CLI summarizes as "N unreadable lines skipped".
- Unknown top-level `type` (`event_msg`, `turn_context`, …) and unknown
  `response_item` payload types → skipped and counted.
- Known types with a missing/null/unparsable payload → skipped and counted.
- Unknown item types inside a recognized content/summary array are ignored
  silently (the line itself was readable).
- Lines longer than 16 MiB exceed the scanner buffer: the rest of that file
  is skipped and counted (1 per oversized file). Real-world lines are well
  under 1 MiB; the enlarged buffer (64 KiB initial, 16 MiB max) handles all
  plausible lines >64 KiB per spec §5.
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
