# Regression and bug-fix history

Each entry: problem · diagnosis · cause · files · fix · test · how to recognise it again.
Newest first.

## 0.7.0 (in development)

### Home showed a menu instead of the Home screen
- **Problem:** pressing Home during a game showed a screen with *Return to game / Close game /
  Show performance monitor*. Choosing the running game on Home asked *Cancel / Close / Resume*.
- **Diagnosis:** source reading. `emulator.home` correctly opened Home, but the NEXUS window then
  received focus (`app.focus` event) and `showPlaying()` replaced Home with the "playing" screen,
  even though the game was suspended. Selecting the suspended game opened a dialog.
- **Files:** `app/web/app.js` (`app.focus` handler, `launchGame`).
- **Fix:** `app.focus` only shows the playing screen when the game is in front; selecting the
  running game resumes it directly (closing it stays on the Home banner).
- **Recognise:** any handler that calls `showPlaying()` must check `S.playing.suspended`.

### Keys and firmware missing after extracting a new version
- **Problem:** a new NEXUS folder started without the keys and firmware installed before.
- **Cause:** portable copies (`portable.txt`, recommended for test builds) keep all data in their
  own folder, so a new folder has none.
- **Files:** `app/systemvault.go`, `main.go`, `systemfiles.go`; tests `systemvault_test.go`.
- **Fix:** a per-user vault, synchronised at start in both directions (newer file wins, nothing
  deleted), and a one-time search of sibling NEXUS folders for an earlier copy's keys.
- **Test:** `TestSystemVault`, `TestSystemVaultFindsEarlierCopy`.

### Ocarina of Time stuck on its loading screen (open)
- **Status:** not reproduced (no game in CI). See `COMPATIBILITY.md` for what was ruled out and
  what the next test must collect.

## 0.6.0
- F11 stuck: full-screen state read before the previous change was applied; now atomic
  (`ToggleFullscreen`) and every press counted (`engine.go`, `app_windows.go`).
- Keyboard not reaching a game drawn in the NEXUS window: clicks on a child window do not move
  the keyboard focus; engine patch 0017 gives focus back while the game is in front.
- Loading-screen Cancel: asked a question the game could cover; now stops at once.
- Duplicate play statistics: play time recorded under the old file-name key (`f-…`) before the
  Title ID was readable; merged after each scan (`statsmerge.go`).
- Tests writing into removed folders: online artwork worker and game sessions outliving tests;
  `App.Close` waits for the worker, test apps stop sessions (`artwork_online.go`, fixtures).

Earlier fixes: see `CHANGELOG.md` and `docs/SOURCE_AUDIT.md`.
