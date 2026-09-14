# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - unreleased

Initial MVP release: a single static, 100%-local, read-only binary that
searches the full session history of four AI coding agents — Claude Code,
Codex CLI, Gemini CLI and OpenCode — across all projects on the machine.
`pastlog` (bare) summarizes detected agents; `agents`, `sessions`, `search`
and `show` (with `--export md` and stable `--json` output everywhere) cover
listing, cross-agent search and transcript printing; `--home` overrides the
storage root. Streaming architecture (no index), grep-style exit codes
(0/1/2), `NO_COLOR`/`--no-color` support, defensive per-agent adapters that
skip and count malformed records instead of failing.

[Unreleased]: https://github.com/pastlog/pastlog/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/pastlog/pastlog/releases/tag/v0.1.0
