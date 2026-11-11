# The NEXUS engine: investigation, architecture, status and plan

## 0. Changes in 2.1

* **Packaging:** the engine ships inside the NEXUS folder (`Engine\nexus_engine.dll`, `Engine\engine.json`, `Engine\Libraries\`).
  * NEXUS finds it relative to `NEXUS.exe` and loads it with `LoadLibraryExW`. Dependencies are searched only in the engine folder, `Engine\Libraries` and System32.
  * Every file is verified against `engine.json`.
  * `tools/nexus/package_engine.py` (engine repo) produces this layout, including the app-local VC++ runtime and licences.
  * `tools/release/make_release.py` and `.github/workflows/release.yml` (NEXUS repo) build both projects at the commits in `engine.lock` and produce one `NEXUS-<version>-Windows-x64.zip`.
* **Engine interface 2:** adds `nexus_engine_suspend(int)`.
  * It pauses emulation and hides the game window for NEXUS Home; status reports `"suspended"`.
  * It is handled on the engine thread inside the existing event loop.
  * The session stays loaded, and suspended time is not play time.
* **Saves:** the engine still uses its title-ID paths. NEXUS moves each user's readable game folders there for the duration of a session and back afterwards (see README, Saves).
* **Not compiled here**, as before: interface 2 and the packaging changes were syntax-checked against the real headers, and the host side was tested with the stand-in engine.


This document covers NEXUS 2.0. It says what is built, what has been tested and what is still
to do. Anything that has not been tested is labelled as untested.

## 1. What changed in NEXUS 2.0

NEXUS 1.x was a frontend. Pressing **Play** started a separate emulator program
(`lichii.exe` / `eden-cli.exe`, or `Ryujinx.exe`) and NEXUS watched its window.

NEXUS 2.0 runs games itself:

```
NEXUS.exe ── loads ──► nexus_engine.dll   (in the NEXUS process, Engine\builds\<version>\)
   │                     │
   │ Play: writes the     ├─ engine thread: Core::System  (CPU: dynarmic JIT, HLE services,
   │ session config and   │                 kernel, loader, file system, NAND)
   │ calls                ├─ GPU: video_core (Vulkan / OpenGL renderers, shader caches)
   │ nexus_engine_start   ├─ audio: audio_core → SDL/cubeb sink, NEXUS 0–200 % gain + limiter
   │                      ├─ input: input_common (SDL3 controllers, keyboard, mouse)
   │ polls status ◄───────┤  game window: SDL3 window owned by the NEXUS window
   │ (frames, state,      └─ game ends / Close game → shutdown → status "stopped"
   │  errors, system)
   └─ back to the home screen; play time, compatibility and saves are recorded
```

* **No external emulator program is started.** Nothing is started hidden, renamed or in the
  background. A test fails the build if any NEXUS Go file names `lichii.exe`, `Ryujinx.exe`,
  `eden-cli.exe` or `yuzu-cmd`, or calls `exec.Command` for anything except opening Explorer
  or the WebView2 fallback browser (`TestNoEmulatorProgramsRemain`).
* The engine is a C ABI library (`nexus_engine.h`, interface version 1, JSON in and out).
  NEXUS loads it with `LoadLibrary` and does not need cgo:
  `api_version`, `info`, `start`, `status`, `stop`, `set_volume` and `focus`.
* **Session lifecycle.** Each launch goes through these steps:
  1. NEXUS writes `Engine\config\session.ini`. It holds the global NEXUS settings plus the
     game's own settings, the user's save folder and the controller profile.
  2. The engine loads the game.
  3. The engine registers update and DLC files found in the library.
  4. If no profile is chosen, the engine auto-maps player 1 to the first controller.
  5. The engine starts the GPU and renders into an SDL window. That window is owned by the
     NEXUS window (`GWLP_HWNDPARENT`), so Windows treats it as part of NEXUS.
  6. The engine reports its state: frames presented, program ID, Switch 1 or Switch 2
     program, and errors.
  7. When the game exits, or the user chooses *Close game*, the engine shuts the session down
     and NEXUS returns to the home screen.
* **Play time** counts only while the engine is running and presenting frames. The rule is
  the same as in 1.x: checkpoints every 30 s, with crash recovery.
* **Engine data** lives in `%APPDATA%\NEXUS\Engine`, never in the program folder. That
  covers keys, firmware, NAND, shader caches, configuration and logs.
  * On the first start, keys, firmware, installed updates/DLC, mods and controller profiles
    are brought in from the emulator data folder NEXUS 1.x used (`%APPDATA%\eden`). They are
    hard-linked or copied; nothing is moved or deleted.
  * Saves are copied only when the user asks (*User settings › Copy earlier saves*).
* **Engine builds** live in `Engine\builds\<version>\`. Each build is a zip containing
  `nexus_engine.dll`, its DLLs, and `engine.json` (version, interface, source commit,
  upstream base, and the SHA-256 of every file).
  * Install is verify-before-use: a damaged or wrong-interface build is refused and nothing
    changes.
  * Builds are never overwritten in place. The previous build is kept for **rollback**
    (*Settings › NEXUS Engine*).

## 2. Upstream projects investigated

| Project | Language / licence | What it is | How NEXUS uses it |
|---|---|---|---|
| **Eden** (git.eden-emu.dev, mirror github.com/eden-emulator/mirror) | C++20, GPL-3.0-or-later | Most active yuzu successor. Core, dynarmic, video_core, audio_core, input_common, SDL frontend | **Upstream of the engine core.** Added as a git remote of the engine fork. The official host was not reachable from the build environment; the GitHub mirror was. |
| **Lichii** (github.com/zortos293/Lichii) | C++20, GPL-3.0-or-later | Eden fork with extra fixes, PGO builds and experimental Switch 2 ("nx2") work | **Base of the NEXUS engine fork** (build-130). build-150 is merged on a sync branch (§4). |
| **Ryujinx / Ryubing** (git.ryujinx.app) | C# / .NET, MIT | Independent emulator with its own HLE, ARMeilleure JIT, own GPU and audio code | **Not linkable into a C++ engine.** It is a different language and runtime, with a different architecture throughout. Using it inside NEXUS would mean hosting the .NET runtime, which still makes it a second emulator, not one NEXUS engine. It is a **reference only**: individual fixes can be ported by hand into the C++ core with attribution (MIT permits this; its notice is kept in `Licences`). No Ryujinx code is in NEXUS 2.0. |

**Overlap.** Eden and Lichii share the yuzu code base. Lichii is Eden plus its own patches,
so one fork of Lichii that tracks both remotes gets the work of both. Ryujinx overlaps in
purpose only; it shares no code.

**Switch 1 vs Switch 2.**

* Switch 1 support is the mature yuzu/Eden core.
* Lichii's Switch 2 work (`NX2_NOTES.md` in the engine source) adds:
  * ARMv8.1–8.5 instructions in dynarmic;
  * the Switch 2 NPDM ACID layout;
  * many service stubs;
  * NVN2/syncpoint/vi2 sessions;
  * VP9 decoding.
* According to those notes, Switch 2 titles need an auto-stub mode and loose ExeFS dumps, and
  one title reaches its intro, another its title screen at 10–17 fps.
* That is upstream's claim. **NEXUS has not run any Switch 2 title, so NEXUS 2.0 makes no
  Switch 2 claim.**
  * The engine reports `"switch2": "experimental"`.
  * The settings page says it is untested.
  * Each game's real result is recorded by the compatibility system (§5).

## 3. What is implemented and what was tested in this build environment

The build machine for this work had 2 CPU cores, 7 GB RAM, no GPU and no system development
libraries, and could only reach package registries and GitHub. **Compiling the full engine
(≈ 1 M lines of C++ plus Boost, Qt-free SDL3, Vulkan and FFmpeg dependencies) was not possible
here, and no game was run.**

**Done and verified here**

* **Engine fork** (`nexus` branch = Lichii build-130 + NEXUS commits):
  * engine library (`src/nexus_engine`) and C ABI;
  * explicit data root;
  * window title and icon;
  * session config through the upstream `SdlConfig`;
  * updates/DLC registration;
  * controller auto-map;
  * Switch 1/2 program detection;
  * the 0–200 % volume path.

  Every NEXUS source file was syntax-checked with `g++ -std=c++20 -fsyntax-only` against the
  real dependency headers (Boost 1.92, SDL3 3.4.8, fmt 12.1, nlohmann 3.12, Vulkan-Headers
  1.4.345), with and without OpenGL: 0 errors.
* **Volume.**
  * The engine gain curve (`nexus_gain.h`) has Catch2 tests in the engine (`tests/nexus`).
  * Run here through a shim: 46 062 checks passed. They cover:
    * 0 % = silent; 50 % halves the level; 100 % is unchanged;
    * linear above 100 %;
    * the upstream bug fixed (upstream turned 101 % into 0.6×);
    * soft limiting never exceeds full scale and stays monotonic.
* **NEXUS host side.** Go tests run against a stand-in engine that follows the same state
  machine and pushes a test tone through the same gain curve. 30 test functions pass (all of them),
  including:
  * session config contents (global + per-game + per-user saves);
  * play time and checkpoints, crash recovery;
  * load errors, stop;
  * live volume changing the audio peak, clamping, persistence;
  * update/DLC files passed to the engine;
  * engine build verify/install/rollback (damaged and wrong-interface builds refused, path
    traversal refused);
  * the "no external emulator" rule.
* **UI.**
  * Walked through in Chrome with zero script errors.
  * No "Lichii" or "Ryujinx" text on any screen visited: home, every settings page, game
    settings, playing screen, game info, controllers.
  * Volume set by dragging and by D-pad/arrow keys reached the engine live and was stored.
* **Startup font.** Covered in its own section below.

**Not done here and still to do**

* Building `nexus_engine.dll`. The CI workflow `.github/workflows/nexus-engine.yml` does it:
  * Windows, MSVC;
  * build the engine and the engine tests;
  * run the tests as a gate;
  * run the ABI smoke test against the built DLL;
  * package with the SHA-256 manifest;
  * publish a release only on an `engine-v*` tag after all of that passes.

  It has **not been run yet**. The workflow and fork are ready; they need to be pushed to a
  GitHub repository.
* Running games: not done (needs the DLL and a GPU).
* Runtime checks that need a real build:
  * the engine window being owned by and focused with NEXUS;
  * multi-session reuse of one `Core::System` (the same pattern as the upstream Qt frontend,
    but untested in this library);
  * controller auto-map timing;
  * audio volume on real output.

  These are the first things to check on the first CI build.
* Until a build is installed, **Play shows "The NEXUS engine is not installed" and starts
  nothing.**

## 4. Upstream update workflow (never "download the newest exe")

`tools/nexus/upstream-sync.sh` in the engine fork:

1. **Remotes.** `lichii` (zortos293/Lichii) and `eden` (eden-emulator/mirror), plus
   `ryujinx` as a read-only reference.
2. **`report <ref>`** lists the new upstream commits and the files that also carry NEXUS
   changes, which are the likely conflicts.
3. **`merge <ref>`** creates `sync/<ref>` and merges. Conflicts are resolved there, never on
   `nexus`.
4. **Build and test in CI.** On push, CI builds and tests the sync branch. Only a passing
   build is merged into `nexus`, then tagged `engine-vX.Y.Z`, which publishes the zip and its
   SHA-256.
5. **Install in NEXUS** with *Settings › NEXUS Engine › Install*. NEXUS verifies the
   manifest and every file. The old build stays.
6. **Rollback.** *Go back to the previous build*, or activate any kept build.

**Done here.**

* Lichii **build-150** (51 commits after build-130) is merged in branch `sync/build-150`
  (merge commit `e1f51d963`).
* The only conflict was in `src/common/fs/path_util.cpp`: upstream added a `portable` flag
  where NEXUS added an explicit data root. It was resolved by keeping both.
* NEXUS's later engine commits are cherry-picked onto the sync branch.
* It is **not merged into `nexus`** because it has not passed CI. The default engine stays
  build-130-based until it does.

## 5. Compatibility system

Every session records:

* whether the game reached its first frame;
* whether it ended with an engine error (and the error code);
* how long it ran;
* whether the program is a Switch 1 or Switch 2 program, from its NPDM as reported by the
  engine.

Ratings:

| Rating | Meaning |
|---|---|
| **Works** | ≥ 10 min over ≥ 2 sessions without a crash, or the user says so |
| **Known issues** | Ran but crashed at least once, or the user says so |
| **Unsupported** | Never reached the first frame, or the user says so |
| **Starts** | Reached gameplay, not played long enough to judge |
| **Not tested** | Never started in NEXUS |

The user's rating always wins and is shown as theirs. No upstream claims are imported.

## 6. Switch 1 / Switch 2 investigation by area

| Area | Engine source (Lichii/Eden core) | Status in NEXUS 2.0 |
|---|---|---|
| Loading | NSP/XCI/NSZ/XCZ/NCA/NRO/NSO/NUP; updates/DLC via the content provider; Switch 2 NPDM ACID parsing (nx2) | Switch 1 path integrated; updates/DLC from library folders passed in. Switch 2 detection reported; Switch 2 loading untested |
| CPU | dynarmic ARMv8 JIT; nx2 adds ARMv8.1–8.5 | Upstream, unchanged |
| GPU | Maxwell/Pascal command processing, Vulkan + OpenGL; nx2 adds NVN2 bits | Upstream; NEXUS selects renderer/settings |
| Shaders | Disk shader cache + async building | Loaded per program ID at session start |
| Audio | audio_core → SDL/cubeb | NEXUS gain 0–200 % with soft limiter (tested); live from the UI |
| Input | input_common, SDL3 | Saved profiles per user/game, or controller auto-map |
| Saves | NAND save data, per-user save directory | Per-NEXUS-user folders (as 1.x); earlier saves imported on request |
| Services | HLE services; nx2 adds many stubs | Upstream |
| Firmware/keys | prod.keys/title.keys; firmware NCAs in NAND | Imported, validated (header key, firmware decryption, version), installed into the engine data folder |
| Performance | upstream | See performance notes |

## 7. Staged plan from here

1. **CI build of the engine (first).** Push the fork to a GitHub repository and let the
   workflow build it. Fix what MSVC reports, since only GCC syntax checks were possible
   here. Get tests and the ABI smoke test green, then tag `engine-v0.1.0`.
2. **First runtime verification on a PC with a GPU.**
   * Homebrew NRO first (no keys needed), then one Switch 1 commercial title.
   * Check the window ownership/focus, the stop path, the second session in one NEXUS run,
     the volume on real output and the controller auto-map.
   * Record the results in the compatibility system.
3. **Merge `sync/build-150`** once it builds and passes. Repeat the sync workflow for every
   upstream release.
4. **In-NEXUS controller mapping editor** writing engine input profiles. Today saved profiles
   or the auto-map are used; there is no in-game remap UI, and NEXUS does not show one that
   does nothing.
5. **Switch 2.** Only after Switch 1 is solid.
   * Expose the nx2 auto-stub as an explicit, labelled experimental option.
   * Test with dumps the user owns.
   * Report per game through compatibility. No general claim.
6. **Selective Ryujinx-derived fixes**, ported by hand into the C++ core with MIT attribution,
   when a specific game needs them.

## 8. Licensing

* The engine is GPL-3.0-or-later (yuzu/Eden/Lichii lineage). NEXUS loads it in-process, so
  NEXUS plus the engine is one combined work. **NEXUS 2.0 is therefore distributed under
  GPL-3.0-or-later**, and its full source is included (`Source\NEXUS-source.zip`).
* **Engine corresponding source:**
  * `Source\engine\lichii-build-130-source.tar.xz` (the upstream base);
  * `Source\engine\nexus-engine-patches\` (every NEXUS commit as `git format-patch`), which
    applied to the base gives exactly the `nexus` branch.

  Dependencies are fetched by the engine's build system as documented in its `docs/Build.md`.
* Upstream licence texts and component licences are kept under `Licences\`.
* **Ryujinx (MIT)** is a reference only; its notice is kept for any future ported fix.
* **Fonts:** Inter (OFL-1.1, interface) and **Outfit (OFL-1.1, startup wordmark)**.
* **No firmware, keys or games are included or downloaded by NEXUS.**

## Startup font

The startup wordmark was **text baked into images**:

* `web/assets/brand/word-nexus.png` and `word-switch.png`;
* the combined `logo-full.png`, which is used for the shine mask and the settings preview.

All three were cut from the user's original artwork, so no font file was involved.

* **Change.** The two words are re-rendered with **Outfit**:
  * licence: SIL Open Font License 1.1, from the google/fonts repository;
  * weights: SemiBold 600 for the spaced "N E X U S", ExtraBold 800 for "SWITCH";
  * same canvases (482×76, 680×126, 680×466), same boxes and margins;
  * the emblem rows of `logo-full.png` are byte-for-byte unchanged;
  * same animation, timings, layout and colours. The images are masks, so the startup colour
    still tints them.

  Generator: `tools/brand/render_wordmark.py`.
* **Fallback.** If a wordmark image cannot load, the startup screen draws the word as live
  text in the same font (`NEXUS Display` = bundled Outfit), then Segoe UI Variable Display or
  the system UI font.
* **Verified:**
  * the startup screen was rendered in Chrome and screenshotted at the end of the animation;
  * the word masks now changed 25 % / 33 % of their pixels;
  * the fallback path was screenshotted with the images blocked.
