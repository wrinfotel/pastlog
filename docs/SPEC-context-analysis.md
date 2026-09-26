# SPEC: `pastlog context` — why is the context growing?

Status: draft v1 (2026-09-22). Feature = per-session context analysis: how the
window filled, what fed it, and the same set of findings for every agent.

## 0. Guiding principle

**Analysis runs on one normalized event stream (IR), never on raw formats.**
Each adapter gains a thin extension that maps its native records to IR; every
rule reads only IR fields. Same code path for all five agents ⇒ by
construction the same findings, the same detail level, the same advice texts.
Agent differences appear ONLY as precision (exact vs estimated tokens), never
as a different rule set.

**No IR field → no rule silently disappears.** If precision is missing, the
rule still runs on byte-based estimates and the finding is flagged `~`
(estimated). The report ends with a one-line precision note.

## 1. Normalized event stream (IR)

A session = ordered sequence of `CtxEvent`s (order = log order; all formats
are time-ordered on disk). Derived from the existing per-adapter parse:

```
CtxEvent {
  Seq        int          // position in session
  At         time.Time    // zero allowed (see Zcode note)
  Kind       enum         // TurnStart | ToolCall | ToolResult | Message | Compact
  Role       string       // user | assistant | tool | system
  Tool       string       // tool name, "" when not a tool event
  ArgsKey    string       // normalized identity key of the call (§1.1)
  ResBytes   int          // raw byte length of tool output / message text
  Tokens     Tokens       // per-turn usage when the format provides it
  TokensKind enum         // Exact | Estimated(bytes/4)
}

Tokens { Input, Output, Reasoning, CacheRead, CacheWrite int64 }

Context proxy per turn:  C(t) = Input + CacheRead + CacheWrite
```

Rationale: every one of the five formats lets the adapter emit, per turn,
either exact usage or a byte proxy (see §3). `ResBytes` is always available —
formats differ in token precision, never in result sizes.

### 1.1 ArgsKey — identity of a call (for repeat detection)

`ArgsKey = tool + "\x00" + normalize(raw-args)`, where `normalize`:
- parse args as JSON if possible → drop known-noise keys
  (`session_id`, `call_id`, timestamps), sort remaining keys recursively
