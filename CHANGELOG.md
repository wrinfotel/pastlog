# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
