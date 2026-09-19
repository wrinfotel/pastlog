# pastlog Desktop — packaged smoke checklist (Windows-first, R-D12)

Manual, scripted smoke for a packaged build (`wails build -webview2 embed`).
CI builds and uploads the artifacts; this checklist is the acceptance pass
that cannot run headless. Budget: ~10 minutes. Everything below is read-only
over agent storage — if any step writes outside the export target, FAIL.

## Preparation

1. Synthetic home (empty state):

   ```powershell
   mkdir C:\smoke-home
   ```

2. Record storage hashes (before):

   ```powershell
   Get-FileHash -Algorithm SHA256 "$env:USERPROFILE\.local\share\opencode\*" | Format-Table
   ```

   (On stock setups the OpenCode dir is `%LOCALAPPDATA%\opencode`.)

## Run

3. Launch `pastlog-desktop.exe` → the window opens with a dark (or system)
   theme and the **Home** view.
4. Home shows four agent cards; **opencode** shows the real session count
   (cross-check `pastlog agents` in a terminal — numbers must match).
5. Empty-state check: Settings → set the home override to `C:\smoke-home`
   → Save → Home now shows "nothing found — install an agent or set the
   home override". Clear the override → Home recovers.
6. **Sessions**: open the view → rows appear, newest first. Type a project
   substring into the project filter → the list narrows within ~¼ s. Scroll
   fast to the bottom of a long list — no blank rows, no jank
   (virtualization).
7. Click a row → the **viewer** opens: title/metadata header, transcript
   entries with timestamps, `copy` buttons. Tool calls and tool results
   start collapsed and expand on click. Assistant messages render as
   markdown (no raw HTML from logs is executed).
8. Click **Export JSON** → the OS save dialog opens → save to
   `C:\temp\smoke-session.json` → the file exists and equals
   `pastlog show <id> --json` byte-for byte:
   `fc.exe /b C:\temp\smoke-session.json <cli-out>` (or compare SHA256).
9. **Search**: type a query (e.g. a word you know is in your logs) → results
   appear within ~¼ s of keystrokes (250 ms debounce), matches highlighted.
   Long-corpus runs show a progress line with a working **Cancel** button.
   Click a hit → the viewer opens scrolled to that hit (highlighted border).
10. **Stats**: pick `--by model` → a table with sessions/messages/input/
    output/reasoning/cache/total (and `cost usd` when a source provides
    cost). Cross-check against `pastlog stats --by model` — numbers match.
11. **Diagnostics**: the `agents` table matches the CLI, warnings (locked
    storage, skipped lines) surface under the table.
12. Settings → About shows the released version/commit/date (matches the
    artifact's `desktop-v*` tag).
13. External links (if any log contains an `https://` link rendered in
    assistant markdown): clicking opens the **system browser**, never inside
    the app window.

## Finish

14. Storage hashes (after) — identical to step 2. The app must not have
    created, modified or deleted anything under the agent storage.
15. The only new files are: the chosen export file(s) from step 8 and the
    settings file in `%APPDATA%\pastlog-desktop\config.json`.
