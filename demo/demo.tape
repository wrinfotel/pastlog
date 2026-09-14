# demo.tape — vhs tape for the README demo GIF (spec §11).
#
# Render with vhs (https://github.com/charmbracelet/vhs) from the repo root:
#
#   vhs demo/demo.tape        # writes demo/demo.gif
#
# Requirement: the `pastlog` binary must be on PATH at render time
# (e.g. `go install github.com/pastlog/pastlog/cmd/pastlog@latest` first).
#
# demo.gif itself is NOT committed — render it once, then reference it from
# the README (a placeholder marks the spot).
#
# Privacy: the tape always records against demo/fixture-home (synthetic,
# anonymized sessions checked into the repo) via the alias below — never
# against your real agent data.

Output demo/demo.gif
Set FontSize 21
Set Width 1100
Set Height 680
Set Padding 42
Set Theme Dracula
Set Shell bash

# Not recorded: point `pastlog` at the synthetic fixture home.
Hide
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
