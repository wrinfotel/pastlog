# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **pastlog Desktop (0.1.0, `desktop-v*` releases)** — the GUI companion to
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

[Unreleased]: https://github.com/wrinfotel/pastlog/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/wrinfotel/pastlog/releases/tag/v0.1.0
