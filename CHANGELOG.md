# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Per-model token breakdown in stats (TASK.md backlog): sessions that
  switched models mid-way now credit every model they used, not just the
  latest one. `pastlog stats --by model` shows one row per used model with
  that model's own tokens (the `sessions`/`messages` cells on a model row
  count the sessions that used it); the desktop project drill-down splits
  the same way. Supported by zcode (per-request `model_usage` rows),
  claude-code (per-message `model` + `usage`) and gemini-cli (per-record
  `model` + `tokens`, legacy store included) exactly, and by codex via the
  per-turn `last_token_usage` delta (best-effort after compaction resets).
  opencode keeps a single model per session — its storage has no per-model
  data. Aggregate totals and the agent/project/day views are unchanged: a
  session still counts once outside the model view.

### Changed

- `stats --model` now matches sessions that used the model anywhere in the
  session (previously only the latest model counted), so a mid-session
  model is no longer invisible to the filter. The filter still selects
  whole sessions — tokens are not cropped.

## [0.2.0] - 2026-09-26

The CLI and the desktop app ship from this entry on their own version
lines: the CLI as `v0.2.0`, the desktop app as `desktop-v0.2.0`
(`desktop-v0.1.0` went out mid-cycle).

### Added

- ZCode adapter (agent `zcode`) — pastlog now reads ZCode's session store at
  `~/.zcode/cli/db/db.sqlite` in both the CLI and the desktop app. Sessions
  (including subagent children), transcripts (messages, tool calls and
  outputs, reasoning) and per-session token usage aggregated from ZCode's
  `model_usage` table (model = the latest request's model; ZCode reports no
  cost). Read-only (`mode=ro`), streaming, locked-DB fallback: one warning,
  other agents unaffected. Documented in
  `internal/adapters/zcode/SCHEMA.md`.
- **pastlog Desktop** — the GUI companion to
  the CLI, built with Wails over the same Go engine. Home/Diagnostics
  summaries, virtualized Sessions with the full CLI filter set, live Search
  with progress + cancel and highlighted hits, a transcript viewer with
  collapsible tool calls and sanitized markdown, token Stats with plain-CSS
  bars, Settings (home override, theme). JSON export is byte-identical to
  the CLI's `--json` (tested); read-only, 100% local, zero telemetry; strict
  CSP; frontend embedded in the binary, no external assets. Windows
  (installer + portable, embedded WebView2 bootstrapper fallback), macOS
  (.dmg), Linux (.deb with declared webkit dependencies).
- Desktop: clickable agent cards on Home drill into a Projects view — one
  agent's projects with their usage aggregates (`stats --by project`
  in-process, keyed by the exact project path), and a per-project page with
  the per-model token/cost breakdown (`stats --by model` narrowed to the
  project's own sessions) plus the project's session list. Scans stream
  progress and are cancellable.
- Desktop: the mint workspace redesign — Home rebuilt as a landing-style
  overview fed by real data only (no mock or invented values anywhere),
  every secondary view restyled into the same design language, one back
  strip across nested views, and the sessions/search/stats filters
  consolidated into a single bar.

### Changed

- `render.GroupStatsKeys`: the raw-key accumulator behind `GroupStats`,
  exported for callers that group by an untransformed key (the desktop
  Projects view groups by the stored project path, not the tilde-shortened
  display form). CLI behavior unchanged.
- `internal/render` JSON shapes are now exported types (same tags and order
  — schemas unchanged) so the CLI and the desktop app share one definition.
- `TestNoNetworkDeps` scopes the no-network audit to the data path plus the
  desktop service glue (`cmd/...`, `internal/...`, `desktop/app`); the wails
  webview shell is excluded by design, the frontend is audited by a bundle
  scan instead.

### Fixed

- Desktop: the light theme choice works on dark-OS machines — the light
  palette only existed inside the `prefers-color-scheme` media query, so
  the explicit setting changed nothing while the OS was dark.
- Desktop: the search and stats filter fields re-run the live query on
  their own change instead of only taking effect at the next keystroke
  elsewhere.
- Desktop: the transcript viewer's collapse/expand toggles work again — the
  per-entry state lived in a value rebuilt by a derivation, so every click
  was silently dropped (broken since 0.1.0).
- Desktop: clicking a search hit scrolls the viewer to that entry via a new
  `entry_head` GUI anchor (the head of the hit entry's full text). Matching
  the snippet line by substring failed for windowed or synthesized lines —
  most real hits, and every tool hit. The CLI `--json` search schema is
  unchanged; the field exists on the GUI surface only.
- Desktop: the sidebar's Local workspace path follows the Settings home
  override immediately instead of waiting for an app restart.

## [0.1.0] - 2026-09-18

Initial MVP release: a single static, 100%-local, read-only binary that
searches the full session history of four AI coding agents — Claude Code,
Codex CLI, Gemini CLI and OpenCode — across all projects on the machine.
`pastlog` (bare) summarizes detected agents; `agents`, `sessions`, `search`,
`show` (with `--export md` and stable `--json` output everywhere) and `stats`
cover listing, cross-agent search, transcript printing and token-usage
aggregation; `--home` overrides the storage root. Streaming architecture
(no index), grep-style exit codes (0/1/2), `NO_COLOR`/`--no-color` support,
defensive per-agent adapters that skip and count malformed records instead
of failing.

### Added

- Cross-agent search (`search`), session listing (`sessions`, `agents`),
  transcript printing with markdown export (`show`, `show --export md`) and
  machine-readable stable `--json` output on every command.
- `pastlog stats` — token statistics across all four agents: aggregate
  input/output/reasoning/cache tokens (and OpenCode's per-session cost)
  grouped by `--by agent|project|day|model`, filterable with
  `--agent`, `--project`, `--model` (case-insensitive substring),
  `--since`/`--until`, with a human table (thousands separators, cost column
  only when cost data exists) and stable `--json` output. An empty selection
  exits 0; adapters without usage data are skipped silently; a locked or
  unreadable OpenCode database degrades to one stderr note while the other
  agents keep contributing.

[Unreleased]: https://github.com/wrinfotel/pastlog/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/wrinfotel/pastlog/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/wrinfotel/pastlog/releases/tag/v0.1.0
