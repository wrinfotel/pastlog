# SPEC: `pastlog optimize` — what burns tokens every session

The per-session analysis (SPEC-context-analysis.md) explains one session.
This spec defines its cross-session fold: the same normalized event stream
(agentlog `CtxSource`, context spec §1), accumulated across every matching
session to report waste that *repeats* — one oversized build log is R1's
finding; the same oversized build log paid in 29 of 41 sessions is a setup
problem, and no per-session report can say so.

The feature is advice-only by design: pastlog never writes to agent storage
or agent config, so unlike config-applying optimizers it can only point at
the pattern, quantify it, and stop.

## 0. Guiding principle

One rule pass, one accumulator set, no index. Sessions stream one at a time
(`agentlog.CollectCtxBatches`); the fold keeps counters only — the same
memory discipline as the sessions-usage pass (context spec §5's note).
Findings are estimates and say so: R6–R8 speak raw result bytes, R9 speaks
exact tokens, and nothing is ever attributed to a bill.

## 1. Identity of a call across sessions

The unit of recurrence is the call identity of context spec §1.1:
`ArgsKey = tool + "\x00" + canonical(rawInput)`. Two sessions calling the
same tool with byte-identical arguments share a group. This is deliberately
strict — commands whose arguments drift (`go test ./...` vs
`go test ./internal/...`) are different groups — under-reporting is
acceptable, false accusations are not.

## 2. Rules (closed set)

### 2.1 R6 — recurring heavy result

A call group with **≥ 3 occurrences across ≥ 2 sessions** whose **average
result size ≥ 2000 bytes** fires once, with the group totals: occurrences,
sessions, total result bytes. This is R1 generalized over time: the advice
(redirect the output to a file once) amortizes across every session that
would have paid it again.

### 2.2 R8 — recurring failure

A call group with **≥ 3 failed occurrences across ≥ 2 sessions** fires with
the failed-occurrence count and the bytes burned on failures only. This is
R3 generalized over time: within one session the loop is visible; across
sessions the loop *is the setup* — the failing command comes back until its
cause is fixed.

### 2.3 R7 — repeated re-discovery

A call group appearing in **≥ 3 distinct sessions** that did not fire as R6
or R8. The frame is re-discovery: every fresh session asks for the same
content from scratch because no durable artifact (agent memory, a summary
doc) captured it. Small result bytes are fine — the pattern, not the size,
is the finding.

### 2.4 Precedence

One finding per call group, checked in the order **R6 > R8 > R7**: a heavy
command's problem is its size, a failing command's problem is its failure,
and only clean, small, repeated calls are reported as re-discovery. A group
can therefore never appear twice in one report.

### 2.5 R9 — static context prefix

Independent of call groups: among sessions whose **first turn carries exact
usage** (context IR `CtxTokensExact`), if **≥ 3 sessions** qualify and the
**smallest first-turn window ≥ 16 000 tokens** (`Input + CacheRead +
CacheWrite`), R9 fires. That minimum is the always-riding prefix estimate —
system prompt, tool declarations, agent memory files — and its exposure is
`min × total turns of the qualifying sessions`, reported as tokens. Only
exact usage accuses: byte-derived first turns are too noisy for a number
that large.

### 2.6 Threshold rationale

- ≥ 2 sessions for R6/R8, ≥ 3 for R7/R9: the "cross" qualifier. Single-
  session repeats belong to R1/R2/R3 and must not double-report.
- 2000 bytes average: below that, "heavy" is noise at terminal width.
- 16k prefix: a lean setup stays under ~10k; 16k is where every request
  visibly pays.

## 3. Honesty rules

- Findings sort by impact (bytes for R6–R8, tokens for R9) and the report
  caps at the **top 10 patterns**; `truncated` counts the rest, and the
  human output says "(+N smaller patterns omitted)".
- Byte numbers are raw result bytes, never converted to tokens; R9's
  exposure is exact-token arithmetic and is the only `tokens` field.
- A session whose stream fails to load is skipped with the shared
  unreadable-storage note; the report keeps the rest (spec §8 best effort).
- Progress hooks tick per session (ruling R-D6); cancellation keeps the
  partial report and says so.
- Secret masking applies to labels and descriptions like every human
  surface; `--json` stays verbatim. The engine never embeds the label in
  `desc`, so a display-side truncation after masking can only ever print a
  substring of the masked text.

## 4. Output shape

CLI (`pastlog optimize [--agent] [--project] [--since] [--until] [--json]
[--no-mask]`), human:

```
optimize report: 3 sessions analyzed

findings:
  R9  static context ~17k tokens (smallest first turn across 3 sessions) rides in every request — ~249k tokens of prefix across 15 turns
  R6  go build -v ./… (Bash) returned ~19k per run ×3 across 3 sessions — every run pays it again

advice:
  • trim the always-loaded context (agent memory files, MCP servers) — every KB rides in every request (R9)
  • redirect this command's output to a file and read back only what you need — every run pays it again (R6)
```

The human line composes `label + desc` where the label is capped (80 runes)
by the renderer; `desc` never repeats it. Empty scope: "no cross-session
waste patterns — the history looks lean".

JSON (stable, additive — ruling R-D4): `{"sessions", "findings": [{"rule",
"tool"?, "label"?, "desc", "sessions", "count", "bytes", "tokens"?}],
"advice": [{"rule", "text"}], "truncated"?}`. The desktop binding
(`App.Optimize`) returns exactly this shape under `"report"`.

Desktop: an OPTIMIZE button on a project page (next to Timeline) opens the
report for that project's scope; advice above findings, impact right. No
sidebar entry — the report is a scope view, like the timeline.

## 5. Non-goals (v1)

- No config reading: installed-but-unused MCP servers and agent memory file
  sizes live in agent config files that also hold credentials — out of
  bounds while the guarantee is "sessions only, read-only".
- No fixes applied, no config edits, no hooks: advice text only.
- No savings in currency: no price table, by design (the no-network rule);
  impact is bytes and tokens.
- No fuzzy command clustering (same intent, different args): strict
  identity only; v2 may group by normalized command shape.
- No cross-session *cause* chains ("sessions blow up after screenshots") —
  that remains context spec §7's v2 idea, untouched here.
