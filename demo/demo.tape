# demo.tape — vhs tape for the README demo GIF (spec §11).
#
# Render with vhs (https://github.com/charmbracelet/vhs) from the repo root:
#
#   vhs demo/demo.tape        # writes demo/demo.gif
#
# Requirement: the `pastlog` binary must be on PATH at render time
# (e.g. `go install github.com/pastlog/pastlog/cmd/pastlog@latest` first),
# and Go must be available: the hidden setup block runs the committed fixture
# generator (`go run ./demo/genopencode`) to create the synthetic OpenCode
# store under demo/fixture-home — no binary .db is committed (see
# demo/README.md).
#
# demo.gif itself is NOT committed — render it once, then reference it from
# the README (a placeholder marks the spot).
#
# Privacy: the tape always records against demo/fixture-home (synthetic,
# anonymized sessions checked into the repo) via the alias below — never
# against your real agent data. The hidden block also unsets the per-OS
# OpenCode data-dir overrides (XDG_DATA_HOME / LOCALAPPDATA) so discovery
# can only resolve the fixture home's store.

Output demo/demo.gif
Set FontSize 21
Set Width 1100
Set Height 680
Set Padding 42
Set Theme Dracula
Set Shell bash

# Not recorded: generate the fixture home's synthetic OpenCode database
# (before the overrides are unset — `go run` needs %LocalAppData% for its
# build cache on Windows), then pin the data-dir overrides and point
# `pastlog` at the fixture home.
Hide
Type "go run ./demo/genopencode" Enter
Type "unset XDG_DATA_HOME LOCALAPPDATA" Enter
Type "alias pastlog='pastlog --home $PWD/demo/fixture-home'" Enter
Ctrl+L
Show

Type "pastlog" Enter
Sleep 4s
Ctrl+L

Type "pastlog agents" Enter
Sleep 3s
Ctrl+L

Type "pastlog sessions --limit 5" Enter
Sleep 3s
Ctrl+L

Type "pastlog search 'jwt refresh'" Enter
Sleep 3s
Ctrl+L

Type "pastlog show 3f9c81a2" Enter
Sleep 4s
Ctrl+L

Type "pastlog version" Enter
Sleep 3.5s
