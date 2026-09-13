# TASK: Build `pastlog` — publishable MVP v0.1.0

> **Who this is for:** a coding agent implementing this from scratch in this repository (currently empty).
> **Goal:** a correct, tested, installable CLI, ready to publish on GitHub — not a feature-complete product.
> **Method:** TDD. Every feature lands with tests. Verify before claiming done.

---

## 1. Product definition

**pastlog** is a local-first CLI that searches the full history of your AI coding-agent sessions
(Claude Code, Codex CLI, Gemini CLI, OpenCode) across all projects on the machine.

One static binary. Zero servers. Zero accounts. Zero telemetry. Strictly read-only.

Every CLI coding agent writes its complete session history (messages, tool calls, outputs) into
hidden folders under `$HOME`, in undocumented per-agent formats, with no cross-project search UI.
After a few months that is gigabytes of valuable knowledge: how a bug was fixed, which commands
were tried, which decisions were made and why. pastlog makes it instantly searchable.

**Positioning vs. existing tools (as of 2026-09):**

| Tool | Gap we exploit |
|---|---|
| `agentlogs/agentlogs` | Team analytics / collaboration focus (capture pipeline) — not a personal instant-search CLI |
| chatgrep.com | Closed-source, focuses on browser AI chats (ChatGPT web etc.), not CLI agent logs |
| `cc-sessions`, `claude-history` | Claude Code only; no Codex/Gemini/OpenCode; no export |
| `claude-code-history-viewer` | GUI desktop app, Claude Code + Gemini only |
| Built-in `/resume` | Current session picker only — no cross-project, cross-agent search |

Our wedge: **universal (4 agents) + fast + 100% local + first-class Windows support.**

## 2. Non-negotiable product principles

1. **100% local.** No network calls. At all. The binary must not depend on any HTTP/TLS code.
   Add a test (`TestNoNetworkDeps`) running `go list -deps ./...` and asserting none of
   `net/http`, `crypto/tls`, or any third-party HTTP client appears in the dependency graph.
2. **Read-only.** Never create, modify, move, or delete agent data. Tests must fail if the
   program writes anywhere except stdout/stderr/temp-dir.
3. **Zero-config.** Works immediately after install, no config file. Auto-discovers agent
   storage under the user home dir. Provide only a `--home <dir>` override (needed for tests
   and non-standard setups).
4. **Zero telemetry.** No update checks, no pings, no crash reporting.
5. **Single static binary**, cross-platform: windows/amd64+arm64, darwin/amd64+arm64, linux/amd64+arm64.
6. **Pipe-friendly.** Plain text on stdout; `--json` for machine-readable output everywhere;
   respect `NO_COLOR`, `--no-color`, and auto-disable color on non-TTY.

## 3. Commands (MVP scope)

Exit-code convention (grep-style, document in README): `0` ok (including "no matches"),
`1` no matches for `search`, `2` real error.

### `pastlog`
No arguments: short help + one-line summary of detected agents and session counts.

### `pastlog agents`
Detected agent sources: name, storage path, session count, total size.

```
$ pastlog agents
opencode     812 sessions   1.4 GB   ~/.local/share/opencode
claude-code    0 sessions   (not found)
codex          0 sessions   (not found)
```
`--json` variant with the same data.

### `pastlog sessions`
List sessions, newest first: agent, project (working dir), date, message count, size, ID prefix.

Flags: `--agent <name>`, `--project <substring>`, `--since <2w|7d|2026-01-01>`, `--until`,
`--limit N`, `--json`.

### `pastlog search <query>`
Search across all session entries (user/assistant messages, tool-call inputs and outputs)
of all agents. Default: case-insensitive literal match; `--case-sensitive` to disable,
`--regex` for regular expressions.

Output: result cards — session header (agent, project, date, ID) followed by matching lines
with the match highlighted and one line of context.

```
$ pastlog search "jwt refresh" --last 30d
codex · myapp/api · 2026-08-02 · sess 3f9c81a2
  The issue was that the refresh token was stored in localStorage
  → we moved it to an httpOnly cookie and rotated on every use
```

Flags: `--agent`, `--project`, `--since/--until`, `--limit N` (sessions), `--max-hits N`,
`--json`, `--no-color`.

### `pastlog show <session-id-or-prefix>`
Print one session as a readable transcript to stdout. Accepts an unambiguous ID prefix;
on ambiguity, list candidates and exit 2. Flags: `--export md` prints the session as
markdown (user redirects to a file), `--json`.

### `pastlog version`
Semantic version, commit hash, build date (injected via `-ldflags`).

## 4. Data sources and adapters

