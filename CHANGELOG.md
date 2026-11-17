# Changelog

All notable NEXUS changes will be recorded here.

The project is in preview. Releases are numbered from **0.0.0**; the first intended stable release is **1.0.0**. Every release, with its notes and download, is on the [Releases page](https://github.com/0ryzin/Nexus/releases).

## 0.7.0 — in development (not released)

- **Home goes straight to the Home screen.** The game keeps running in the background (or paused, as chosen); choosing it on Home resumes it at once. The "playing" screen no longer appears over Home when the NEXUS window gets focus.
- **Keys and firmware stay installed across versions.** They are kept in a per-user vault (`%LOCALAPPDATA%\NEXUS\SystemFiles`; Linux `~/.local/share/NEXUS-SystemFiles`) and restored into any copy of NEXUS at start, portable or not; an earlier portable copy next to the new one is found once. Newer files are never replaced by older ones; nothing is deleted or uploaded.
- **HDR output** setting (*Emulation settings › Graphics*): Off, Auto, On, using the engine's native HDR10 output (Lichii #1326, already in the engine); falls back to SDR when the display offers no HDR format. Not tested on an HDR display.
- Ocarina of Time stuck loading: investigated (see docs/COMPATIBILITY.md); not reproduced, not yet fixed.
- Knowledge base: CLAUDE.md, docs/DEVELOPMENT_NOTES.md, COMPATIBILITY.md, REGRESSION_HISTORY.md, research/LICHII.md, research/FRITCHII.md, benchmark method in PERFORMANCE.md.

## 0.8.0 — in development (not released)

- **NEXUS Engine 0.8.0 on Lichii build 177** (was build 160): 87 upstream engine changes, among
  them updates no longer applied over NUP programs (a likely cause of Ocarina of Time staying
  black after a save loads), Switch 2 GPU-service waits that could never end, faster x86-64 code
  generation, GPU split and buffer-cache work, memory reuse and Windows commit-limit handling.
  The NEXUS host patches were re-applied on top; the yuzu project's copyright notices that
  Lichii removed are kept. Not yet tested with games.
- Starting a Switch 2 game on a PC with less than 16 GB of RAM shows a warning (as Lichii #1478 does).
- The optimiser keeps Switch 2 games on the engine's tested settings (reactive flushing on,
  conservative video memory, no scaling above 1x), and each Switch 2 session logs the settings
  and update files it started with.
- Windows: NEXUS no longer sometimes starts without a window; starting it again brings the running
  window forward. The performance bar appears without pressing F11.
- Controller: close a game kept on Home from its Options menu, the Power menu or the Home banner.
  Fixed Home trapping the controller when pressed during the loading screen's fade, and the loading
  screen waiting forever when a game started quickly.
- Shaders are prepared before a game plays (Settings › Game sessions, on by default): NEXUS finds
  the shader programs in the game's own files, and the engine compiles them before the game starts
  (once per game and driver; *Start now* skips). A game's Options menu has *Prepare Shaders Now*.
  See docs/SHADERS.md for what is and is not prepared in advance. Not measured with a game yet.
- Engine on Lichii build 316 (283 upstream changes, several for Ocarina of Time); games from NSP/XCI
  files now get their control data (NACP).
- Shaders: the loading screen and the performance bar show pipelines ready from the cache, builds
  compiling now, background preparation and new ones this session.
- NEXUS Account: keeps synchronising with a game kept on Home, after sleep and when NEXUS comes to
  the front; a passing GitHub error no longer signs the device out; switching accounts never
  copies one account's data into another without asking.
- Idle: less work with a controller connected and behind a running game (measured, see
  PERFORMANCE.md).
- Engine: the texture cache's host-memory cap now works on Linux too (for 16 GB shared-memory
  handhelds; not tested on one).

## 0.6.0 — test build (pre-release)

0.5.0 was never released; its changes below ship in 0.6.0 together with:

- **NEXUS Account with GitHub** (sign in with a code at github.com/login/device; synchronisation through a private Gist in the user's own GitHub account; no NEXUS server). Play time synchronises (each device's time, added up). Automatic synchronisation when a game closes. The self-hosted account service (`app/accountd`) remains for advanced use.
- **Home is Shift+H** (H alone goes to the game; existing installations moved from H).
- **Controller hot-plug** during a game (NEXUS Engine 0.7.0, patch 0017).
- **Keyboard focus** stays with a game drawn in the NEXUS window (patch 0017).
- **F11** no longer gets stuck (atomic full-screen state; every press counted).
- **Cancel** on the loading screen stops at once; *Who's playing?* has a Cancel button.
- **Launch animation** on the game's Home icon, then the loading screen.
- **Correct play time** (hours and minutes) per game and user.
- Published as a pre-release (`<!-- nexus:prerelease -->` in the release notes).

## 0.5.0 — not released (included in 0.6.0)

