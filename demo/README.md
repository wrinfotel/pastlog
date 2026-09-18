# pastlog demo

`demo.tape` renders the README demo GIF with
[vhs](https://github.com/charmbracelet/vhs) (from the repo root:
`vhs demo/demo.tape`, writing `demo/demo.gif` — not committed). It always
records against `demo/fixture-home`, never against real agent data.

## Fixture home

`demo/fixture-home` is a synthetic, anonymized agent-data home: claude-code
and codex JSONL sessions, a gemini-cli JSONL session, and an OpenCode
database. Per spec §9 no binary `.db` is committed: the OpenCode store is
created at render time from committed source,

```sh
go run ./demo/genopencode
```

which writes `demo/fixture-home/.local/share/opencode/opencode.db` (the
directory is gitignored). The generator is idempotent — it replaces an
existing store, so re-rendering is safe. Its two deterministic sessions
reproduce exactly the opencode lines of the README quickstart blocks
(`pastlog`, `agents`, `sessions`); the schema mirrors the observed OpenCode
tables (`internal/adapters/opencode/SCHEMA.md`) minus the declared perf
indexes, which the adapter's grouped scans never need here, so the tiny
store lands at the 7-page (28 KiB) footprint the README `agents` output
shows.

## Render requirements

- `pastlog` on `PATH` at render time
  (`go install github.com/wrinfotel/pastlog/cmd/pastlog@latest`).
- Go, for the generator step above.
- `vhs` itself.

The tape's hidden setup block unsets `XDG_DATA_HOME` / `LOCALAPPDATA` (so
the OpenCode storage root resolves to the fixture home on any OS), runs the
generator, and aliases `pastlog` to `--home $PWD/demo/fixture-home`.
