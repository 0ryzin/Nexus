# Changelog

All notable NEXUS changes will be recorded here.

The project is in preview. Releases are numbered from **0.0.0**; the first intended stable release is **1.0.0**. Every release, with its notes and download, is on the [Releases page](https://github.com/0ryzin/Nexus/releases).

## 0.5.0 — in development (not released)

- **Import Save from ZIP:** saves from any compatible source; nested folders, several saves per archive, a preview with per-save selection, progress, all-or-nothing import with backups; unsafe archives are refused (paths outside the target, links, passwords, unusual compression ratios, damage) with an explanation of what to change. The Lichii-specific import source is removed.
- No other emulator names in the interface; acknowledgements are in *Settings › System › Legal* and the Licences folder (its README was out of date and is corrected).
- **Graphics card:** on PCs with two GPUs, games run on the dedicated card (NEXUS chooses the Vulkan device instead of the engine's index 0); *Integrated graphics* is an option. Known problem Vulkan layers (Overwolf) are reported.
- **Android (source only, not released):** the shared core runs on Android arm64 (`StartCore`, dlopen engine host, readiness check, lifecycle, thermal and memory handling); the Android app (`android/`) with WebView interface, device detection, storage access and on-screen controller; CI builds and tests it and starts it on an emulator. The NEXUS Engine is not built for Android yet, so games cannot run there. See docs/ANDROID.md.
- **One window:** games are drawn inside the NEXUS window instead of a window of their own (NEXUS Engine 0.6.0, patch 0016). F11 makes the NEXUS window full screen and restores its previous size and position; the performance monitor and REC indicator follow the NEXUS window and hide when it is minimised or behind another window. *Settings › Game sessions › Game window* can return to a separate window. Not yet tested with a real game on Windows.
- **Home key H** works for keyboard players: the engine's default keyboard layout put the R button on H. Default bindings on NEXUS shortcut keys now move to a free key for the session, with a message; bindings you chose are never changed.
- **Advanced emulation settings** (global and per game): CPU clock and GPU clock (Normal, Boost, Overclock), Synchronise core speed, speed limit 10–1000 % with 50/100/200 % presets and a custom value, multi-core, and emulated memory 4/6/8/10/12 GB. NEXUS refuses a memory layout the PC does not have the memory for.
- **Amiibo:** *Settings › Amiibo* library: import dumps of your own amiibo (checked, copied; the original is unchanged), create NEXUS virtual amiibo from a name (looked up in the open AmiiboAPI database) or an amiibo ID, rename, assign to a user, details, remove; optional user-supplied amiibo keys. When a game scans for an amiibo, NEXUS shows the library and gives the game the one you choose. Virtual amiibo work only in NEXUS and are not authenticated Nintendo amiibo. NEXUS includes no amiibo data or keys.
- Docs: ANDROID, ENGINE_RESEARCH, RELEASES_1.0 (plan); SOURCE_AUDIT and ARCHITECTURE updated.

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
