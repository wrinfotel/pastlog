# pastlog

**Search the full history of your AI coding agents — 100% local, one binary.**

**[Project site & live demo →](https://wrinfotel.github.io/pastlog/)**

pastlog indexes nothing, uploads nothing, and configures nothing: it streams
the session logs that Claude Code, Codex CLI, Gemini CLI, OpenCode and ZCode
already
wrote under your home directory and makes them searchable across all your
projects — user/assistant messages, tool calls and tool outputs included.
`pastlog stats` aggregates token usage (and OpenCode's session cost) across
agents, projects, days and models. One static binary, zero servers, zero
accounts, zero telemetry, strictly read-only.

Supported agents: **Claude Code** · **Codex CLI** · **Gemini CLI** · **OpenCode** · **ZCode**

<p align="center"><img src="https://github.com/wrinfotel/pastlog/releases/download/v0.1.0/demo.gif" alt="pastlog demo: agents, sessions, search and show against fixture data" width="100%"></p>

## Contents

- [Install](#install)
- [Quickstart](#quickstart)
- [pastlog Desktop](#pastlog-desktop)
- [Exit codes](#exit-codes)
- [How it finds your data](#how-it-finds-your-data)
- [Performance](#performance)
- [How pastlog compares](#how-pastlog-compares)
- [FAQ](#faq)
- [Roadmap](#roadmap)
- [License](#license)

## Install

pastlog ships as a single static binary — no runtime, no dependencies, no
config files. Three ways to get it:

### 1. Download a release binary (recommended)

Grab the archive for your platform from the
[v0.1.0 release](https://github.com/wrinfotel/pastlog/releases/tag/v0.1.0) and
put `pastlog` on your `PATH`:

| Platform | Archive |
|---|---|
| Windows, Intel/AMD 64-bit | `pastlog_0.1.0_windows_amd64.zip` |
| Windows on ARM | `pastlog_0.1.0_windows_arm64.zip` |
| macOS, Apple Silicon | `pastlog_0.1.0_darwin_arm64.tar.gz` |
| macOS, Intel | `pastlog_0.1.0_darwin_amd64.tar.gz` |
| Linux, Intel/AMD 64-bit | `pastlog_0.1.0_linux_amd64.tar.gz` |
| Linux on ARM | `pastlog_0.1.0_linux_arm64.tar.gz` |

Every release ships a `checksums.txt` with SHA256 sums. Verify on
macOS/Linux before unpacking:

```sh
sha256sum -c checksums.txt --ignore-missing
```

Windows (PowerShell):

```powershell
Expand-Archive pastlog_0.1.0_windows_amd64.zip -DestinationPath .
Move-Item .\pastlog.exe "$env:USERPROFILE\go\bin\"   # or any folder on PATH
```

macOS/Linux:

```sh
tar xzf pastlog_0.1.0_darwin_arm64.tar.gz   # your platform's archive
sudo install pastlog /usr/local/bin/
pastlog version                             # sanity check
```

> **macOS Gatekeeper:** release binaries are not notarized (out of scope for
> v0.1), so macOS may refuse to run a downloaded binary with "cannot be
> opened because the developer cannot be verified". Either allow it under
> *System Settings → Privacy & Security*, or remove the quarantine flag:
>
> ```sh
> xattr -d com.apple.quarantine ./pastlog
> ```

### 2. Install with Go (any platform)

```sh
go install github.com/wrinfotel/pastlog/cmd/pastlog@latest
```

Requires Go 1.27 or newer. `go install` builds without version metadata, so
`pastlog version` reports `0.0.0-dev (commit none, date unknown)` — release
binaries report the tagged build.

### 3. Build from source

```sh
git clone https://github.com/wrinfotel/pastlog
cd pastlog
go build ./cmd/pastlog    # produces ./pastlog (.exe on Windows)
```

### Package managers

Homebrew and Scoop formulas are planned (see [Roadmap](#roadmap)); for now
use one of the three ways above.

## Quickstart

No arguments: short help plus a one-line summary of detected agents. On a
machine without any agent data you get `no agent data found — install an
agent or pass --home <dir>` and exit code 0.

```console
$ pastlog
pastlog searches the full history of your AI coding-agent sessions across all projects on this machine.
100% local, read-only, zero config.

Usage:
  pastlog [flags]
  pastlog [command]

Available Commands:
  agents      list detected agent sources with session counts
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  search      search all session entries across agents
  sessions    list sessions, newest first
  show        print one session as a readable transcript
  stats       aggregate token usage across agents, projects, days and models
  version     print version, commit and build date

Flags:
  -h, --help          help for pastlog
      --home string   user home directory holding the agent data (default: auto-detect)
      --no-color      disable colored output (also honors NO_COLOR and non-TTY)

Use "pastlog [command] --help" for more information about a command.
claude-code: 2 sessions, codex: 1 session, gemini-cli: 1 session, opencode: 2 sessions
```

`pastlog agents` — detected agent sources with session counts and on-disk
footprints (paths shown relative to your home directory):

```console
$ pastlog agents
claude-code  2 sessions  1.7 KB  ~/.claude/projects
codex        1 session   664 B  ~/.codex/sessions
gemini-cli   1 session   548 B  ~/.gemini/tmp
opencode     2 sessions  28.0 KB  ~/.local/share/opencode
zcode        0 sessions  (not found)
```

The same data as JSON (`--json` works on every command):

```console
$ pastlog agents --json
[
  {
    "name": "claude-code",
    "detected": true,
    "path": "/home/dev/.claude/projects",
    "sessions": 2,
    "bytes": 1778
  },
  {
    "name": "codex",
    "detected": true,
    "path": "/home/dev/.codex/sessions",
    "sessions": 1,
    "bytes": 664
  },
  {
    "name": "gemini-cli",
    "detected": true,
    "path": "/home/dev/.gemini/tmp",
    "sessions": 1,
    "bytes": 548
  },
  {
    "name": "opencode",
    "detected": true,
    "path": "/home/dev/.local/share/opencode",
    "sessions": 2,
    "bytes": 28672
  },
  {
    "name": "zcode",
    "detected": false,
    "path": null,
    "sessions": 0,
    "bytes": 0
  }
]
```

`pastlog sessions` — every session, newest first: agent, project, local
date, message count, size, ID prefix. Filter with `--agent <name>`,
`--project <substring>`, `--since <2w|7d|2026-01-01>`, `--until`, and cap
with `--limit N`:

```console
$ pastlog sessions --limit 5
codex        ~/dev/myapp    2026-08-02 17:10  2 messages   664 B  9b2d4c1e
claude-code  ~/dev/myapp    2026-08-02 17:03  2 messages  1.1 KB  3f9c81a2
gemini-cli   ~/dev/myapp    2026-08-02 17:03  2 messages   548 B  e4a7f2b3
claude-code  ~/dev/website  2026-08-01 21:22  2 messages   645 B  aaaa1111
opencode     ~/dev/api      2026-07-29 19:48  2 messages   251 B  7c1e9a0f
```

```console
$ pastlog sessions --project myapp --since 60d
codex        ~/dev/myapp  2026-08-02 17:10  2 messages   664 B  9b2d4c1e
claude-code  ~/dev/myapp  2026-08-02 17:03  2 messages  1.1 KB  3f9c81a2
gemini-cli   ~/dev/myapp  2026-08-02 17:03  2 messages   548 B  e4a7f2b3
opencode     ~/dev/myapp  2026-07-28 19:35  1 message     98 B  2f8d6b3a
```

`pastlog search <query>` — case-insensitive literal search across user and
assistant messages, tool-call inputs and tool outputs of all agents, with
the match highlighted and one line of context. `--regex` interprets the
query as a regular expression, `--case-sensitive` disables the default
folding, `--limit N` caps sessions scanned and `--max-hits N` caps total
hits printed (200 by default):

```console
$ pastlog search "jwt refresh"
codex · dev/myapp · 2026-08-02 · sess 9b2d4c1e
  → the jwt refresh endpoint returns 401 after an hour

claude-code · dev/myapp · 2026-08-02 · sess 3f9c81a2
  → Fix jwt refresh token rotation
  → the jwt refresh kept failing because the old token was still accepted — moving it to an httpOnly cookie and rotating on every use.

gemini-cli · dev/myapp · 2026-08-02 · sess e4a7f2b3
  → can you add a jwt refresh regression test?
```

`--json` emits the stable machine-readable schema (match offsets are rune
offsets into `line`); the example below truncates to one session with
`--limit 1`:

```console
$ pastlog search "jwt refresh" --json --limit 1
[
  {
    "session": {
      "id": "9b2d4c1e-2222-4222-8222-222222222222",
      "agent": "codex",
      "project": "/home/dev/myapp",
      "title": "the jwt refresh endpoint returns 401 after an hour",
      "started_at": "2026-08-02T14:10:00Z",
      "ended_at": "2026-08-02T14:10:12Z",
      "messages": 2,
      "size_bytes": 664
    },
    "hits": [
      {
        "kind": "message",
        "role": "user",
        "timestamp": "2026-08-02T14:10:05Z",
        "context": "",
        "line": "the jwt refresh endpoint returns 401 after an hour",
        "match_start": 4,
        "match_end": 15
      }
    ]
  }
]
```

`pastlog show <session-id-or-prefix>` — one session as a readable
transcript. Accepts an unambiguous ID prefix; on ambiguity it lists the
candidates and exits 2:

```console
$ pastlog show 3f9c81a2
# Fix jwt refresh token rotation
claude-code · 3f9c81a2-1111-4222-8333-cccccccccccc · ~/dev/myapp · 2026-08-02 17:03:22 → 2026-08-02 17:03:26 · 2 messages · 1.1 KB

       -  summary      Fix jwt refresh token rotation
17:03:22  user         the refresh token is stored in localStorage, is that safe?
17:03:25  assistant    the jwt refresh kept failing because the old token was still accepted — moving it to an httpOnly cookie and rotating on every use.
17:03:26  tool result  tests pass: 12 ok, 0 failed
```

`--export md` prints the session as markdown for redirecting into a file;
`--json` prints the stable JSON schema:

```console
$ pastlog show 3f9c81a2 --export md > jwt-session.md
$ cat jwt-session.md
# Fix jwt refresh token rotation

- **agent:** claude-code
- **session:** 3f9c81a2-1111-4222-8333-cccccccccccc
- **project:** /home/dev/myapp
- **started:** 2026-08-02 17:03:22
- **ended:** 2026-08-02 17:03:26
- **messages:** 2
- **size:** 1.1 KB

## summary

Fix jwt refresh token rotation

## user · 2026-08-02 17:03:22

the refresh token is stored in localStorage, is that safe?

## assistant · 2026-08-02 17:03:25

the jwt refresh kept failing because the old token was still accepted — moving it to an httpOnly cookie and rotating on every use.

## tool result · 2026-08-02 17:03:26

tests pass: 12 ok, 0 failed
```

`--json` prints the same session as the stable machine-readable schema
(`entries[].kind` is one of `summary`, `message`, `tool_call`, `tool_result`):

```console
$ pastlog show 3f9c81a2 --json
{
  "id": "3f9c81a2-1111-4222-8333-cccccccccccc",
  "agent": "claude-code",
  "project": "/home/dev/myapp",
  "title": "Fix jwt refresh token rotation",
  "started_at": "2026-08-02T14:03:22.15Z",
  "ended_at": "2026-08-02T14:03:26.24Z",
  "messages": 2,
  "size_bytes": 1001,
  "entries": [
    {
      "kind": "summary",
      "role": "",
      "text": "Fix jwt refresh token rotation",
      "timestamp": null
    },
    {
      "kind": "message",
      "role": "user",
      "text": "the refresh token is stored in localStorage, is that safe?",
      "timestamp": "2026-08-02T14:03:22.15Z"
    },
    {
      "kind": "message",
      "role": "assistant",
      "text": "the jwt refresh kept failing because the old token was still accepted — moving it to an httpOnly cookie and rotating on every use.",
      "timestamp": "2026-08-02T14:03:25.001Z"
    },
    {
      "kind": "tool_result",
      "role": "tool",
      "text": "tests pass: 12 ok, 0 failed",
      "timestamp": "2026-08-02T14:03:26.24Z"
    }
  ]
}
```

`pastlog stats` — where your tokens go: token usage aggregated across all
five agents, grouped with `--by agent|project|day|model` (default `agent`)
and filterable with the same `--agent`/`--project`/`--since`/`--until`
filters as `sessions`, plus `--model <substring>` (case-insensitive match on
the session's model). Groups print in ascending key order (chronological for
`--by day`); `total` is input+output+reasoning; the `cost usd` column appears
only when the selection includes sessions that carry a cost (OpenCode records
one per session — the other agents' logs don't, shown as `-`):

```console
$ pastlog stats
agent        sessions  messages  input  output  reasoning  cache read  cache write  total  cost usd
claude-code         3         6    120      45          0         200           30    165         -
codex               2         3    300      90         25          80            0    415         -
gemini-cli          2         4    200      60         10          15            0    270         -
opencode            2         3  1,833     507        107      10,240          640  2,447      0.50
```

Group by model (sessions whose logs record no model land in the `-` group)
or by day; filter first if you only want one slice:

```console
$ pastlog stats --by model --model sonnet
model              sessions  messages  input  output  reasoning  cache read  cache write  total
claude-sonnet-4-5         1         2    120      45          0         200           30    165

$ pastlog stats --by day --since 60d
day         sessions  messages  input  output  reasoning  cache read  cache write  total  cost usd
2026-08-01         1         2      0       0          0           0            0      0         -
2026-08-02         6        11    620     195         35         295           30    850         -
2026-08-08         2         3  1,833     507        107      10,240          640  2,447      0.50
```

`--json` emits the stable machine-readable schema; `cost_usd` is `null` when
no session in the group provided a cost and a number (even `0`) when any did.
An empty selection is not an error: the human output prints nothing and the
JSON prints `[]`, both with exit code 0:

```console
$ pastlog stats --agent opencode --json
[
  {
    "key": "opencode",
    "sessions": 2,
    "messages": 3,
    "tokens": {
      "input": 1833,
      "output": 507,
      "reasoning": 107,
      "cache_read": 10240,
      "cache_write": 640,
      "total": 2447
    },
    "cost_usd": 0.5
  }
]
```

| Flag | Meaning |
|---|---|
| `--by agent\|project\|day\|model` | grouping dimension (default `agent`; project keys render tilde-shortened like `sessions`) |
| `--agent <name>` | only one agent's sessions (unknown names exit 2) |
| `--project <substring>` | only sessions whose working dir contains it (case-insensitive) |
| `--model <substring>` | only sessions whose model name contains it (case-insensitive) |
| `--since <Nd\|Nw\|YYYY-MM-DD>` | only sessions started after that point |
| `--until <Nd\|Nw\|YYYY-MM-DD>` | only sessions started before that point |
| `--json` | stable JSON schema instead of the table |

(The examples above were captured from the real binary on a synthetic
fixture home whose sessions carry usage fields; where your own agent logs
record no usage, sessions still count — their token columns stay 0.)

`pastlog version` — semantic version, commit and build date, injected at
link time. A release binary reports the tagged build:

```console
$ pastlog version
pastlog v0.1.0 (commit 15f543aef6a3d48aee44f42077459bf364083b4c, date 2026-09-13T10:00:00Z)
```

(a plain `go install` build without ldflags reports `pastlog 0.0.0-dev
(commit none, date unknown)`.)

Colors: matches and agent names are highlighted on a TTY; `--no-color`,
`NO_COLOR`, and non-TTY output (pipes, redirects) disable color. Non-standard
setups: `--home <dir>` points pastlog at any home directory.

## pastlog Desktop

**Prefer a window to a terminal?** pastlog Desktop is the GUI companion to
the CLI: the same engine, the same guarantees, the same view of your data —
browse and search the full history of all five agents across all projects,
click an agent on Home to drill into its projects and see which models each
one used and at what token cost, read transcripts comfortably (collapsible
tool calls, markdown-rendered assistant messages), inspect token-usage
statistics, and export anything to JSON or markdown.

| CLI | Desktop |
|---|---|
| `pastlog` (summary) | Home (agent cards drill into their projects) |
| `pastlog agents` | Diagnostics |
| `pastlog sessions` | Sessions (virtualized, all filters) |
| `pastlog search` | Search (live, progress + cancel, click a hit to open the session) |
| `pastlog show` | Session viewer (with `--export md`/`--json` parity) |
| `pastlog stats` | Stats · Projects (per-agent project list → per-model usage of one project + its sessions) |
| `pastlog version` | About (in Settings) |

Every GUI JSON export is byte-identical to the CLI's `--json` output —
proven by tests that run both against the same data.

### Desktop install

Grab a `desktop-v*` release from the
[releases page](https://github.com/wrinfotel/pastlog/releases) — the first one
is [desktop-v0.1.0](https://github.com/wrinfotel/pastlog/releases/tag/desktop-v0.1.0),
~15–20 MB per platform:

| Platform | Artifact |
|---|---|
| Windows, Intel/AMD 64-bit | `pastlog-desktop-amd64-installer.exe`, or `pastlog-desktop-amd64.exe` as a portable single file |
| Windows on ARM | `pastlog-desktop-arm64.exe` |
| macOS, Apple Silicon | `pastlog-desktop-<version>-arm64.dmg` |
| macOS, Intel | `pastlog-desktop-<version>-amd64.dmg` |
| Linux, amd64 / arm64 | `pastlog-desktop_<version>_amd64.deb` / `pastlog-desktop_<version>_arm64.deb` |

Every desktop release ships a per-platform `checksums-<platform>.txt` with
SHA256 sums of its artifacts.

- **Windows:** unsigned in v0.1 (SmartScreen may warn — same honesty as the
  CLI). WebView2 is preinstalled on Windows 11 and virtually all Windows 10
  devices; the installer embeds Microsoft's silent Evergreen bootstrapper for
  the rare builds without it. Nothing else is installed.
- **macOS:** unsigned — remove the quarantine flag with
  `xattr -d com.apple.quarantine ./pastlog\ Desktop.app` after mounting the
  dmg.
- **Linux:** the `.deb` declares `libgtk-3-0` and `libwebkit2gtk-4.1-0`; your
  package manager resolves them automatically.

### What the desktop app does NOT do

No telemetry, no auto-update checks, no network calls in its own operation
(the frontend is embedded in the binary and loads zero external assets; a
strict CSP is enforced). It never writes to agent storage — its only writes
are your chosen export destination and its own settings file in the OS
app-config dir (theme + home override). Session content is rendered as text
or sanitized markdown only; nothing shown is ever executable, and links open
in your system browser, never inside the app.

<!-- TODO: desktop screenshots (Home / Search / Viewer / Stats) -->

## Exit codes

grep-style, on every command:

| Code | Meaning |
|---|---|
| `0` | ok — including "nothing found" (`agents`/`sessions` list nothing, `stats` prints nothing for an empty selection, `show` prints an empty session) |
| `1` | `search` found no matches (nothing is printed) — stats never exits 1: an empty aggregate is a valid result |
| `2` | real error — bad flag value (e.g. an invalid `--by`), unknown agent, unreadable `--home`, ambiguous session-ID prefix, unusable storage |

```console
$ pastlog search "kubernetes"
$ echo $?
1
```

## How it finds your data

Everything is read from under a single home directory: `--home <dir>` wins,
then `USERPROFILE`/`HOME`, then the OS default. pastlog never writes to any
of these locations — the guarantee is enforced by tests, including opening
the OpenCode database with SQLite's `mode=ro` (see the
[read-only FAQ](#does-it-modify-my-logs)).

| Agent | Storage location (home-relative) | Notes |
|---|---|---|
| claude-code | `~/.claude/projects/<escaped-cwd>/<session-uuid>.jsonl` | one JSONL file per session; the project comes from each record's `cwd` field (directory names are ambiguous and never decoded) |
| codex | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` | one rollout JSONL per session (`session_meta` + `response_item` records) |
| gemini-cli | `~/.gemini/tmp/<project-hash>/chats/session-*.jsonl` (+ `chats.json` for legacy stores) | `<project-hash>` is not decodable; the project comes from the records' `directories[]` |
| opencode | `~/.local/share/opencode/opencode.db`; on Windows `%LOCALAPPDATA%\opencode\opencode.db` first, falling back to `~/.local/share/opencode/opencode.db`; on macOS/Linux `$XDG_DATA_HOME/opencode/opencode.db` (when the variable is set) first, falling back to `~/.local/share/opencode/opencode.db` (first existing wins) | SQLite database, opened strictly read-only (`?mode=ro`); if it is locked by a running OpenCode instance, pastlog warns once and continues with the other agents |
| zcode | `~/.zcode/cli/db/db.sqlite` | SQLite database, opened strictly read-only (`?mode=ro`); if it is locked by a running ZCode instance, pastlog warns once and continues with the other agents |

Schema details per agent, including exactly what is parsed, skipped, and
counted: [`internal/adapters/claudecode/SCHEMA.md`](internal/adapters/claudecode/SCHEMA.md),
[`internal/adapters/codex/SCHEMA.md`](internal/adapters/codex/SCHEMA.md),
[`internal/adapters/geminicli/SCHEMA.md`](internal/adapters/geminicli/SCHEMA.md),
[`internal/adapters/opencode/SCHEMA.md`](internal/adapters/opencode/SCHEMA.md),
[`internal/adapters/zcode/SCHEMA.md`](internal/adapters/zcode/SCHEMA.md).

## Performance

No index in v0.1 — pure streaming over whatever the agents wrote. Measured
end to end (listing + scan) on a synthetic 500 MB corpus with the benchmark
suite from [`internal/search/bench_test.go`](internal/search/bench_test.go):

| Benchmark | Corpus | Result |
|---|---|---|
| literal search (the human `search` flow, fast listing) | 500 MB | 2.81–2.82 s ≈ **177.6–178.1 MB/s** |
| literal search (the `--json` path, full-parse listing) | 500 MB | 7.15 s ≈ 69.9 MB/s |
| regex search | 500 MB | 6.95–6.99 s ≈ 71.5–72.0 MB/s |
| raw prefilter scan incl. file reads | 500 MB | 1.27–1.36 s ≈ 368–395 MB/s |

Hardware disclaimer: these are dev-machine numbers (windows/amd64, Intel
i5-12400F, warm file-system cache) — expect variation with CPU, storage and
the shape of your agent data. Two caveats worth knowing before comparing:

1. Real-world corpora are mixed: files whose first record line does not
   classify (e.g. sessions opening with a summary line) fall back to a full
   parse, so real-world runs land **between** the full-parse and fast-path
   numbers above.
2. On Windows, per-file open overhead under antivirus interception dominates:
   expect **seconds** where Linux/macOS land near the spec's 300 ms/500 MB
   budget. The CPU-side pipeline alone is far faster than that: a one-off
   scratch harness measured the in-memory prefilter at ~4.6 GB/s (500 MB in
   ~0.11 s) — that figure is **not** reproducible via `go test -bench`, which
   reports the file-read-inclusive 368–395 MB/s `BenchmarkPrefilterRaw` row
   above.

### Desktop binding-layer numbers

The GUI adds one serialization hop between the Go core and the webview.
`BenchmarkMarshalSessions10k`
([`desktop/app/export_test.go`](desktop/app/export_test.go)) marshals the
10,000-session JSON payload the Sessions view receives: **≈ 5.8 ms** on the
same dev machine (windows/amd64) — negligible against the scan itself, and
the reason the virtualized lists stay smooth at 10k+ rows.

## How pastlog compares

As of 2026-09:

| Tool | Gap pastlog fills |
|---|---|
| `agentlogs/agentlogs` | Team analytics / collaboration focus (capture pipeline) — not a personal instant-search CLI |
| chatgrep.com | Closed-source, focuses on browser AI chats (ChatGPT web etc.), not CLI agent logs |
| `cc-sessions`, `claude-history` | Claude Code only; no Codex/Gemini/OpenCode; no export |
| `claude-code-history-viewer` | GUI desktop app, Claude Code + Gemini only |
| Built-in `/resume` | Current session picker only — no cross-project, cross-agent search |

pastlog's wedge: **universal (5 agents) + fast + 100% local + first-class
Windows support.**

## FAQ

### Is anything uploaded?

No. There is no network code in the binary at all — no HTTP client, no TLS,
no telemetry, no update checks.
[`TestNoNetworkDeps`](internal/agentlog/nonetwork_test.go) fails the build
if `net/http`, `crypto/tls` or any third-party HTTP package ever appears in
the module's dependency graph.

### Is Windows supported?

Yes, first-class — it is a development platform here, not an afterthought:
CI runs the race-enabled test suite on Windows, the release build produces
`windows/amd64` and `windows/arm64` binaries, and path handling is
normalized (`--project` matches forward and back slashes alike).

### Does it modify my logs?

No — read-only by design and enforced by tests. Adapters only open files
for reading, and the OpenCode adapter opens its database with SQLite's
`?mode=ro`; a write attempt is refused by the driver
([`TestNoWritesToDatabase`](internal/adapters/opencode/adapter_test.go)).

### A scan says "N unreadable lines skipped" — what does that mean?

Agent formats are undocumented and version-dependent, so every adapter
parses defensively: lines it cannot classify (truncated JSON, unknown record
shapes, unexpected field types) are skipped, counted, and summarized on
stderr as one lowercase line — the run itself never fails because one record
is unusable. The count is exact for `pastlog sessions`; `pastlog search`
uses a fast listing that reads only the first record line of each file, so
its count is a lower bound.

### Where do I report a broken schema?

Per-agent schema notes live next to the adapters —
[claude-code](internal/adapters/claudecode/SCHEMA.md),
[codex](internal/adapters/codex/SCHEMA.md),
[gemini-cli](internal/adapters/geminicli/SCHEMA.md),
[opencode](internal/adapters/opencode/SCHEMA.md) — including what is parsed,
skipped, and counted. If a format change makes pastlog miss or misread your
sessions, open an issue at
<https://github.com/wrinfotel/pastlog/issues> with the agent name and version
(please never attach real session data — a synthetic record that reproduces
the shape is enough).

## Roadmap

- **MCP server** (`pastlog mcp`) — let your coding agent search its own history
- **TUI** — interactive browsing on top of the same engine
- **Homebrew / Scoop packages** — `brew install` and `scoop install` formulas
- **Optional index** — for corpora where streaming is not enough

## License

[MIT](LICENSE) — © 2026 pastlog contributors
