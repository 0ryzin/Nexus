# NEXUS Engine: upstream sources, what is integrated, and what is not

NEXUS ships one engine, the **NEXUS Engine** (`nexus_engine.dll`). It is built from upstream Lichii
plus the NEXUS patches in [`engine/patches`](../engine/patches). This page records where its code
comes from, how the other Switch emulator projects relate to it, and what was and was not
integrated, so that nobody has to guess.

## Audit of the current engine (NEXUS Engine 0.2.0)

| Layer | Origin | Licence |
|---|---|---|
| Whole emulation core: CPU (dynarmic JIT), HLE services, kernel, file system/NAND, loader, GPU (Maxwell/Ampere command processing, Vulkan and OpenGL renderers, shader recompiler), audio core, input (SDL3) | **yuzu** → **Eden** → **Lichii** build-150 (zortos293/Lichii, tag `build-150`, commit `17baed716`) | GPL-3.0-or-later (yuzu components partly GPL-2.0-or-later, compatible) |
| DLSS replacement (NVIDIA Streamline loader, headers only), FSR 3/3.1/4 paths, frame generation (FSR 3, Lossless Scaling), NX2 (Switch 2) experimental work | Lichii / Eden | GPL-3.0-or-later; Streamline headers MIT; NVIDIA/AMD/Lossless runtimes are *not* shipped and are loaded from the user's PC |
| `nexus_engine` library and C interface, 0–200 % gain + limiter, explicit data folder, host-managed data folder, suspend/resume, controller layouts, packaging | NEXUS patches 0001–0011 | GPL-3.0-or-later |

The full upstream history (yuzu, Eden, Lichii) is in the engine source shipped in every release
(`Source\engine\nexus-engine-source.tar.gz`), with all original copyright headers intact.

## The other projects

| Project | Language / licence | Relation to the NEXUS Engine | Integration |
|---|---|---|---|
| **yuzu** | C++, GPL-2.0-or-later/GPL-3.0 | Ancestor of Eden and Lichii; discontinued in 2024. | Already the foundation of the whole engine. Nothing further to take. |
| **Eden** | C++, GPL-3.0-or-later | Direct ancestor of Lichii; Lichii regularly ports Eden changes. | Included through Lichii. Further Eden changes can be cherry-picked as patches when they apply to the Lichii tree; each needs its own build and game testing. |
| **Lichii** (nx2) | C++, GPL-3.0-or-later | The engine's direct upstream. | Base moved from build-130 to **build-150** in NEXUS 0.0.0 (51 upstream commits). Newer Lichii builds are adopted the same way: rebase the patches, build, test. |
| **Ryujinx / Ryubing** | C#, MIT | Separate emulator with a different architecture (ARMeilleure CPU, its own GPU and HLE layers, .NET runtime). | **No code can be merged** into a C++ engine. Specific fixes can be re-implemented in C++ where they apply (MIT permits this with attribution); none has been in this release. NEXUS reads only Ryujinx's keys during an import. |
| **"Raiden"** | — | No Switch emulator of that name could be verified (the name belongs to a shoot-'em-up series). | Not treated as a source. If a specific project was meant, its repository is needed. |

**Why not "combine all of them":** yuzu, Eden and Lichii are one lineage; Lichii already contains
the other two. Ryujinx cannot be linked into the C++ engine at all, and shipping it as a second
engine would mean two emulators inside NEXUS — the opposite of one NEXUS engine. The realistic path
is the one NEXUS takes: one engine, upstream Lichii as the base, compatible fixes from Eden/Lichii
adopted as tested patches, and Ryujinx fixes re-implemented individually where a game needs them.

## Support status

| | Status |
|---|---|
| Nintendo Switch games (Switch 1) | **Working** in the engine (upstream test suite passes in every build). Real-game behaviour in NEXUS has not yet been tested by the project (build machines have no GPU or games). |
| Nintendo Switch 2 games | **Experimental.** Code from upstream NX2 work is present; NEXUS makes no compatibility claim. |
| Frame generation (FSR 3), DLSS replacement, FSR 4 | **Experimental**, hardware dependent, not tested by the project. |
| Lossless Scaling frame generation | Needs the user's own Lossless.dll; not tested by the project. |
| Online play, local wireless, Amiibo | Not offered by NEXUS. |