- **Import Save from ZIP:** saves from any compatible source; nested folders, several saves per archive, a preview with per-save selection, progress, all-or-nothing import with backups; unsafe archives are refused (paths outside the target, links, passwords, unusual compression ratios, damage) with an explanation of what to change. The Lichii-specific import source is removed.
- No other emulator names in the interface; acknowledgements are in *Settings › System › Legal* and the Licences folder (its README was out of date and is corrected).
- **Graphics card:** on PCs with two GPUs, games run on the dedicated card (NEXUS chooses the Vulkan device instead of the engine's index 0); *Integrated graphics* is an option. Known problem Vulkan layers (Overwolf) are reported.
- **Android (source only, not released):** the shared core runs on Android arm64 (`StartCore`, dlopen engine host, readiness check, lifecycle, thermal and memory handling); the Android app (`android/`) with WebView interface, device detection, storage access and on-screen controller; CI builds and tests it and starts it on an emulator. The NEXUS Engine is not built for Android yet, so games cannot run there. See docs/ANDROID.md.
- **One window:** games are drawn inside the NEXUS window instead of a window of their own (NEXUS Engine 0.6.0, patch 0016). F11 makes the NEXUS window full screen and restores its previous size and position; the performance monitor and REC indicator follow the NEXUS window and hide when it is minimised or behind another window. *Settings › Game sessions › Game window* can return to a separate window. Not yet tested with a real game on Windows.
- **Home key H** works for keyboard players: the engine's default keyboard layout put the R button on H. Default bindings on NEXUS shortcut keys now move to a free key for the session, with a message; bindings you chose are never changed.
- **Advanced emulation settings** (global and per game): CPU clock and GPU clock (Normal, Boost, Overclock), Synchronise core speed, speed limit 10–1000 % with 50/100/200 % presets and a custom value, multi-core, and emulated memory 4/6/8/10/12 GB. NEXUS refuses a memory layout the PC does not have the memory for.
- **Amiibo:** *Settings › Amiibo* library: import dumps of your own amiibo (checked, copied; the original is unchanged), create NEXUS virtual amiibo from a name (looked up in the open AmiiboAPI database) or an amiibo ID, rename, assign to a user, details, remove; optional user-supplied amiibo keys. When a game scans for an amiibo, NEXUS shows the library and gives the game the one you choose. Virtual amiibo work only in NEXUS and are not authenticated Nintendo amiibo. NEXUS includes no amiibo data or keys.
- **NEXUS Account:** create an account, sign in, link another PC or a Steam Deck with a one-time code that the signed-in device approves, device list with removal, sign-out, password change, recovery code. Synchronises users, library names and choices, NEXUS preferences and the amiibo library (emulation settings optional) between devices: works offline, sends changes later, and asks you when the same item was changed on two devices. *Settings › NEXUS Account*, the cloud icon on Home and in the dock. The account service (`app/accountd`) is included and tested but **not deployed yet**, so accounts cannot be created in this build (docs/ACCOUNTS.md).
- **Amiibo Creator:** search the open amiibo database by name, character or series, or type an ID; display name, character, series, type, user, notes and your own picture; live preview; edit, duplicate and remove; library search and filter. Entries are checked again before a game receives them.
- A game left running in the background is paused after 15 minutes (configurable, or never).
- **Performance overlay redesigned** after the handheld style: a thin translucent strip over the game's picture with coloured labels, large values and small units, scaled to the picture's height; or the frame rate only, in a corner. It now belongs to the window the game is drawn in (not a topmost desktop window): it follows moves, resizes and F11 at once, sits on the picture (not the black bars), and hides when the game is minimised or closed. The REC indicator works the same way. Not yet checked on a real Windows desktop.
- F11 returns a maximised window to maximised.
- Fixed: a game played before NEXUS could read its Title ID (no keys yet, or a NUP package before 0.3.0) appeared twice in a user's play statistics. Play time recorded under the old file-name key is merged into the game after each library scan; different games, regions and editions (other Title IDs) are never merged.
- NEXUS identifies itself as 0.5.0 (VERSION); development builds without a version say 0.5.0-dev.
- NEXUS Account buttons explain why accounts are not available yet instead of doing nothing; the Amiibo Creator reports any problem opening it and logs it.
- Docs: ANDROID, ENGINE_RESEARCH, ACCOUNTS, RELEASES_1.0 (plan); SOURCE_AUDIT and ARCHITECTURE updated.

## 0.4.0 — Windows x64 (preview)

- Screen recording (F12): game window only, game sound and optional microphone, hardware encoding (NVIDIA NVENC, AMD, Intel Quick Sync) with software fallback, fragmented MP4 written safely with recovery after a crash, REC indicator, saved/failed messages, *Settings › Screen recording*. New module `capture/` (`nexus_capture.dll`).
- Screenshots move to Alt+F12 (migrated from F12); shortcuts accept Ctrl/Alt/Shift.
- Home keeps the game running in the background (muted, input blocked by the engine), with a green indicator and Resume; *Pause the game* is an option. NEXUS Engine 0.5.0 (patch 0015).
- Save import from Lichii's save folder (read only, backup before replacing).
- The upstream game-list cache is no longer read and is removed from NEXUS's engine data.
- Interface idle CPU on Home from 5.3 % to 0.3 % of a core, Library from 12.3 % to 0.4 % (docs/PERFORMANCE.md).
- Appearance page always shows the current theme and accent.
- `NEXUS.exe` version information: CompanyName "Nexus Development" (metadata only; NEXUS is not code-signed, docs/SIGNING.md).
- Release page: Windows download table generated from the built ZIP with its SHA-256; the published download is downloaded again and checked, or the release returns to draft.
- Docs: SOURCE_AUDIT, ARCHITECTURE, PERFORMANCE, SIGNING, mobile research (replaced by ANDROID in 0.5.0).
- Release notes: [docs/releases/v0.4.0.md](docs/releases/v0.4.0.md).

## 0.3.0 — Windows x64 (preview)

- NEXUS Engine 0.4.0: upstream Lichii build-160 (30 commits: Ocarina of Time guard-house freeze fix, Vulkan pipeline/memory fixes, shader and CPU performance work, stability fixes) + patch 0014.
- Fixed: the controller Home button opened the game's "return to HOME" prompt and could flicker between the game and NEXUS. The button no longer reaches the game, and one press only ever goes to NEXUS Home.
- Fixed: Switch 2 games in NUP packages (such as the Ocarina of Time remake) had no title or icon from their own files; NEXUS now reads NUP metadata, and NSP/XCI icons are read more tolerantly.
- Compatibility: problems recorded with an older engine are history, not the current status; known engine fixes are named.
- Save Data overhaul: `UserData\Saves\<PLAYER_ID>-<name>\<GAME_ID>-<SHORT>`, read and written by the engine directly; import with player/game identification (folder IDs, Ryujinx metadata), export, backup, restore, delete; one-time migration from 0.2 with a backup first.
- Media tab: screenshots (F12 / controller capture button) with game and date; recordings placed in the Media folder are shown and played.
- F11: fullscreen/window while playing. Smoother loading-to-game transition (the loading screen fades out before the game window appears).
- Controllers: remembered by serial number where available, chosen per user and player, own layouts.
- Game Updates and Mods per game, checked by content; disabled mods passed to the engine.
- NVIDIA DLSS files: detection, consent with NVIDIA's licences, download from NVIDIA's Streamline release, checksum and signature check.
- Automatic NEXUS updates: check, download, SHA-256 check, staged install, rollback if the new version does not start. A read-only access token is needed while the repository is private.
- Removed: importing other emulators' system files and data (Lichii, Eden, Ryujinx …). NEXUS manages its own system files.
- Light theme: white background with light-grey panels; selected and hovered states use the accent in both themes.
- Release notes: [docs/releases/v0.3.0.md](docs/releases/v0.3.0.md).

## 0.2.0 — Windows x64 (preview)

- Fixed: a black window with a white frame covered the screen while a game loaded. NEXUS Engine 0.3.0 (patch 0013) keeps the game window hidden until its first frame.
- Loading screen shows real shader-cache progress (count, not an invented percentage).
- NEXUS Home: keyboard key (default H, configurable or off, conflict detection) and controller Home/Guide button; background sessions (muted, no input) with a running indicator, Resume Game and Close Game.
- Performance monitor overlay (FPS, frame time, CPU/GPU use and temperature, VRAM, RAM, RAM speed; N/A when unavailable), global or per game.
- Frame-rate limit for Switch 2 games with presets, "match my display", VSync and speed-limit explanations; detected refresh rates in the hardware panel.
- Platforms: Windows available; Linux and macOS planned (docs/PLATFORMS.md).
- Release notes: [docs/releases/v0.2.0.md](docs/releases/v0.2.0.md).

## 0.1.0 — Windows x64 (preview)

- Fixed: games failed with "Legacy emulator data or an unfinished import was found…" on PCs with another Switch emulator installed. The NEXUS Engine (0.2.0) now treats its NEXUS-chosen data folder as host-managed.
- NEXUS creates its engine data and configuration itself; NEXUS's own import of other emulators' data (copy only, backup, start fresh, interrupted-import recovery).
- New launch flow: Who's playing? → artwork opening → NEXUS loading screen → game.
- Launch errors in NEXUS's words, with technical details and a diagnostic log.
- docs/ENGINE_SOURCES.md: engine origin, other projects, support status.
- Release notes: [docs/releases/v0.1.0.md](docs/releases/v0.1.0.md).

## 0.0.0 — first public preview (Windows x64)

- NEXUS application with the NEXUS Engine 0.1.0 (Lichii build-150 + NEXUS patches) in one ZIP.
- First-time setup, up to 8 users, profile colour and picture, light and dark themes.
- Controller layouts for all games or per game; game presets.
- Frame generation (FSR 3 / Lossless Scaling), NVIDIA DLSS in place of a game's DLSS, AMD FSR 4 — offered according to the detected graphics card.
- Hardware detection and "Optimise for my device"; readable save folders with export/import; animated wallpapers; 0–200% game volume.
- Release notes: [docs/releases/v0.0.0.md](docs/releases/v0.0.0.md).

## Repository setup

- Initial repository documentation and licensing information.
- README rewritten to describe the current development state and engine limitations.
- Added engine architecture/status documentation.
- Added application and engine versioning policy.
- Added third-party acknowledgement and contribution guidance.

No stable release has been published from this repository yet.
