# Lichii research (reference project)

**Project:** Lichii, https://github.com/zortos293/Lichii (an Eden/yuzu fork with Switch 2 work).
**Licence:** GPL-3.0-or-later (source files keep their Eden/yuzu/citra SPDX notices). The NEXUS
Engine is a fork of Lichii build 160 (commit `2eb5f081c`), so every Lichii change up to build 160
is already in NEXUS, under the same licence and notices. Nothing here describes a NEXUS change
unless it says so.

Sources examined: the engine fork's local history (tags `build-65` … `build-160`), in particular
the 30 commits `build-150..build-160` and the OoT-related commits `build-130..build-150`, read
from their commit messages and diffs.

## Build 150 → 160 (30 commits)

| Commit | Area | What was wrong → what changed | In NEXUS |
|---|---|---|---|
| 12062785e #1313 | file_sys | **OoT guard-house freeze.** With the Max Resolution built-in, render targets outgrow the graphics allocator's initial pool and the Castle Town guard house dereferences a null allocation (main+0x403f0, endless unmapped reads). The patch makes the allocator-init store (`stp x8,x9` at main+0xC2A36C) call a code cave that doubles the initial pool size (x8). Game-specific binary patch for a game limitation exposed by a higher resolution; matched by build id. Measured upstream: 3/3 freezes before, 0/2 after (RTX 4090, 2×). | yes (base) |
| f6bbee241 #828 | core | **Duskbloods network test** reads GPU-written values (tone pass exposure) back on the CPU; without reactive flushing it read stale values and the picture pulsed. Reactive flushing = when the CPU reads guest memory a GPU pass wrote, the pending GPU work is flushed and the data downloaded first. Forced on for that title only, as a session flag. | yes |
| bc3d0e394 #637 | video_core | Duskbloods uses a DLSS network for its upscaler; replaced by a temporal upscale of the render region with the game's motion vectors, jitter and history (handles different-size motion/depth targets in handheld mode). | yes |
| 9069e5a84 #638 | vulkan | 3D colour targets bound with a 1-layer depth target: clears were clipped to one layer, so fog volumes kept accumulating light → infinity → NaN. Clears of the named volume slices now go through a separate path. | yes |
| 32ba5b4a2 #552 | kernel, nx2_gsv | Duskbloods network test boots to menus: its heap needs more than the 12 GB arrangement leaves (env probe to grow the app pool; InfoType 31 must not be 0), GPU-only regions written by the CPU must be backed up front. Online checks remain unsolved. | yes |
| 4f3f9fe62 #1363 | shader | **Yoshi and the Mysterious Book black 3D world:** three shader forms were rejected – a geometry shader using ALD/OUT (the liveness check did not model them, so Unreal's tonemapping LUT pass was skipped and every frame after it was black), bindless TLD4 with AOFFI offsets and a uniform-register handle, and 3D SURED. General shader-compiler coverage, not a game hack: any game using these forms benefits. | yes |
| d42e90710 #1333 | buffer_cache | **Mario Kart World lighting:** #1231 sized read-only native compute bindings to the end of their mapping (up to 1 GiB) – which also backed OoT's 1 GiB heap with a 1 GiB device buffer. Back to an 8 MiB bound, growing only via a plausible NVN size word, which covers MKW's lighting reads. Any game whose compute shaders read more than 8 MiB through such bindings depends on the size word. | yes |
| e63d0a04d #1334 | vulkan | Graphics pipelines were evicted after 9000 frames unused, so revisiting a place re-translated hundreds of shaders on the GPU thread. Now kept up to a quarter of RAM (LRU). Option "build all cached shaders before game start". | yes |
| be2414796 #1337 | vulkan | An evicted pipeline keeps its SPIR-V (zstd) in a store of 1/64 of RAM; rebuilding it skips shader translation. | yes |
| 8e5259aa7 #1329 | vulkan | Empty command-buffer submits (~4,600/s in OoT Castle Town) skipped; +6 % fps measured upstream. Port of Fritchii EXP61. | yes |
| 20621cda9 #1327 | dynarmic | LDAR/LDAPR/LDAXR as plain loads and no barrier for DMB LD/ST on x86 (x86 already orders them). OoT Kokiri Forest 36.5→38.1-38.8 fps (upstream). | yes |
| fd9c0133d #1331 | dynarmic | Each CPU core's exclusive reservation on its own cache line (false sharing removed). | yes |
| 43af97ffa #1320 | shader_recompiler | Less compilation work (SSA definitions of predicates/flags kept in blocks). | yes |
| 933d35ff0 #1335 | vulkan | AMD drivers: allow device memory overallocation (8 GB cards ran out). | yes |
| b4b830c8f #1325 | vulkan | Images over the 2^28-texel limit that fit in 1 GiB are backed (Metroid Prime 4 light volumes drew black). | yes |
| 0490f57b9 #1326 | video_core | **Native HDR output** (HDR10/scRGB, Off/Auto/On, only listed swapchain formats, HDR10 metadata). | yes; NEXUS 0.7 adds the setting |
| 6f5a03143 #852 | core_timing | Looping events resumed after a pause were delayed by the pause length (audio pump stalled → Duskbloods stream error). | yes |
| 73a1b58e7 #1095 | engines | 3D state registers written in one call (~5 M writes/s in OoT). | yes |
| 752328bf3 #1332 | loader | MKW dynamic FPS; GpuSplit front/back cache lines split. | yes |
| 121e5c04a, 4c6ef2b81 | shader | More Ampere/Maxwell instruction forms (Pokémon Pokopia jump tables, TLD4.B). | yes |
| 23a6a56c6, babdb6bb7 | diagnostics | Sampler crash in generated code fixed; log rotation no longer loses the previous log. | yes |
| others | UI (Qt), LAN play, nxformat | Qt-only UI, LAN play, NX archive format. | engine parts yes; Qt UI not used |

## Build 130 → 150 (OoT items, not the guard house)
- Texture collector thresholds kept within the host's commit limit → OoT out-of-memory on
  Windows (#1210).
- ADPCM nibble sign → OoT ocarina crackle (#1196).
- FSR 3.1 / FSR 4 in place of OoT's anti-aliasing network (#1060-#1062).

## What NEXUS takes from this
- All of the above runs in NEXUS already (same engine source). NEXUS-specific work is the host.
- Engine settings NEXUS writes must match what Lichii is tested with (they do: first-run values).
- HDR is now selectable in NEXUS (*Emulation settings › Graphics › HDR output*).
