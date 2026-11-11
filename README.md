![NEXUS banner](assets/NEXUS_Banner_Logo.jpg)

# NEXUS

**A console-style game library and integrated emulation environment for Windows.**

NEXUS is an independent, console-inspired gaming environment being developed for Windows. It brings your game library, user profiles, play statistics, controller navigation, themes and emulation settings together in one interface.

NEXUS is being developed around an integrated emulation engine. The intended experience is to launch and manage games from NEXUS itself, rather than asking users to choose and launch a separate emulator application.

> **Development status:** NEXUS is still experimental. The supplied development package does not include a compiled NEXUS Engine build, so game launching is not yet ready for normal use. The engine source and integration are present in the supplied source package, but the engine must be built and tested before games can run. See [Engine status](docs/ENGINE_STATUS.md).

## Project goals

- A console-like experience with a startup sequence and controller-friendly interface.
- One library for games, with search, sorting, artwork and per-game settings.
- Multiple user profiles, separate saves, play-time statistics and compatibility notes.
- Customisable themes, colours, wallpapers and startup options.
- An integrated engine that NEXUS controls directly, with engine builds verified before installation and the ability to roll back.
- A portable application folder, with user data stored separately by default where supported.

## Current features

The current Windows development build supplied for review includes:

- A startup animation, home screen and game library.
- User profiles, profile icons and per-user play statistics.
- Controller, keyboard and mouse navigation.
- Game artwork management, library search and sorting.
- Themes, wallpapers and appearance settings.
- Per-game settings and compatibility tracking.
- Engine management screens, including verified engine-build installation and rollback support.
- Import paths for relevant data from earlier NEXUS installations and the Eden data directory.

Features in the interface do not necessarily mean the full emulation path has been verified. In particular, real-game launching has not yet been validated with a compiled engine build.

## Emulation engine

The NEXUS emulation project draws on the work and codebases of **Lichii, Eden, yuzu and Ryujinx**, including related forks and components such as Ryubing. These projects have contributed to the wider foundation and development of NEXUS's emulation capabilities. Their original names, licences and notices remain acknowledged; see [Third-party notices and acknowledgements](THIRD_PARTY_NOTICES.md).

NEXUS is designed to load its emulation engine directly. It is not intended to launch a separate Lichii, Ryujinx or other emulator application when a game is selected.

**Current limitation:** no compiled nexus_engine.dll is included in the supplied package. Until a compatible engine build is produced and installed, selecting Play will report that the engine is unavailable and will not start a game.

Switch 2 support is experimental upstream work and has **not** been verified by NEXUS. No Switch 2 compatibility claim is made.

More detail: [Engine architecture and status](docs/ENGINE_STATUS.md).

## Requirements

The current Windows development package is intended for:

- Windows 10 or Windows 11, 64-bit.
- Microsoft Edge WebView2 Runtime.
- Microsoft Visual C++ 2015–2022 x64 runtime for the engine.
- A graphics driver supporting Vulkan 1.3 (recommended) or OpenGL 4.6.
- Your own legally obtained game dumps, console keys and firmware. These are not included.

NEXUS does not provide games, console keys or firmware.

## Getting started

1. Obtain a NEXUS Windows development or release package when one is published.
2. Extract or install the NEXUS package using the instructions supplied with that build.
3. Run NEXUS.
4. Create a user profile and add your game-library folders in System Settings.
5. Install only system files and game dumps that you are legally entitled to use.
6. Install a compatible NEXUS Engine build when one is available.

The supplied package currently has no compiled engine build, so steps involving game launching cannot yet be completed.

## Data and portability

On the current Windows development build, NEXUS stores user data under:

    %APPDATA%\NEXUS\

This includes profiles, settings, artwork, statistics, logs, saves and engine data. The program and engine-build folders remain alongside NEXUS.exe.

An empty portable.txt beside NEXUS.exe enables portable mode, keeping application data inside the NEXUS folder. Data locations on other platforms will be documented with their respective builds.

## Versioning

The first intended stable NEXUS release is **1.0.0**. The source in the supplied package currently defaults to 1.0.0-dev; that is a development identifier, not a published stable release.

NEXUS application versions and NEXUS Engine build versions are tracked separately. See [VERSIONING.md](VERSIONING.md).

## Repository contents and source availability

This repository is currently being set up for NEXUS project documentation and releases. Its main branch does not yet contain the complete application and engine source trees or a downloadable release package.

The supplied development package contains a NEXUS application source archive and emulation-engine source and integration work. When distributing a compiled build, the corresponding source and all required licence notices must be made available in accordance with the applicable licences. See [LICENSE](LICENSE) and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

## Licensing and acknowledgements

The NEXUS project and its GPL-licensed components are distributed under **GNU GPL version 3 or, at your option, any later version**, subject to the separate licences that apply to third-party components and assets. The full licence is in [LICENSE](LICENSE).

NEXUS draws on Lichii, Eden, yuzu and Ryujinx, including related forks and components such as Ryubing. Other bundled libraries, fonts and engine dependencies have their own licences and attribution requirements. See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for licensing information, notices and acknowledgements.

Some artwork in the supplied personal development package is Nintendo-derived material supplied for personal use. It is not licensed by this repository for redistribution. Replace or remove those assets before sharing a build unless you have the necessary rights.

NEXUS is an independent project and is not affiliated with, endorsed by, or sponsored by Nintendo. Nintendo Switch is a trademark of Nintendo.

## Updates

The current supplied build does **not** include a self-updater. This repository is intended to be the project's official location for future source updates, documentation and releases. Do not assume automatic in-app updates are available until that feature has been implemented and tested.

## Disclaimer

NEXUS is experimental software supplied without warranty. Emulation compatibility, performance and stability vary by game, hardware, drivers and engine build. Use only game files and system files you are legally entitled to use.
