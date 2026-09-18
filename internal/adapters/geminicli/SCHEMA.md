# gemini-cli storage schema (from the gemini-cli source, main branch 2026-09)

The Gemini CLI keeps project-scoped session files under `~/.gemini/tmp/`. No
real gemini-cli data existed on the build machine, so the schema was compiled
from the gemini-cli source and all fixtures under `testdata/` are synthetic
and anonymized. pastlog reads only; nothing here is written back. The format
is undocumented and version-dependent: the adapter is defensive — unknown
shapes are skipped and counted, never fatal, never a panic.

## Location

```
~/.gemini/tmp/<project-hash>/chats/session-<YYYY-MM-DDThh-mm>-<sessionId8>.jsonl   main sessions
~/.gemini/tmp/<project-hash>/chats/<parentSessionId>/<sessionId>.jsonl             subagent sessions
~/.gemini/tmp/<project-hash>/chats.json                                            LEGACY monolithic format
```

- `<project-hash>` is a hash of the project working directory; it is NOT
  decodable, so `Session.Project` comes from the records' `directories[]`
  (empty when absent).
- Main sessions live directly under `chats/`; subagent sessions one level
  deeper, in a directory named after the parent session id.
- Legacy versions (pre-JSONL migration) wrote one monolithic `chats.json` per
  project dir. Both formats are supported; other files are ignored.
- Detection = `~/.gemini/tmp` exists and is a directory.

## JSONL layout

Line 1 is a metadata record, then one MessageRecord per line:

```
{"sessionId":…,"projectHash":…,"startTime":"2026-08-02T14:03:20Z","lastUpdated":…,
 "kind":"main"|"subagent","directories":[…],"summary":…,"memoryScratchpad":…}
{"id":…,"timestamp":"2026-08-02T14:03:22Z","type":"user","content":…}
```

| Field | Meaning |
|---|---|
| `sessionId` | session id (subagent files may omit it — the filename is the fallback) |
| `startTime` / `lastUpdated` | session bounds; `Session.StartedAt` / `Session.EndedAt` |
| `directories` | project working directories; the first entry becomes `Session.Project` |
| `summary` | becomes `Session.Title` ("" when absent) |
| `kind` | `main` / `subagent` — not needed by the v0.1 model, ignored |

MessageRecord `type` ∈ `user | info | error | warning | gemini`. `gemini`
records additionally carry `toolCalls?: [{id, name, args, result?, status,
timestamp}]`, `thoughts?: [{subject, description, timestamp}]`, `tokens?`,
`model?`. `content` (and `displayContent`, which pastlog ignores) is a Gemini
`PartListUnion`: a string OR an array of parts — `{text}` → text,
`{functionCall:{name,args}}` → tool call, `{functionResponse:{name,response}}`
→ tool result, anything else → that part is skipped and the rest keeps
parsing.

Compaction protocol: checkpoint records `{"$set":{"messages":[…]}}` (a
snapshot of the conversation so far) and deletion records may appear mid-file.
Checkpoints are recognized protocol records: they are applied by mapping
their message array in place and are **never counted** as skipped; message
elements inside them that fail to parse are genuinely unknown shapes and are
counted. Records matching no known shape (no message `type`, no `$set`, no
metadata fields — e.g. deletion records, whose shape pastlog does not
recognize) are skipped **and counted**.

## Mapping to pastlog's model

- `Session.ID` = metadata `sessionId`; fallback: filename minus `.jsonl`.
  `Entries` resolves files by exact filename first, then by scanning metadata
  records, then inside legacy `chats.json`.
- `Session.Project` = first of `directories[]` ("" if none — the directory
  hash is not decodable).
- `Session.Title` = metadata `summary` ("" if absent).
- `Session.StartedAt` / `EndedAt` = metadata `startTime` / `lastUpdated`;
  record timestamps fill the bounds only when the metadata record has none.
- `Session.SizeBytes` = JSONL file size.
- Entries per MessageRecord, in record order (within a record: content texts,
  content part entries, tool calls/results, thoughts):
  - `type: user` → one `Message` per text part, role `user`
  - `type: info | error | warning` → `Message`, role `system` (best effort)
  - `type: gemini` → `Message`, role `assistant`, per text part; PLUS per
    `toolCalls` element: `ToolCall` role `tool`, text = compact JSON of
    `args` (falls back to the tool name when args are missing), and when a
    `result` is present a `ToolResult` role `tool` with the flattened result
    text; PLUS per `thoughts` element a `Summary` role `assistant`, text =
    `"subject: description"` (best effort, kept for search surface)
  - content-level `{functionCall}` / `{functionResponse}` parts map to
    `ToolCall` / `ToolResult` the same way
- `Messages` (message count, spec §9) = number of `Message`-kind entries
  (user/assistant/system — info/error/warning records count too, matching the
  `agentlog.SessionMeta.Messages` semantic used by the other adapters);
  tool calls and thoughts are excluded.

## Token usage (`pastlog stats`, M7)