The core of the project. Each agent has its own storage format → one adapter per agent,
all conforming to a single interface. Formats are undocumented and version-dependent:
**adapters must be defensive** (skip unknown shapes, count skipped lines, never panic).

| Agent | Location (home-relative) | Format | Priority |
|---|---|---|---|
| claude-code | `~/.claude/projects/<escaped-cwd>/<session-uuid>.jsonl` | JSONL, lines with `type` (user/assistant/system), `message.content`, `timestamp`, `sessionId`, `cwd` | **P0** |
| codex | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` | JSONL, `session_meta` + `response_item` records | **P0** |
| gemini-cli | `~/.gemini/tmp/<project-hash>/…` | JSON/JSONL session files — **verify schema first** | P1 |
| opencode | `~/.local/share/opencode/opencode.db` | SQLite (sessions/messages tables) | P1 |

**Mandatory verification step before writing each adapter:** inspect real data where available
(this machine currently has real OpenCode data at `~/.local/share/opencode/`; no Claude Code /
Codex / Gemini data — build those from documented formats and fixtures). Document the actual
observed schema in `internal/adapters/<agent>/SCHEMA.md`. **Never commit real user data —
all fixtures must be synthetic and anonymized.**

OpenCode specifics: open SQLite strictly read-only (`?mode=ro`). If the DB is locked by a
running OpenCode instance, warn once and continue with other agents.

## 5. Unified model and adapter interface

```go
type Session struct {
    ID        string
    Agent     string    // "claude-code" | "codex" | "gemini-cli" | "opencode"
    Project   string    // working dir, "" if unknown
    Title     string    // optional
    StartedAt time.Time
    EndedAt   time.Time // optional
    SizeBytes int64
}

type EntryKind int // Message, ToolCall, ToolResult, Summary

type Entry struct {
    Kind      EntryKind
    Role      string    // user/assistant/tool — best effort
    Text      string
    Timestamp time.Time // optional
}

