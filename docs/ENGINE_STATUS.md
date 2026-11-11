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

## Current implementation status

The supplied package contains:

- The NEXUS application executable and application source archive.
- NEXUS engine integration code and emulation-engine source/patch material.
- Engine-build management UI and verified install/rollback logic.
- A build and test workflow description for producing an engine package.
- Tests and a fake/stand-in engine path for application-side development.

**A compiled nexus_engine.dll is not included in the supplied package.** The package documentation states that the engine could not be compiled in the environment used to prepare it. Therefore, game launching is not ready for normal use in this package.

## What has and has not been verified

The supplied engineering notes report application-side tests for session management, play-time tracking and crash recovery, error handling, volume settings, update/DLC hand-off, engine-build verification and rollback. They also report syntax checks of NEXUS engine C++ sources and tests of the volume-gain calculation.

The supplied notes explicitly state that the following were **not** verified in a real engine build:

- Compiling and loading the final engine DLL on a Windows gaming PC.
- Launching and playing a real game.
- Real GPU rendering and window ownership/focus behaviour.
- Real audio output and live volume changes.
- Controller auto-mapping during an actual game session.
- Compatibility across commercial games.

These are release-blocking validation tasks, not completed features.

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
