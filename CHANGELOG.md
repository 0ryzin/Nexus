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

- **Linux and Steam Deck (built and tested by CI, not released):** one AMD64 build for every
  AMD64 Linux PC and the Steam Deck, plus ARM64; each as an AppImage (everything inside) and a
  tar.gz (uses the system's WebKitGTK 4.1). Gaming Mode is detected: full screen, no window
  decoration, F11 off. Linux updates are announced, with a link to the release page.
  See docs/LINUX.md. Not yet tested on a Steam Deck. Known issue: Switch 2 games' AV1 movies
  do not play on Linux yet (the Linux engine has no AV1 decoder).
- **Game saves sync between devices (NEXUS Account):** two-way, per game and user, with content
  hashes, versions and the sending device; unchanged saves are not re-sent; a newer save is never
  overwritten; both sides changed = a conflict the user resolves; the replaced save is backed up
  first; received saves are verified before use; works offline and catches up later; never blocks
  start-up or a game. Status in *NEXUS Account › Game saves*. Tested against a simulated GitHub
  (Windows ↔ Deck scenario); not yet tested between two real devices.
- **First start offers** Sign in with a NEXUS account / Create one / Continue with a local profile,
  so no throwaway profile is needed. A temporary local profile can be removed later ("Only on
  this device" or "Remove from account too"); its saves are archived, never deleted, and sync no
  longer brings it back.
- **Typing on the Steam Deck:** text fields open the Steam keyboard (falls back to NEXUS's
  controller keyboard) on a Deck or in Gaming Mode, also when the Deck's controls act as mouse
  and keyboard. *Settings › General › Typing* chooses. Not tested on a Deck.
- **Interface sounds on Linux:** the AppImage now carries the GStreamer plugins Web Audio needs and
  allows autoplay; CI records the start-up sound from a PulseAudio sink. Not tested on a Deck.
- **Steam:** NEXUS offers once to add itself to the app menu, which is what Steam's "Add a
  Non-Steam Game" lists (a downloaded AppImage is not there); then once to add its artwork
  (NEXUS wordmark only) to its Steam shortcut. NEXUS never adds or edits Steam shortcuts.
- **Steam Deck preset for Switch 2 games** (applied once per game on first start when nothing is
  set): 810p, FSR 1, low GPU accuracy, background shaders, 30 fps cap. The 12 GB memory layout is no
  longer refused for Switch 2 games. Not measured on a Deck.
- **Import Shader Data… / Export Shader Data…** in each game's right-click menu: export to a ZIP
  (shader pack + a description naming the game, format and checksums); import from a ZIP (NEXUS
  exports and original NEXUS-engine-family caches, recognised by header; other formats reported
  with the reason, nothing invented) or from another device linked by Device Linking (directly on
  the local network, never through the account). Background job with progress, backup first,
  only new entries added, the source never changed. Background shader compilation can use fewer
  threads ("Reduced", engine patch 0041).
- **Navigation:** Home screen and Themes merged into *Personalisation*; shortcuts to Shaders,
  Performance and Graphics removed from the dock (still in Settings); Home has no "Games" or "Recently played" section any more (Windows and Linux: one
  interface): the tabs above the game row and the Recently played card (Cards/Grid) are gone; the
  game row is newest first and ends with All Software. With favourites or own pages, the first tab
  is "Home". Recent-play data is kept (sorting, profile page). Clicking a profile opens it;
  "Play as" switches. The red expanding selection ring is now a quiet white/black ring.
- Test downloads: each file on its own; the CI bundles that unpacked to `out/` with both Linux
  files are renamed `zz-release-bundle-…-not-a-download`.
- Fixed: deleting a profile could hang NEXUS; Enter in the delete dialog cancelled instead of
  deleting; the name had to match in capitals ("Mins" now deletes "MiNS").
- **Ocarina of Time black screen below 12 GB (suspected cause, engine patch 0042):** when the
  engine does not recognise a Switch 2 game from its file before starting it, it ran it with the
  configured memory and the Switch 1 clock. NEXUS now tells the engine the title it identified;
  the engine log says when this was needed. Not confirmed with the game yet.
- Controller fixes found by a new controller-only sweep of every Home shortcut and settings
  category at the Deck's 1280×800: Right on a settings category now goes into the panel (it went
  to the search box), and Down past a short panel's last row no longer jumps into the category list.
- **Frame generation shows the truth:** while a game runs, NEXUS shows whether frame generation
  is really running or why not (engine patch 0033). The built-in method is named for what it is,
  NEXUS's optical-flow interpolation on any Vulkan GPU, instead of "AMD FSR 3".
- **NVIDIA Smooth Motion on Linux** as a frame generation method, offered only where the NVIDIA
  driver (575+), an RTX 40/50 card and the driver's layer are found; NEXUS asks the driver for it
  and shows it as requested (the driver does not report back). Not tested on an RTX card.
- Linux: graphics cards and displays are detected (sysfs); DLSS and FSR 4 are Windows-only.
- **Switch 2 Pro Controller over USB (experimental):** SDL 3.4.20 with libusb from Lichii
  build-341 (engine patches 0031/0032). On Linux it needs a udev rule the user installs (shipped,
  never installed by NEXUS). Not tested with the controller.
- **Switch 1 fixes from Lichii build-341** (engine patches 0034-0038): dynarmic invalidation race,
  slope depth bias at higher resolutions, VP9 video, Switch Sports black water.
- **Game menu with a controller on Home:** X (the Search button) on a game opens the same menu
  as a right-click (Options/Y and keyboard X still work); also from the Cards/Grid details panel.
  X elsewhere on Home still opens the Library search.
- **OOT Launch**, a second startup animation (*Settings › General › Startup animation*): red
  power-on, a pixel emblem changing colour, a white shield becoming a green-and-gold "40TH
  ANNIVERSARY" crest over a triangle mosaic, white and gold flashes, then Home, which keeps the
  gold pattern as its background. Drawn by NEXUS with its own synthesised sound; no Nintendo
  artwork, characters, logos or audio. The standard animation stays the default.
- **Restore defaults:** every setting has a small restore button once it differs from its
  default (controller: Options on the setting); *Reset all settings* and *Restore all keybinds*
  ask first and say what is kept. Users, saves, games, keys, firmware, the GitHub connection,
  library folders and per-game settings are never reset.
- **On-screen keyboard** after the Steam Deck's, for every text field: opens when a field is
  chosen with a controller (A, or the Search button), never with the mouse or a physical keyboard.
- **Search in a game's settings**, with *This game* / *Global* labels and *Only this game's*.
- Fixed: typing H, X, Q or E in the settings search (and in e-mail and password fields) went
  Home, opened Options or switched tabs.
- **GitHub stays connected.** NEXUS signed out of GitHub about eight hours after each sign-in:
  GitHub's sign-ins now last eight hours and come with a refresh token that NEXUS threw away.
  NEXUS now keeps it (encrypted with Windows data protection) and renews the sign-in by itself;
  only *Disconnect* or revoking NEXUS on GitHub ends it, never a network problem. Connect once
  more after updating. The account panel shows the connection on its own (Connected, Reconnecting,
  Authentication expired, Temporary connection problem, …) with *Synchronise now*, *Test
  connection*, *Reconnect* and *Disconnect*.
- **Settings regrouped** (General, Personalisation, Games, Controllers, Graphics & performance,
  Accounts & cloud, Advanced, About) with a **search box**, a *Changed* mark on settings that
  differ from the default, and *Advanced* sections for rarely used options. New panels: General,
  Shader management, About NEXUS. No setting or value was removed or reset (docs/SETTINGS.md).
- **Faster start without the startup animation:** Home in 147 ms instead of 235 ms after the
  interface loads (measured, Linux test build); the game list arrives with the first answer.
  The startup log records when Home became usable, the library, the account and the first sync.
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
- A game's shader data can be exported and imported between your own devices (Options menu);
  importing only adds what the device lacks, after checksum and format checks.
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
- Ocarina of Time closing NEXUS without a message when a save loads (17 Nov): two engine fixes
  Lichii made after build 316 are ported (#2041, a Windows race in the engine's memory tables that
  ends the process with an access violation; #2062, texture cache page-table bookkeeping). Most
  likely cause, not confirmed: no dump existed. **Not yet tested with the game.** See docs/CRASHES.md.
- Crashes are no longer silent: the engine writes a `Host crash` line and `eden_crash.dmp`;
  NEXUS re-enables Windows Error Reporting without its dialog, keeps Go's crash output, and after
  an unexpected close says so at the next start with a crash report (`Logs\crashes`, no keys or
  account details).
- Shader work on the loading screen and in *Prepare Shaders Now* shows the remaining time,
  estimated from measured throughput only ("Calculating remaining time..." until there is enough
  data; "Waiting: no progress" when it stalls; "Shader preparation complete." when done).
- **Loading: the boot shader precache no longer starts over.** When it was skipped with *Start
  now* (or the game closed during it), its work was forgotten, and Ocarina of Time rebuilt 4,671
  programs at every start (82–513 s on the user's PC). The engine now keeps what it built, and the
  next start builds only the rest. The new loading time is not measured yet (docs/PERFORMANCE.md).
- **Controllers are named by model** (vendor and product IDs), not as "Xbox 360 Controller":
  Xbox Series X|S, Xbox One/One S/Elite, DualShock 4, DualSense (Edge), Switch Pro, Joy-Con, Steam
  Deck. Unknown ones get a generic name. Not tested with physical controllers (docs/CONTROLLERS.md).
- **Button prompts follow the controller in use** (Xbox, PlayStation, Nintendo, generic), drawn as
  icons that name the physical button NEXUS reads. They change when another controller is used.
- Controllers connecting or disconnecting are announced by name. A button held while a controller
  reconnects does nothing. The **controller for NEXUS menus** can be chosen per user and is kept
  across restarts.
- **Manage Mods** (a game's Options menu): import a ZIP or folder, switch on or off, remove (to the
  Recycle Bin), details, compatibility with the game's build, conflicts, requirements, and what the
  engine applied at the last start. Mods are layered over the game by the engine; game files are
  never changed. The engine's built-in Ocarina of Time patches are listed for the game build
  they are made for (docs/MODS.md).
- **Lossless Scaling setup**: finds Steam from the registry and every Steam library (any drive),
  finds Lossless Scaling's folder from its app manifest, and checks Lossless.dll's shaders before
  copying. Identical files are not copied again; a working copy is replaced only when you choose.
  *Locate Lossless Scaling installation…* accepts the app's, a library's or Steam's folder
  (docs/LOSSLESS.md).
- The game menu is grouped (Play; Game: settings, mods, save data, information; Shaders; More…).
- Profile pickers: the selected profile keeps its accent ring; hovering another one only lightens
  it; keyboard and controller focus is a separate outline.
- NEXUS tries WebView2 again when it fails to start (it ended NEXUS once right after a crash). The
  engine's play-time and TAS folders are created (the engine logged errors without them).
- **Crash reports name the engine's functions.** Every build ships the engine's symbol table
  next to the engine (no separate download). It is used only for the exact engine build that
  crashed (docs/CRASHES.md).
- **Shared shader cache:** shaders one NEXUS user's games built are ready for every NEXUS user of
  the PC (`%ProgramData%\NEXUS\ShaderCache`). Only data that works on any graphics card is
  shared; each user's driver-specific files stay theirs. Can be turned off per user; a portable
  NEXUS keeps its own. Tested with the stand-in engine and two NEXUS data folders, **not yet with
  a game on two Windows accounts** (docs/SHADERS.md).
- **Home screen layouts:** Classic (as before), **Cards** (large cards, the selected game's
  picture and details) and **Grid** (games, folders and widgets in a grid, shortcuts down the
  side). Per user:
  - favourites, folders, custom pages (shoulder buttons), games hidden from Home;
  - order: recently played, A–Z, play time, date added, my order;
  - shortcut order and visibility, optional widgets, glass or flat panels;
  - a plain background colour;
  - Restore defaults.
  
  Per game: background and title pictures, display name. After HomeNX's ideas; no HomeNX code or
  assets (docs/HOME.md).

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
