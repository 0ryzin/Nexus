# Lichii research (reference project)

**Project:** Lichii, https://github.com/zortos293/Lichii (an Eden/yuzu fork with Switch 2 work).
**Licence:** GPL-3.0-or-later. Since 17 Nov 2026 (NEXUS 0.8 development) the NEXUS Engine is
based on Lichii **build 316** (commit `5a66c8959`); before that on build 177 (`87d84d513`, 13 Nov)
and build 160 (`2eb5f081c`). Every Lichii change up to build 316 is in the NEXUS Engine. Lichii
#1544 removed the yuzu Emulator Project's SPDX copyright lines from 2,512 files; the NEXUS Engine
restores them (engine patch 0018), because the GPL requires keeping copyright notices. Checked
again on build 316: all 2,484 files that carried the yuzu notice at build 160 and still exist
carry it in the NEXUS Engine. Where Lichii added its own notice since (citron in `friend.cpp`
and `nifm.cpp`, ScarletNX in `mii_model.cpp`), both are kept. Nothing here describes a NEXUS change
unless it says so.

Sources examined: the engine fork's local history (tags `build-65` … `build-160`), in particular
the 30 commits `build-150..build-160` and the OoT-related commits `build-130..build-150`, read
from their commit messages and diffs.

## Build 177 → 316 (283 commits) - now in the NEXUS Engine

### After build 316: ported into NEXUS (17 Nov 2026)

- `bfc2359f0` #2041 *common: keep a sparse vector page writable when the fault handler commits it
  again* → patch 0025. Windows `FakePageFaultHandler` committed pages read-only (and re-protected
  committed ones read-only); a thread storing into a page it had just committed then faulted on a
  write the handler does not handle. Likely cause of the Ocarina of Time save-load crash
  (docs/CRASHES.md).
- `d589f9532` #2062 *texture_cache: unregister an image from its own address space's page table*
  → patch 0026.
- Not ported: `f0bf0cfda` #2055 (mapping provenance in unmapped-read logs; diagnostic, larger
  change to `DeviceMemoryManager`), and the other 11 commits (VP9, accounts, tests, docs, sockets,
  NCA discovery, frame dumps) which do not touch this crash.

Read from the commit messages and, for the items marked, the diffs. By area: video_core 39,
vulkan 34, texture_cache 12, shader 11, file_sys 10, core 9, Ocarina-specific 7+, crypto 7,
loader 5, buffer_cache 4, audio 3.

| Area | What matters for NEXUS |
|---|---|
| Ocarina of Time | #1901 stubs gsv command 903, the video finalize OoT sends after its intro movie (a hang candidate in the loading path); #1818/#2010 movie frame pacing; #2012, #1853, #1807 mesh pipeline and compile fixes; #1890 Castle Town device loss above 1x on AMD; #1871/#1875/#1995 output pass recognition |
| Game start | #1935 (diff read) the CLI registers the booted NSP/XCI so the game gets its NACP; **NEXUS had the same gap and now does the same (engine patch 0022)**. #1912 registers NUP add-on content in the loader; #1920/#1918 NSO loading checks |
| Shaders and pipelines | #1803 Loading Shaders waits for the last two sessions' pipelines and builds older ones in the background; #1838/#1685 when the driver cache is saved; #1712 cache flush threshold |
| Memory (handheld) | #1991 an integrated GPU's retention floor rises with its thresholds; #1860 recent images kept under the lowest budget seen lately; #1731 VMA blocks drained on old-age eviction; #1686 device-local memory VMA places in host memory; #1908 a compute upload list that grew forever |
| Out-of-memory and stability | #1870, #1931, #1963, #1993 degrade or retry instead of aborting when memory runs out; #1938 device loss ends the process cleanly; #1534 buffer join slowdown and crash |
| Stopping a game | #1932 kernel objects closed before the kernel shuts down (at stop and on failed loads); #1952 the cheat engine stops first |
| Frame pacing | #2021, #2023, #1779 frame generation and pacing count game frames |
| Audio | #1847 the UI volume is no longer applied again inside the emulated audio controller at boot. Checked against NEXUS patch 0001 (NEXUS sets its 0-200 % master volume at the sink): compatible, and it removes a double application |
| Memory detection | Steam Deck installed-RAM detection from udev (tests in `memory_detect.cpp`). Lichii still has no Linux commit room, so NEXUS patch 0020 stays |

