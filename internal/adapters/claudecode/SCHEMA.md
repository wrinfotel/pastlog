# claude-code storage schema (assumed — no real data on this machine)

Claude Code keeps one JSONL file per session. pastlog reads only; nothing here
is written back. This document describes the schema the adapter is built
against. It was compiled from publicly documented community observations, not
from live data — **no real Claude Code data existed on the build machine**
(all fixtures under `testdata/` are synthetic and anonymized). Whenever the
real format diverges, the adapter is defensive: unknown shapes are skipped and
counted, never fatal, never a panic.

## Location

```
~/.claude/projects/<escaped-cwd>/<session-uuid>.jsonl
```

- `<escaped-cwd>` is the session's working directory with every
  non-alphanumeric character replaced by `-` (e.g. `C:\Users\dev\myapp` →
  `C--Users-dev-myapp`, `/home/dev/myapp` → `-home-dev-myapp`).
- Directory names are ambiguous (several paths escape to the same name), so
  pastlog **always reads the project from the records' `cwd` field and never
  decodes the directory name**.
- Detection = `~/.claude/projects` exists and is a directory.
- Only `*.jsonl` files directly inside project directories are considered.

## Record shapes

Each line is a JSON object. Fields the adapter uses:

| Field | Meaning |
|---|---|
| `type` | record type: `user`, `assistant`, `system`, `summary`; anything else is skipped and counted |
| `message` | `{role, content}` on `user`/`assistant`/`system` records; `content` is a string **or** an array of typed blocks |
| `message.model` | the model that produced the record (assistant messages carry it) → `Usage.Model` |
| `message.usage` | `{input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens}` → token statistics (M7) |
| `timestamp` | ISO 8601 UTC, e.g. `2026-08-02T14:03:22.150Z` |
| `sessionId` | session UUID; fallback: JSONL filename minus `.jsonl` |
| `cwd` | working dir → `Session.Project` (first non-empty wins) |
| `summary` | on `type:"summary"` records → `Session.Title` (first wins) |
| `uuid`, `parentUuid` | record lineage — ignored by pastlog |

### content blocks

| block type | pastlog entry |
|---|---|
| `{"type":"text","text":...}` | `Message` (role = record role) |
| `{"type":"tool_use","name":...,"input":{...}}` | `ToolCall`, role `assistant`, text = tool name + raw input JSON |
| `{"type":"tool_result","content": string \| [blocks]}` | `ToolResult`, role `tool`, text = string content or concatenated block texts |
| anything else | ignored |

A bare JSON string inside a content array is treated as a `text` block.
Message blocks keep the record's role (`user`/`assistant`/`system`); tool
results are always role `tool`.

## Mapping to pastlog's model

- `Session.ID` = `sessionId` field (first seen), else filename minus extension.
- `Session.Project` = first non-empty record `cwd`.
- `Session.Title` = first `summary` record.
- `Session.StartedAt` / `EndedAt` = first / last `timestamp` on records the
  adapter actually classified (skipped records contribute nothing).
- `Session.SizeBytes` = JSONL file size.
- `Messages` (message count, spec §9) = number of `Message`-kind entries:
  `user`/`assistant`/`system` records whose content carries at least one text
  (string) part. `summary`/snapshot/unknown records are excluded; a
  tool-use-only assistant turn adds a `ToolCall` but no message.
- A file yields a session if it contains at least one non-empty line.

## Token usage (`pastlog stats`, M7)

`SessionsUsage` reads usage facts in the same streaming pass that yields the
sessions (`agentlog.UsageSource`; one walk, same sorted order, same
skip-accounting):

| Usage field | Source |
|---|---|
| `Input` | sum of `message.usage.input_tokens` over the session's records |
| `Output` | sum of `message.usage.output_tokens` |
| `Reasoning` | — claude-code records carry no separate reasoning count; stays 0 |
| `CacheWrite` | sum of `message.usage.cache_creation_input_tokens` |
| `CacheRead` | sum of `message.usage.cache_read_input_tokens` |
| `Model` | LAST non-empty `message.model` in the file (assistant records carry it); "" when none |
| `CostUSD` / `HasCost` | — claude-code logs carry no per-session cost; `HasCost` stays false |
| `Models` | per-model breakdown (TASK.md backlog): each record's usage lands on its `message.model`; a record without a model keeps the model in effect (the last non-empty one seen), so the entries keep first-use order and sum to the session totals |

- `Messages` keeps the `SessionsMeta` semantic, so sessions and messages of
  zero-usage sessions still count in the aggregates.
- Malformed usage never makes a line unreadable: a `message.usage` that is
  not a JSON object (missing, null, string, number) contributes zero, and a
  field of the wrong JSON type contributes zero for that field alone (the
  other fields of the same object still count).
- `SessionsMetaFast` (the search fast path) reads only line 1 and carries no
  usage — the stats flow always uses the full `SessionsUsage` pass.

### usable content

On `user`/`assistant`/`system` records, `message.content` must be a string or
an array of blocks. Any other JSON shape (number, boolean, object) means the
record has no usable message: the line is skipped and counted, like an
unknown record type. An empty string or an empty array is a usable (if
empty) message: the line is readable and not counted.

## Defensive behavior

- Corrupt / truncated / non-object lines → skipped, counted
  (`SkippedLines()`); the CLI summarizes as "N unreadable lines skipped".
- Records with unknown `type`, or known types without a usable `message`,
  are skipped and counted.
- Unknown block types inside a recognized record are ignored silently (the
  line itself was readable).
- A shape-mismatched element inside an otherwise readable content array
  (e.g. `{"type":42}` where a block object is expected) stops parsing that
  array: entries parsed before it are still emitted, and the line is
  additionally counted as skipped — hybrid semantics: the readable prefix is
  kept (never dropped), and the corrupt part is counted per spec §8.
- Lines longer than 16 MiB exceed the scanner buffer: the rest of that file
  is skipped and counted (1 per oversized file). Real-world lines are well
  under 1 MiB; the enlarged buffer (64 KiB initial, 16 MiB max) handles all
  plausible lines >64 KiB per spec §5.
- Other read failures mid-file (I/O errors) make the file unreadable: the
  file is skipped silently and NOT counted — nothing was parsed wrong, the
  storage itself became unreadable (same class as an unopenable file).
- Unreadable files (permission errors) are skipped silently.
- Listing failures are surfaced, never silently truncated (M4): the session
  file walk (`filepath.WalkDir` over `projects/`) returns an error when the
  storage cannot be read, and the CLI reports the agent as storage-unreadable
  (one note on stderr) while the other agents continue — listing does not
  proceed with partial results. Absent storage simply lists zero sessions.
  The walk is also immune to glob metacharacters in the home path (`[`, `*`,
  `?`), which would make a `filepath.Glob` pattern silently match nothing.
- The skipped counter accumulates across scans and is never reset; an adapter
  is not safe for concurrent scans (one Adapter per goroutine; see the
  Adapter type documentation).
- Iteration callbacks may abort the stream by returning an error; the error
  is propagated unchanged.

## Streaming

`Sessions`/`Entries` never load a whole file: `bufio.Scanner` with
`Scanner.Buffer` (spec §5) walks line by line, parsing only the fields listed
above. `SessionsMeta` provides message counts in the same pass; session
listing filters (`--agent/--project/--since/--until`) are applied by the
caller to each yielded session before accumulation, so unmatched sessions are
dropped early.