type Adapter interface {
    Name() string
    Detect() bool                                   // storage present?
    Sessions(iter func(Session) error) error        // streaming; never loads whole files
    Entries(s Session, iter func(Entry) error) error // streaming
}
```

Streaming is mandatory: `bufio.Scanner` with an enlarged buffer (`Scanner.Buffer`,
handle lines > 64 KB), early cutoff by date/agent/project filters before parsing.

## 6. Repository layout

```
cmd/pastlog/main.go
internal/cli/            command wiring (spf13/cobra), help texts, exit codes
internal/discovery/      home/XDG/USERPROFILE resolution per OS, --home override
internal/agentlog/       model, Adapter interface, registry
internal/adapters/       claudecode/ codex/ geminicli/ opencode/ (+ SCHEMA.md each)
internal/search/         literal + regex matcher, snippet builder, hit caps
internal/render/         human cards & tables, JSON renderer (stable schema)
internal/version/        ldflags-injected version info
```

Dependencies allowlist (keep it short): `spf13/cobra`, `fatih/color` or
`charmbracelet/lipgloss` (pick one), `modernc.org/sqlite` (pure Go, read-only — no CGo,
keeps cross-compilation trivial), test libs `stretchr/testify` optional. Everything else:
stdlib. Any addition must be justified in the PR description.

## 7. Performance

Budget: literal search over 500 MB of JSONL ≤ 300 ms (warm FS cache) on a 2020 laptop —
no index in v0.1, streaming only. Add a benchmark (`go test -bench`) over a synthetic
~200 MB fixture; measure MB/s and record the number in the README. Avoid `regexp`
unless `--regex` is given. Parse only the fields needed (small structs, `encoding/json`).

## 8. Errors and UX

- Agent storage absent → not an error; shown as "not found" in `agents`, skipped elsewhere.
- Corrupt/unknown lines → skip, count, summarize on stderr ("12 unreadable lines skipped").
- Ambiguous session-ID prefix → list candidates, exit 2.
- User-facing errors: lowercase, actionable, no stack traces (`2+ words of context` style).
- Colors via chosen styling lib; `NO_COLOR` / `--no-color` / non-TTY respected.

## 9. Testing

- **Fixtures per adapter** under `internal/adapters/<agent>/testdata/`: empty file, file
  without trailing newline, unicode content, >64 KB single line, truncated JSON line,
  anonymized realistic session. OpenCode fixture: commit a generator script that creates
  the SQLite DB at test time in a temp dir (do not commit a binary .db).
- Unit tests for discovery (per-OS home resolution via env overrides), matcher
  (case folding, regex compile errors, hit limits), prefix-ID resolution, renderers.
- Golden tests for human output and `--json` output.
- `go test -race ./...` clean; `golangci-lint run` clean; `gofumpt` formatted.
- Target ≥ 70% coverage on `internal/*` (excluding generated fixtures).

## 10. CI/CD and release

- `.github/workflows/ci.yml`: matrix ubuntu/macos/windows × amd64/arm64 (arm64 mac
  mandatory), Go stable; jobs: lint, test -race, bench smoke (small fixture).
- `.github/workflows/release.yml`: GoReleaser on `v*` tags — archives (`.zip` for Windows,
  `.tar.gz` elsewhere), SHA256 checksums file, GitHub Release. Homebrew tap + Scoop bucket
  repos are separate placeholders; config committed but unconfigured.
- `LICENSE` (MIT), `.goreleaser.yaml` must pass `goreleaser check`.
- macOS Gatekeeper caveat documented in README (`xattr -d com.apple.quarantine`) —
  notarization is out of scope for v0.1.

## 11. README (English) — publishing requirements

- One-liner: **"Search the full history of your AI coding agents — 100% local, one binary."**
- GIF demo: commit a `vhs` `.tape` file producing `demo.gif`; if rendering is impossible in
  this environment, leave a clear placeholder and the tape for later.
- Install: `go install`, Homebrew tap, Scoop, direct binary download — all four documented.
- Quickstart with realistic output blocks for every command.
- "How it finds your data" table with per-agent storage paths (transparency = trust).
- FAQ: "Is anything uploaded?" → no network code exists (link the test); "Is Windows
  supported?" → yes, first-class; "Does it modify my logs?" → read-only, enforced.
- Benchmarks table.
- Honest comparison table (section 1).
- Roadmap: MCP server (`pastlog mcp` — let the agent search its own history), TUI,
  token/project stats, optional index.

## 12. Out of scope for v0.1

No index, no TUI, no MCP server, no write/prune/delete operations, no stats, no semantic/AI
search, no config file (flags + `--home` only), no auto-update, no notarization.

## 13. Milestones

| # | Scope | Acceptance | Est. |
|---|---|---|---|
| M1 | Scaffold, model, discovery, claude-code adapter | `pastlog sessions` lists synthetic Claude Code sessions; unit+golden tests green | 4–6 h |
| M2 | Codex adapter, search engine, `show`/`--export md` | search finds hits across both adapters with correct exit codes; `--json` stable | 6–8 h |
| M3 | Gemini + OpenCode adapters (incl. read-only SQLite) | real OpenCode DB on this machine lists and searches; locked-DB fallback works | 4–6 h |
| M4 | Renderers, UX polish, no-network test, benchmarks | bench recorded; race/lint clean; coverage ≥ 70% | 3–4 h |
| M5 | CI, GoReleaser, README, LICENSE, demo tape | CI green on all matrix legs; `goreleaser check` passes | 3–4 h |
| M6 | Pre-flight publishing checklist (see §15) | report with green checkboxes; nothing committed under `testdata/` resembles real user data | 1 h |

## 14. Definition of done

- [ ] All commands behave per §3, verified against the real OpenCode data on this machine
- [ ] `TestNoNetworkDeps` green; read-only guarantee covered by tests
- [ ] `go test -race ./...`, `golangci-lint run`, `gofumpt` all clean; bench number documented
- [ ] CI green on windows/macos/linux; `goreleaser check` passes
- [ ] README per §11 complete; LICENSE present; fixtures synthetic/anonymized
- [ ] `pastlog agents` on a clean machine (no agents installed) prints a friendly
      "nothing found — install an agent or pass --home" and exits 0

## 15. Pre-flight publishing checklist (manual, M6)

- [ ] Final name check (re-run right before repo creation): `github.com/<owner>/pastlog`,
      npm name, Homebrew formula name, Scoop manifest. Fallbacks if taken: `agentgrep`, `lorelog`.
      *(Verified 2026-09-13: `pastlog` — free everywhere that matters: GitHub has no
      exact-match repo (only a 0-star personal "PASTLog" memo project, negligible), npm
      registry 404, Homebrew formula 404, Scoop main bucket 404. Previously rejected:
      `agentlogs` — taken (agentlogs/agentlogs); `chatgrep` — taken (chatgrep.com);
      `yore` — taken (pawamoy/yore).)*
- [ ] Repo init: git, first commit, description + topics (`cli`, `go`, `claude-code`,
      `codex`, `gemini-cli`, `opencode`, `ai`, `search`, `tui-ready`)
- [ ] Tag `v0.1.0` → release workflow produces binaries for all 6 targets
- [ ] Launch: Show HN, r/commandline, Terminal Trove submission, PR to awesome-cli-apps