- else: trim + collapse whitespace
Two calls have the same ArgsKey iff they asked the same tool for the same
thing. Repeats = same ArgsKey occurring ≥2 times **with any assistant turn in
between** (back-to-back retries are the error-loop rule's business, §2.4).

Examples: `Read{path:main.go}` read twice = repeat. `Bash{cmd:"go test"}`
failed 3× in a row = error loop (different rule, no ArgsKey needed).

## 2. Rules (closed set, all five agents)

Thresholds are relative to the session's own scale, so small and huge
sessions behave identically. Let S = C(last turn) (final window estimate).

| # | Rule | Signal (IR) | Fires when | Finding text (one per rule) |
|---|------|-------------|-----------|------------------------------|
| R1 | Oversized tool results | ToolResult.ResBytes | any result > 10% of S_bytes OR top-3 results together > 40% of S_bytes | "top results by size: build.log 28k (~19%), screenshot 15k (~10%), …" |
| R2 | Repeated reads | ToolCall.ArgsKey | same ArgsKey ≥2× with a turn between; report count + bytes of ALL instances | "README.md read 3× (every re-read re-enters the window)" |
| R3 | Error loops | ≥2 consecutive ToolResults (or one Message between) that are error-shaped for the same tool | same error-shape ≥3 consecutive attempts | "`go test ./...` failed 4× in a row (≈9k tokens burned on retries)" |
| R4 | Growth & compactions | TurnStart.Tokens (or estimates) | (a) monotonic growth, no plateau → "grew all session"; (b) Compact events: drop % + turns-to-regrow | "compact at turn 37: −71%, regrew to 80% in 9 turns" |
| R5 | Anomalous single turn | TurnStart delta | one turn's delta > 30% of final S | "turn 14 added ~40k (35% of final window) — inspect what ran" |

### 2.1 Why exactly these five

Every one is computable from IR fields ALL agents provide (sizes, order,
error-shape, per-turn usage-or-estimate). Anything needing agent-exclusive
fields (Claude thinking blocks, Zcode model-io) is out — it would break the
"same rules everywhere" contract.

### 2.2 Error-shape detection (R3), uniform across agents

- Claude Code: `tool_result` with `is_error:true` → **adapter emits
  `ToolResult.Err=true`** (field documented in the Claude Code log format;
  NOT present in the repo's current fixtures — treat as may-be-absent and
  keep the regex fallback active)
- Codex: function_call_output whose output text matches
  `(?i)error|failed|exception|traceback` (first 2 KB) OR exit-code note
- Gemini CLI: `status:"error"` on toolCalls entries
- Opencode/Zcode: part `tool` with `state.status` != "completed"
- Fallback everywhere: regex on the first 2 KB of the result text
(Exact flags are Exact; regex-only is marked estimated.)

### 2.3 Byte size of the window, S_bytes

Proportional estimate, no hardcoding: share of the session's bytes occupied
by tool results/messages ≈ share of the window. For sessions with exact
tokens: `bytes_per_token ≈ C(last) / total bytes of turns`, applied backwards.
Findings show "~" whenever derived from bytes.

### 2.4 R3 vs R2 boundary

Consecutive same-key calls (no assistant turn between) = retry → R3.
Same-key calls with assistant turns between = genuine re-read → R2.
One rule per phenomenon, no double-count.

## 3. Adapter mapping (what each format already provides)

| Agent | Storage | Per-turn tokens | Compaction marker | Tool call/result | Result size | Error flag |
|---|---|---|---|---|---|---|
| Claude Code | JSONL `~/.claude/projects/<proj>/*.jsonl` | ✅ exact: `message.usage` (input/output/cache_creation/cache_read) on every assistant line | ⚠️ heuristic: user message matching `^\s*/compact` OR isCompact-summary | tool_use / tool_result blocks | ✅ raw len | ✅ `tool_result.is_error` |
| Codex | JSONL `~/.codex/sessions/**` | ✅ exact CUMULATIVE totals: `token_count.info.total_token_usage`; context curve = per-record total | ✅ `compacted` record type (currently "unknown → skipped"; promote to recognized) | function_call / function_call_output | ✅ raw len | ⚠️ regex on output (exact exit-code not observed; use conservative regex, mark `~`) |
| Gemini CLI | JSON `~/.gemini/tmp/<hash>/chats/*.jsonl` | ✅ exact per `gemini` record: `tokens{input,output,cached,thoughts}` | ✅ `$set` checkpoint records (already parsed) | `toolCalls[]` (+ in-content functionCall/Response) | ✅ raw len | ✅ `toolCalls[].status` != success (verify literal "error" vs codes on real data) |
| Opencode | SQLite `opencode.db` (read-only) | ✅ exact: `step-finish` part payload `tokens` per step — SCALAR number (verified in gen.go fixture) | ✅ `session.time_compacting` (epoch ms) + part `compaction` | part `tool`: `state.input` / `state.output` | ✅ len(output string) — observed up to ~300 KB | ✅ `state.status`; values seen: `completed`, `pending` (gen.go) — treat `failed`/`error` as error, verify full value set on a real DB |
| Zcode | SQLite `~/.zcode/cli/db/db.sqlite` | ✅ exact: `model_usage` rows per request (in/out/reasoning/cache_w/cache_r, `started_at` order) | ✅ part type `compaction` + session `time_compacting` | part `tool` state.input/output | ✅ len(output) | ✅ `state.status` |
| — | — | — | — | — | — | — |

Zcode timestamps: model_usage carries `started_at` (per SCHEMA), so turn
ordering is exact — no estimation needed. Verify on the real DB.

## 4. Output shape (`pastlog context <session-id>`)

```
Context profile: 3f9c81a2 (claude-code, ~118k tokens final, 41 turns)

context curve (per-turn window estimate):
  12k ▂▂▃▃▄▅▅▆▆▇▇█ … ▇ compact ▂▃▃▅▆▇▇█

top context feeders:
  ~19%  Bash      build.log            28k   (read twice, §R1+R2)
  ~10%  Read      screenshot.png       15k   (b64 inline image)
   7%   Read      README.md            3×5k  (re-read after edits)
   5%   Bash      go test ./...        4 fails × ~2k (retries)

compactions: 1 (turn 37, −71%, regrew to 80% in 9 turns)

advice:
  • redirect long outputs to a file, grep what you need (R1)
  • re-reads after edits: ask for diffs instead of full files (R2)
  • fix the failing test before retrying the suite (R3)
  • compact landed late: window was ~90% before it fired (R4)

precision: exact tokens (claude-code)
```

- one findings table, identical rule set for every agent; zero-token agents
  (hypothetically) degrade to `~estimated` — never to a shorter report
- `--json` variant mirrors stats' style (IR-native, for scripting)
- advice pool keyed by rule id, one advice per fired rule, sorted by impact
  (bytes/tokens share). Same advice text for the same rule across agents.

## 5. Aggregation levels (where findings attach)

The same rule pass over the IR stream can be rolled up at three depths; the
per-session report never becomes mandatory infrastructure for the listing.

**L1 — session (deep, on demand).** `pastlog context <session-id>`: full pass
over that session's log → curve, feeders, compactions, advice (see §4).
Nothing is cached or precomputed — latency stays with the explicit request.

**L2 — agent/project (aggregate).** `pastlog context --agent claude-code
--since 30d` (also `--project`, or bare `pastlog context` for everything):
run the same rules over every matching session, keep per-session counts of
fired rules, print the roll-up:

```
Context findings, last 30 days — claude-code, 20 sessions analyzed

  repeated reads (R2)      14 sessions   README.md, src/config.go most often
  error loops   (R3)        9 sessions   `go test ./...` dominant
  oversized results (R1)   7 sessions   build logs, inline screenshots
  compactions   (R4)       11 sessions   median regrow-to-80%: 8 turns
  anomalous turns (R5)      2 sessions

  median peak window: ~118k tokens (exact)
```

This is `stats` for context instead of tokens — and the natural demo material:
"your agent re-reads the same files in 70% of sessions" is a shareable
finding. Sorting = rule frequency, then affected sessions. Session ids stay
available (drill down with L1).

**L3 — the `sessions` listing: untouched.** A per-row context-health badge
would require a full pass of every listed file; that trades the listing's
latency for a column. Rejected for v1. Candidate for v2 only, and only with
a cache — a separate design conversation (it touches the no-index philosophy).

Implementation note: L2 is nearly free once L1 exists — same rules, fold
per-session results into counters instead of printing a report. The only
shared-state risk is memory on large sweeps: L2 streams sessions one at a
time and keeps counters only (the sessions-usage pass already proves this
pattern scales).

## 6. Implementation plan (small, incremental)

1. **IR + Claude Code first** (richest format; vertical slice proving the
   report): extend adapter to emit CtxEvent stream, implement R1/R2/R3/R4/R5
   against it, report + tests on fixture sessions (fixtures already in
   demo/fixture-home).
2. Codex: promote `compacted` record to recognized-silent → Compact event;
   cumulative→per-turn delta conversion (subtract previous total).
3. Gemini: tokens + `$set` already parsed; map status → Err.
4. Opencode: read `step-finish.tokens` (parts are already streamed); expose
   `time_compacting`.
5. Zcode: model_usage rows already summed in SessionsUsage — reuse the same
   cursor, emit per-request Tokens; part `compaction` → Compact.
6. Cross-agent parity test: golden session synthetically rendered in all five
   native formats (fixture generator already exists for opencode/zcode), assert
   **identical findings & advice** (only precision flag differs).

## 7. Non-goals (v1)

- No token-counting of arbitrary text (tiktoken etc.) — bytes/4 proxy only,
  flagged `~`
- No rewriting/compression advice beyond the fixed pool
- No cross-session *cause* analysis (v2 idea: "your sessions always blow up
  after screenshots") — L2 roll-ups (§5) count how often rules fire across
  sessions, but do not correlate findings with session features; that is a
  different, heavier feature
