# NEXUS Engine: architecture and development status

## Overview

NEXUS is designed to load its emulation engine in-process as a native library. The intended launch flow is:

1. The user selects a game in NEXUS.
2. NEXUS checks that a compatible engine build and required system files are installed.
3. NEXUS prepares a session configuration containing global settings, per-game settings, the user's save location and the selected controller profile.
4. NEXUS loads the engine library and starts the game session.
5. The engine reports session state, frames presented, program information and errors to NEXUS.
6. When the game exits, NEXUS returns to its home screen and records play statistics and compatibility information.

The design does not call for launching a separate emulator executable. The engine is a native library loaded by NEXUS.

## Upstream projects and foundations

NEXUS's emulation development draws on **Lichii, Eden, yuzu and Ryujinx**, including related forks and components such as **Ryubing**. These projects form part of the wider upstream foundation and development work. Their original copyright notices and licences must be preserved for the relevant code and components.

See [Third-party notices, licences and acknowledgements](../THIRD_PARTY_NOTICES.md) for the project acknowledgements and licence summary.

## Current implementation status (NEXUS 2.2)

**The engine is compiled, tested and shipped in the NEXUS ZIP.** The repository's Windows build
(`.github/workflows/nexus-build.yml`) checks out upstream Lichii build-130, applies the NEXUS patch
series in `engine/patches`, and builds `nexus_engine.dll` with clang-cl. Every build then:

- runs the engine's test suite (1,156 test cases, all passing);
- calls the DLL's C interface (interface version 2, build info, status, volume, suspend, bad-request handling);
- runs NEXUS's own test suite on Windows and Linux;
- builds `NEXUS.exe` and packs it with the engine (and the complete app-local VC++ runtime) into `NEXUS-2.2.0-Windows-x64.zip`;
- extracts that ZIP into a fresh folder, checks that every DLL the engine imports is in the package or part of Windows, and runs `NEXUS.exe --engine-check`, which finds, verifies and loads the engine exactly as on a user's PC (result: ready, NEXUS Engine 2.2.0, Switch core + Switch 2 experimental, Vulkan/OpenGL renderers).

## What has and has not been verified

Verified by the build: compiling, linking and packaging the engine; the engine's unit tests;
loading the engine DLL in NEXUS from an extracted ZIP; NEXUS's application tests (sessions,
play time, crash recovery, saves, settings reaching the engine, controller layouts, user limit).

**Not yet verified** (the build machine has no GPU and no games):

- Launching and playing a real game.
- Real GPU rendering and window ownership/focus behaviour.
- Real audio output and live volume changes.
- Controller layouts and presets inside an actual game session.
- Compatibility across commercial games.

These are the first checks to do on a gaming PC with your own keys, firmware and games.

## Switch 2 status

The upstream source contains experimental Switch 2-related work. The supplied NEXUS notes state that NEXUS itself has not run a Switch 2 game. Switch 2 support must therefore be described as **experimental and unverified**. Do not advertise general Switch 2 compatibility without repeatable testing and per-game evidence.

## Engine build management

The intended engine package is a versioned archive containing the engine library, required runtime libraries and a manifest. The manifest records the engine version, source commit and file hashes. NEXUS verifies the package before installation and keeps earlier builds available for rollback.

Engine build identifiers are independent from NEXUS application version numbers. See [VERSIONING.md](../VERSIONING.md).

## Release checklist

Before describing NEXUS game emulation as ready for release:

- [ ] Build the engine library using the documented toolchain for the target platform.
- [ ] Run the engine's automated tests and the NEXUS integration/smoke tests.
- [ ] Verify the engine manifest and every packaged file hash.
- [ ] Test a homebrew title first, then legally obtained test titles.
- [ ] Verify launch, focus, return-to-home and repeated sessions.
- [ ] Verify audio output, volume control and controller input.
- [ ] Test missing/invalid keys and firmware handling.
- [ ] Record per-game results and clearly label untested systems.
- [ ] Publish the complete corresponding source and all required third-party notices with the binary.
