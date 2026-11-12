# Engineering research: what other Switch emulators do, and what NEXUS does about it

This is an ongoing log. NEXUS studies Ryujinx, Ryubing, yuzu, Eden and Lichii as technical
references. It compares what it learns with NEXUS's own implementation, and writes its own code
where NEXUS is missing something that matters.

**Rule:** no source files, functions or patches from these projects are copied into NEXUS's own
code (`app/`, `capture/`, `android/`). Changes are designed for NEXUS's architecture and
written in NEXUS's own modules.

**The NEXUS Engine is different.** It is still the upstream engine plus NEXUS patches, as
`docs/SOURCE_AUDIT.md` explains. This cycle merged no new upstream engine code.

## The projects and their state (November 2026)

| Project | Language and lineage | State | Most relevant to NEXUS |
|---|---|---|---|
| **yuzu** | C++ | Discontinued (2024). It is the base of the Eden and Lichii lineage, so of the NEXUS Engine. | Its engineering write-ups (progress reports) explain many problems the engine still has to handle. |
| **Ryujinx** | C# (.NET) | Discontinued (October 2024). MIT licence. | A different architecture: high-level emulation of services, and a persistent translation cache for its CPU JIT. |
| **Ryubing** | C# fork of Ryujinx | Continues Ryujinx. It describes itself as a quality-of-life uplift for existing Ryujinx users. Recent work moved to .NET 10 and SDL3, with Vulkan package updates and input fixes. | Input and controller handling; its UI conventions. |
| **Eden** | C++ fork in the yuzu line | The most active desktop and Android fork. 2026 releases improved JIT caching, Mali rendering and Qualcomm driver compatibility, and added updates and DLC without installing to NAND, mod import, and a frame-pacing option. | Android/ARM64 graphics drivers, and CPU JIT caching. |
| **Lichii** | C++ fork in the Eden/Eglantine line | Switch 2 focus. Fast-moving builds, with game fixes such as Ocarina of Time and Dynamic FPS. **It is the NEXUS Engine's upstream base.** | Switch 2 games; upstream fixes that NEXUS rebases onto. |

## Areas examined, findings and decisions

### 1. Graphics card selection on PCs with two GPUs

This is performance priority 1. **Implemented in 0.5.0.**

- **Problem.**
  - Gaming laptops have an integrated GPU and a dedicated one.
  - The engine renders on the Vulkan device at index 0 unless told otherwise (`vulkan_device`).
  - The Vulkan loader often lists the integrated GPU first. Public reports from other
    Vulkan applications show index 0 being the Intel GPU on Optimus laptops, with the NVIDIA GPU
    at a later index.
- **How the yuzu lineage avoids it.**
  - Its programs (`yuzu.exe`, `yuzu-cmd.exe`) export `NvOptimusEnablement` and
    `AmdPowerXpressRequestHighPerformance`.
  - NVIDIA documents that the Optimus driver looks for this export in the application when it starts.
- **What NEXUS was missing.**
  - The engine is a DLL loaded into `NEXUS.exe`, so the engine's own copies of those exports
    have no effect.
  - `NEXUS.exe` (Go) does not export them.
  - NEXUS also never set `vulkan_device`, so on such laptops games could run on the integrated GPU.
- **NEXUS's own implementation** (`gpu_select.go`, `vulkan_windows.go`, `vulkan_parse.go`).
  - NEXUS lists the Vulkan devices in its own process. This is the same loader and the same order
    the engine sees.
  - It chooses one by preference:
    - the default is the dedicated card, then the one with the most device-local memory;
    - software rasterisers are never chosen;
    - the option *Integrated graphics (longer battery life)* picks the integrated GPU instead.
  - It writes the device's index into the session configuration, and logs the choice.
  - It is per game and global, in *Emulation settings › Graphics card*.
  - This is stronger than the export approach: it does not depend on a vendor driver honouring an
    export, and the user can choose.
- **Tests.** Selection rules (`TestChooseVulkanDevice`), structure parsing
  (`TestParseVulkanDevice`), and the index written into the configuration
  (`TestSessionConfigChoosesGPU`).
- **Limitation.** Not yet verified on a real two-GPU laptop. The Windows code compiles, and
  the logic is tested.

### 2. Faulty implicit Vulkan layers

Stability. **Implemented in 0.5.0.**

- **Problem.**
  - Overlays and tools install implicit Vulkan layers that load into every Vulkan program.
  - yuzu's developers traced crashes to an outdated Overwolf layer and advised removing it.
- **NEXUS** (`vulkanlayers.go`, `detectVulkanLayers` in `hardware_windows.go`).
  - The hardware report lists the enabled implicit layers, as the loader reads them from the registry.
  - It warns about known problem layers in the hardware notes.
  - The list of known layers is small and evidence-based. Layers are not disabled, because they
    belong to other programs.

### 3. Timer resolution and power throttling

Frame pacing. **Checked; nothing to add.**

- The yuzu lineage raises the Windows timer resolution and opts out of power throttling for the
  emulator process.
- The NEXUS engine host already does this when a session starts (`nexus_engine.cpp` calls
  `SetCurrentTimerResolutionToMaximum`).

### 4. Shader and pipeline caching

- **Upstream state.**
  - The engine keeps a per-game disk pipeline cache. NEXUS exposes it as *Disk shader cache*, with *Asynchronous shader building*.
  - Ryujinx keeps a versioned shader cache, plus a persistent cache of JIT-translated CPU code (PPTC) that shortens later game starts.
  - Eden's 2026 releases improved JIT caching.
- **Decision: deferred.**
  - A persistent CPU translation cache for the engine's JIT is deep engine work.
  - It is the most promising remaining start-up improvement. NEXUS will design its own once the
    engine's JIT boundaries are documented (see `docs/SOURCE_AUDIT.md`, stage 3).

### 5. Save data compatibility

**Implemented, in NEXUS's own code.**

- Other programs store saves either as the console's folder layout (player and game IDs), or
  with a metadata file (`ExtraData0`, as in Ryujinx-style saves).
- NEXUS's importer recognises both layouts by structure, not by program. It now does this from
  any ZIP, as described in *Import Save from ZIP*.

### 6. Android and ARM64

- **Findings.**
  - Eden's 2026 releases focused on stock Qualcomm drivers and Turnip, Snapdragon 8 Elite
    performance, and Mali rendering performance.
  - Native-execution projects need per-game prepared packs.
- **Decisions.**
  - NEXUS's readiness check ranks GPU families accordingly: Adreno 730+ recommended; Mali and Immortalis supported, with a warning.
  - NEXUS's Android path is its own engine host and app layer (`docs/ANDROID.md`).

### 7. Updates, DLC and mods without installing

**NEXUS already has this.** NEXUS 0.3.0 added per-game updates and mods that are checked by
content and need no NAND installation. Eden 0.2.0 added a similar feature.

## Rejected or deferred

- **Copying upstream fixes into NEXUS code.** Not done, by policy. Engine fixes come through the
  upstream rebase, and are documented there.
- **Disabling other programs' Vulkan layers automatically.** It could break those programs. NEXUS
  only reports them.
- **A NEXUS-wide frame-pacing override.** Engine settings already cover VSync, the frame-rate
  limit and display matching. No measurable benefit was shown.
- **SDL3 migration (as in Ryubing).** It belongs to the engine. Upstream decides it.
