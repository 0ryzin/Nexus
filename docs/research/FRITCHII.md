# Fritchii research (reference project)

**Project:** Fritchii, https://github.com/Frizti/Fritchii (a fork of Lichii focused on
Ocarina of Time performance and handhelds such as the ROG Ally X).
**Licence:** GPL-3.0 (`LICENSE.txt`), Eden/yuzu notices kept. Code from it may enter the GPL
NEXUS Engine only with its notices and a reference to the original commit; NEXUS's own Go code
does not take code from it.

Sources examined: the repository history (30,068 commits on the default branch; tags `frizti-v1`
… `fritchii-v5.8.4`, 5-11 November 2026), commit messages and diffs between release tags. The
GitHub release pages themselves could not be read from this environment.

## Releases

| Tag (date) | Commits | Main changes |
|---|---|---|
| frizti-v1 … v3, fritchii-v4/4.1, v5 … v5.2 (5-9 Nov) | | Fork start: built-in "switches" tuned for OoT (`nx2_switches.txt`), tuned defaults (GPU accuracy Low, Mailbox, async presentation, async shaders), OoT 60/30 fps choice, tuned settings re-applied once per revision |
| v5.3 | 4 | AMD Windows driver detected by vendor for the shader-library default; updater retries |
| v5.4 | 2 | AMD: precompiled pipelines recorded with a dynamic logic op build where it is static |
| v5.5 | 5 | NVIDIA shader cache size for its process; frame generation forced off (camera flicker); EDS2 and dynamic vertex input in the tuned settings |
| v5.6 | 92 | OoT high refresh mode (Ctrl+U, ripples above 60 fps); **EXP61** no empty submits (also in Lichii #1329); **EXP58** delete superseded guarded render targets after 60 frames; query-cache and fence lists as reusable vectors; semaphore ticks on own cache lines |
| v5.7 | 36 | **Texture-cache ceiling 7680 MiB** (VRAM 10 → ~6 GB over 30 min at 58.8 fps, upstream); **EXP49 async read-back** (Stalchild fight 51.9 → 56.9 fps); JIT warm-up; zero-page pass (trim untouched pages); no 40 MB/s game-file read pacing; one barrier pair for runs of uploads (Lichii #1245) |
| v5.8 | 16 | Own data folder imported automatically; **EXP48** stop write-protecting guest pages the CPU rewrites every frame; **EXP63** per-thread GPU page-translation cache; NVN conditional render macro on the back thread; skip dynarmic IR verification; **do not wait for time services when settings are applied during loading** (#1168, UI froze until loading ended); 16-byte block-linear swizzle |
| v5.8.1 … v5.8.4 | 12 | Save tools; DLSS quality per game; **EXP47 reverted** (read-back lock bits) |

## Relevance to NEXUS

| Technique | Status in NEXUS | Note |
|---|---|---|
| EXP61 empty submits | in NEXUS (via Lichii #1329) | |
| Texture-cache ceiling | **NEXUS-native equivalent (0.8.0, patch 0020)** | Fritchii caps the cache at a fixed 7680 MiB. Lichii 177 already caps it by the device budget and, on Windows, by the host's commit room (`host_memory_limit.h`); on Linux that cap was off because `GetSystemCommitRoom` returned nothing. NEXUS reports Linux's room (MemAvailable + SwapFree, or CommitLimit - Committed_AS in strict mode), which is what matters on a 16 GB shared-memory handheld. Written for NEXUS; no Fritchii code. Unit-tested, not measured on a handheld |
| EXP58 render-target GC | not taken | Lichii 177's collector follows the driver's budget and a retention limit; superseded render targets age out there. No measured problem in NEXUS to fix |
| EXP49 async read-back, EXP48 write-protect skip, EXP63 translation cache | candidates | measured gains upstream are OoT-specific; regressions were reverted there (EXP47). Port only with a before/after measurement |
| JIT warm-up, IR verification skip | candidates | load time and stutter |
| #1168 time services during loading | not needed | NEXUS applies settings only before boot |
| Tuned defaults (Low accuracy, async) | same as NEXUS's seeds | |
| OoT high refresh / FPS mods | not taken | game-specific; NEXUS uses the Lichii built-ins |

No Fritchii code was added to NEXUS. Where an idea was used, the NEXUS change says so in its
comment and commit message.
