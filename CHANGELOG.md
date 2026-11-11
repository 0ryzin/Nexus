# Changelog

All notable NEXUS changes will be recorded here.

The project is in preview. Releases are numbered from **0.0.0**; the first intended stable release is **1.0.0**. Every release, with its notes and download, is on the [Releases page](https://github.com/0ryzin/Nexus/releases).

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