`SessionsUsage` reads usage facts in the same streaming pass that yields the
sessions (`agentlog.UsageSource`; one walk over JSONL files then legacy
stores, same sorted order, same skip-accounting). Gemini records carry a
`tokens` summary and a `model` name (schema-confirmed fields of the gemini-cli
MessageRecord):

| Usage field | Source |
|---|---|
| `Input` | sum of `tokens.input` over the session's records |
| `Output` | sum of `tokens.output` |
| `Reasoning` | sum of `tokens.thoughts` |
| `CacheWrite` | — gemini-cli reports no cache-write count; stays 0 |
| `CacheRead` | sum of `tokens.cached` |
| `Model` | LAST non-empty record `model`; "" when none |
| `CostUSD` / `HasCost` | — gemini-cli logs carry no per-session cost; `HasCost` stays false |

- `tokens.tool` and `tokens.total` are recognized but **ignored**: `tool`
  counts tool-driving tokens, `total` double-counts the others, and mapping
  them would inflate the input/output columns. Documented best-effort.
- Per-record values are per-turn deltas (not cumulative, unlike codex), so
  usage ACCUMULATES across the session's records.
- Every field of `tokens` is optional; missing fields contribute 0. A record
  whose `tokens` value has the wrong JSON shape is a known type with an
  unusable field → the record is skipped and counted (gemini convention).
- Records replayed inside a compaction checkpoint (`{$set:…}`) contribute
  their entries only — their token facts stay with the original records, so
  checkpointed content is never double-counted.
- The legacy `chats.json` path runs the same record processing, so legacy
  sessions carry usage the same way.
- `Messages` keeps the `SessionsMeta` semantic, so sessions and messages of
  zero-usage sessions still count in the aggregates.
- `SessionsMetaFast` (the search fast path) reads only line 1 and carries no
  usage — the stats flow always uses the full `SessionsUsage` pass.

## Legacy chats.json

Top-level `{version, sessions: [...], currentSessionId}`; each `sessions[]`
element is the same ConversationRecord shape with the messages inline in a
`messages` array. Mapping is identical to the JSONL layout, with:

- `Session.SizeBytes` = length of the raw session element — an approximation,
  since the whole file backs every session.
- `Entries` re-streams the file and emits only the matching session's
  entries; the raw-line search prefilter is **ignored** for these sessions
  (a raw line is not a record boundary in a monolithic JSON file).
- Elements without a `sessionId` are skipped and counted.

## Defensive behavior

**Skipped-record accounting (controller ruling on spec §8):** the counter
reserves itself for corrupt/UNKNOWN shapes. Recognized-but-unmapped records —
the compaction checkpoints `{$set:…}` — are applied in place and never
counted, so healthy data reports zero skipped records. Everything that
matches no known shape increments the counter:

- Corrupt / truncated / non-object records → skipped, counted
  (`SkippedLines()`); the CLI summarizes as "N unreadable lines skipped".
- Unknown record shapes (no message `type`, no `$set`, e.g. deletion
  records, whose shape is unrecognized) → skipped and counted.
- Known types with unusable `content`/`toolCalls`/`thoughts` shapes → skipped
  and counted. Unknown part shapes inside a readable array are skipped
  silently (the record itself was readable). A record with absent or null
  content is readable and simply contributes no entries.
- Message elements inside a recognized container (checkpoint `messages`
  arrays, legacy `chats.json` session `messages` arrays) that fail to parse
  are unknown shapes → skipped and counted.
- Lines longer than 16 MiB exceed the scanner buffer: the rest of that file
  is skipped and counted (1 per oversized file). The enlarged buffer (64 KiB
  initial, 16 MiB max) handles all plausible lines >64 KiB per spec §5.
- Session listing (JSONL sessions, subagent files, and the legacy `chats.json`
  stores) walks the storage tree (`filepath.WalkDir`) instead of using
  `filepath.Glob`: Glob silently matches nothing when the home path contains
  glob metacharacters (`[`, `*`, `?`), while a walk is immune. Unreadable
  storage during listing is surfaced as a whole-adapter error (the CLI notes
  the agent as storage-unreadable), never silently truncated; absent storage
  simply lists zero sessions.
- Other read failures mid-file (I/O errors) make the file unreadable: the
  file is skipped silently and NOT counted — nothing was parsed wrong, the
  storage itself became unreadable (same class as an unopenable file).
- Unreadable files (permission errors) are skipped silently.
- Iteration callbacks may abort the stream by returning an error; the error
  is propagated unchanged.

## Streaming

`Sessions`/`Entries` never load a whole file: `bufio.Scanner` with
`Scanner.Buffer` walks JSONL line by line, and the legacy `chats.json` is
walked token-by-token with `json.Decoder`, decoding one session element at a
time. `SessionsMeta` provides message counts in the same pass.
`EntriesFiltered` accepts the search engine's raw-line predicate for JSONL
sessions (spec §7 hot path); it is ignored for legacy sessions (see above).
