# NEXUS source audit

Where every part of NEXUS comes from, as of NEXUS 0.4.0 (engine 0.5.0, upstream base Lichii
build-160). This is the internal record the project uses to keep its licence obligations and its
claims accurate. It is updated with every release that changes the picture.

**Summary.** The NEXUS *application* and the new *capture module* are original NEXUS code. The NEXUS
*engine's emulation core* (CPU, GPU, shaders, audio, input, Horizon OS services, file system,
loaders) is **not** independent: it is the yuzu → Eden → Lichii code base, built with a set of
NEXUS patches. NEXUS must not be described as an independently implemented emulator until the
components in section 4 have actually been replaced.

Line counts are physical lines of C/C++ (engine) or Go/JS/CSS/HTML (application), counted from the
repositories at this release; they show proportions, not effort.

## 1. Original NEXUS code (written for NEXUS)

| Component | Where | Size | Licence |
|---|---|---|---|
| NEXUS application: window, local API, library and game metadata readers (NCA/NSP/XCI/NRO/NUP parsers written in Go from public format documentation), user profiles, Save Data (layout, migration, import/export, backups), Media library, controller memory, game updates and mods, compatibility records, settings and per-game configuration, engine session host, diagnostics, performance overlay, updater, DLSS download manager, device linking | `app/*.go` | ~20 200 lines Go (+ ~3 900 lines of tests) | GPL-3.0-or-later |
| NEXUS interface (home, library, settings, setup, launch flow, Media, Save Data …) | `app/web` | ~4 500 lines JS/CSS/HTML | GPL-3.0-or-later |
| NEXUS capture module: window capture, GPU colour conversion/scaling, Media Foundation encoding, game-audio and microphone capture, fragmented MP4 (new in 0.4.0) | `capture/` | see `capture/` | GPL-3.0-or-later |
| NEXUS engine host layer: the in-process engine library and its C interface (`nexus_engine.dll`: session lifecycle, status, Home/shortcuts, controller mapping, save-folder table, screenshots, reveal, background running) | `engine/patches` → `src/nexus_engine` | ~1 300 lines C++ | GPL-3.0-or-later (it links the upstream core) |
| NEXUS changes inside upstream files (save folders, gain stage, host-managed data folder, hidden-until-first-frame window …) | `engine/patches` | ~450 lines across 15 upstream files | GPL-3.0-or-later, as the files they change |
| Build, packaging, verification and release tooling | `.github/workflows`, `app/tools`, `engine/patches` (tools/nexus) | — | GPL-3.0-or-later |

## 2. Derived from upstream emulator projects (not independent)

All of this is the upstream code base the NEXUS engine is built from, unchanged except for the NEXUS
patches listed above. Copyright notices in these files are kept as they are.

| Area | Upstream path (build-160) | Lines | Origin |
|---|---|---|---|
| Horizon OS services, kernel, loaders, file system, memory, core timing | `src/core` | ~295 000 | yuzu (Citra/yuzu team) → Eden → Lichii (incl. Switch 2 work) |
| GPU emulation (Maxwell/Ampere command processing), Vulkan and OpenGL renderers, texture and buffer caches, upscalers/frame generation integration | `src/video_core` | ~162 000 | yuzu → Eden → Lichii |
| Shader recompiler (guest shaders → SPIR-V/GLSL) | `src/shader_recompiler` | ~57 000 | yuzu → Eden → Lichii |
| CPU emulation: dynarmic ARM JIT | `src/dynarmic` | ~128 000 | dynarmic (merryhime et al.), vendored by Eden |
| Audio core and output sinks | `src/audio_core` | ~33 000 | yuzu → Eden |
| Input: HID core, input drivers (SDL, Joy-Con, GC adapter …) | `src/hid_core`, `src/input_common` | ~37 000 | yuzu → Eden |
| Common utilities, settings, file-system helpers | `src/common` | ~30 000 | yuzu → Eden |
| Configuration reader, SDL window | `src/frontend_common`, `src/yuzu_cmd` (window classes only) | ~5 500 | yuzu → Eden → Lichii |
| Network | `src/network` | ~3 000 | yuzu → Eden |
| Engine test suite | `src/tests` | ~50 000 | upstream (+ 3 NEXUS test files) |

