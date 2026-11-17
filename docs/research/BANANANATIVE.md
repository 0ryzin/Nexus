# Banana Native research (reference project)

**Project:** Banana Native, https://github.com/shamonj03/BananaNative (read 17 Nov 2026 at
`341299f`, 8 Nov). An Android port of seedds' nxnative. **Not an emulator**: like Wine, it runs a
game's own ARM64 code on the phone's CPU, supplies the system services the game calls, and
translates its NVN graphics to Vulkan (Metal on Apple). No NEXUS code comes from it.

## What it does that is relevant

| Technique | How it works there | Applies to NEXUS? |
|---|---|---|
| **Ahead-of-time "pack" per game** | Each game is prepared once (about an hour on a phone, faster on a PC): its code rewritten for the host and **all its shaders translated to Vulkan in advance**, found in the game files (nxnative's `shader_split.py`, the same fingerprinting Fritchii's extractor follows). | The idea, yes: NEXUS now finds the game's shader programs in its files and compiles the compute ones before play (docs/SHADERS.md). Translating graphics shaders fully in advance needs render state, which Banana Native gets from its own NVN layer at runtime plus the driver's cache; on first starts of OoT it still reports minutes of loading and black frames "until the GPU driver's pipeline cache fills". |
| **Texture storage shared by (pool, offset, description)** | Host textures are reused for NVN texture objects over the same storage; a new texture takes over overlapped storage; resolution variants share one exact-sized heap. Fixed a ~13 MB/frame leak (Super Mario Bros. Wonder) and growth to 1.6 GB (STEINS;GATE). | Not directly: NEXUS emulates the GPU, and its texture cache (Lichii) tracks images by GPU address with its own overlap, join and collection rules (build 316 adds #1845 bad-overlap collection, #1731 VMA block draining, #1991 integrated-GPU retention). Copying Banana Native's rules without measuring a game would be guesswork. |
| **Guest memory sizing by measurement** | Heap, alias region and stack sizes per game, measured from the game's own memory calls (`DM_SVC=1`), with SDK allocator patches. | No: NEXUS gives Switch 2 games the console's full 12 GB, as the engine emulates the whole memory map. The principle (measure before sizing) is what NEXUS follows: the low-RAM warning and the Linux commit-room cap use measured host memory. |
| **Guest memory imported into Vulkan by FD aliasing** | Android drivers without placed memory import guest memory through file descriptors. | Android-specific; NEXUS on Android is not released. |

## Implemented in NEXUS from this study

Nothing as code. The shader preparation (docs/SHADERS.md) follows the same principle Banana Native
and Fritchii share (prepare a game's shaders from its own files before play), using the precache
builder already in NEXUS's engine. Texture-storage sharing and memory sizing need measurements on
games and handheld hardware first; they are noted here for when that is possible.