Rebase: 21 NEXUS patches re-applied; 2 conflicts, both resolved by keeping both sides (the SPDX
lines above, and the memory tests). Excluded: nothing deliberately; Lichii's GPU trace replay and
the CLI's other additions are not used by NEXUS's engine host.

## Build 160 → 177 (87 commits)

Read from the commit messages and diffs (`git log build-160..build-177`). Grouped by area; the
number is Lichii's pull request.

| Area | Changes | Why it matters to NEXUS |
|---|---|---|
| **Loader / file system** | #1504: an installed update's ExeFS is no longer applied over a NUP or tree `.nx` program (they carry their own RomFS, so one version's code ran against another version's data); LayeredFS mods are logged. #1456 Switch 1 `.nx` detected as Switch 1. #1503 missing-key messages; keys derivable from the key file are derived. #1500 AoI storage reads throttled (boot hang). | **Prime suspect for OoT stopping after a save loads** when an OoT update is in the game folder: NEXUS passes updates from the game folders to the engine. |
| **Switch 2 GPU services (nx2_gsv)** | #1512 guest syncpoint increments mirrored before answering waits; #1590 vi2 command 53 timestamp recorders answered (Unreal sees its frames shown); #1413 the GPU thread never waits on kernel locks for sparse blocks; #1458 every device range in a released span released. | Waits that never end look exactly like an endless black loading screen. |
| **Core / kernel** | #1450 guest threads suspended before power-off (no PANIC when stopping); #1574 a process is registered only after its program loaded; #1576 teardown that still makes progress is waited for; #1451/#1580 zero-page trim pass no longer kills the emulator when Windows runs out of commit, reads less, backs off; #1581/#1532/#1528 SMMU and device memory blocks reused and returned. | Stopping games, memory on 16 GB PCs. |
| **CPU (dynarmic)** | #1531/#1570 x86-64 code generation from #831 (host call/ret, live flags, address folding, cold code); #1517 register allocator bitmasks (JIT compile time). | Faster code and JIT compilation. |
| **GPU / Vulkan** | #1516/#1515/#1519/#1520/#1521/#1585/#1586 split front/back optimisations (cheaper reports, fences, fast draws, command-list reuse); #1479 guest reads skip cache locks on untouched pages; #1518 page protection changed after dropping the lock; #1436/#1460/#1459/#1441 buffer-cache correctness; #1505 no cached-pipeline build after a boot that never finished; #1592 a puller torn down mid-wait leaves its waiters. | Fewer GPU-thread waits, correct buffers. |
| **Shaders** | #1468 fewer SSA phis; #1463/#1577 precache fixes and stale-precache rebuilds; #1511 HSETP2 with constant buffer; #1523 null texture handles; #1485 OoT light-table shadow maps (dark water grid). | Compile time, OoT water. |
| **Games** | OoT: #1508 DLSS frame generation, #1589 DLSS quality changes render resolution; AoI dynamic FPS (#1513); Star Fox rumble (#1491). | |
| **Input / audio** | #1497 DualSense HD haptics, #1499 two-motor rumble. | |
| **Renames** | #1541/#1550/#1551/#1552/#1553 yuzu → Lichii names (`lichii_cmd`, `LICHII_*` options); #1544 yuzu SPDX lines removed (restored in NEXUS). | NEXUS engine patch 0019 follows the new paths. |
| **Qt / Android only** | game list, recommended settings UI, Android package renames, 16 GB RAM warning dialog. | not used by NEXUS (NEXUS has its own UI) |

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