Not built into NEXUS: the Qt interface (`src/yuzu`, ~46 000), the Android app (`src/android`,
~42 000), the room server and web service.

**Licence:** GPL-3.0-or-later (some yuzu components GPL-2.0-or-later, compatible). NEXUS as a whole
is distributed under GPL-3.0-or-later; the source of every release is public (repository +
`engine/patches` + the recorded upstream commit).

## 3. Third-party libraries (required dependencies)

Fetched by the engine build (`cpmfile.json`) or vendored in `externals/`; each keeps its own licence
(texts in the release's `Licences` folder): Boost, fmt, SDL3, cubeb, Opus, FFmpeg (decoders), zlib,
zstd, lz4, xxhash, nlohmann/json, SimpleIni, OpenSSL, enet, httplib, cpp-jwt, frozen, xbyak
(x86 code generation for dynarmic), oaknut (ARM64), biscuit, sirit (SPIR-V emitter),
SPIRV-Headers, Vulkan-Headers, Vulkan Memory Allocator, Vulkan Utility Libraries, glad, stb,
bc_decoder, nxformat, FidelityFX SDK / FSR (AMD), NVIDIA Streamline headers (MIT; NVIDIA's runtime
files are *not* shipped — see THIRD_PARTY_NOTICES), libusb, tzdb data. The NEXUS application uses
only the Go standard library, Microsoft WebView2 (system component) and Windows APIs. The capture
module uses only Windows APIs (Windows Graphics Capture, Direct3D 11, Media Foundation, WASAPI).

## 4. What could become independent NEXUS code, and in what order

Staged, each step replacing an upstream part only after the NEXUS implementation matches it in tests
and in real games. The emulation core is by far the largest part; replacing it is a multi-year
effort, so the plan starts at the edges and works inward.

| Stage | Component | Why this order | Status |
|---|---|---|---|
| 0 | Application, game metadata, saves, media, configuration, updates, diagnostics | Already NEXUS | Done |
| 1 | Engine host/session layer (`nexus_engine`) | Owns lifecycle, errors, shortcuts | Done (NEXUS code; links the core) |
| 1 | Capture and recording | New feature, no upstream equivalent used | Done in 0.4.0 |
| 2 | Window/presentation layer (replace `yuzu_cmd` SDL window classes with a NEXUS window and swapchain owner) | Small, well-bounded, platform-specific — also the first piece any other platform would need | Planned |
| 2 | Configuration path: NEXUS writes the engine settings structure directly instead of an INI the upstream reader parses | Removes a translation layer; NEXUS already owns every value | Planned |
| 3 | Input layer: a NEXUS controller backend (device identity, mapping, Home/capture handling) feeding the HID core | NEXUS already owns controller identity and layouts | Planned |
| 3 | Audio output sink (NEXUS backend per platform) | Bounded interface (`Sink`) | Planned |
| 4 | Shader/pipeline cache management (NEXUS cache format, versioning, preparation for other devices) | Needed for Android preparation | Research |
| 5 | Larger core parts (services, GPU, shader recompiler, CPU JIT) | Very large; only with a dedicated team and test corpus | Not planned yet |

Rules for every stage: write from public documentation and observed behaviour, not by copying or
renaming upstream code; keep the upstream implementation available behind a switch until the NEXUS
one is proven; compare both with the same tests and games; record the change here.

## 5. External programs

NEXUS does not start, need or look for any other emulator program, installation or interface. The
engine is part of the NEXUS download (`NEXUS\Engine`), verified at every start
(`NEXUS.exe --engine-check`). Save import can *read* another emulator's save folder when the user
chooses one (never changing it).
