# Emulator audit (18 Nov 2026)

Requested in the 0.8 refinement pass: look at Lichii, Fritchii, Ryubing/Ryujinx and yuzu, and say
what NEXUS should take, without updating anything blindly. NEXUS runs one engine, the NEXUS Engine
(GPL-3.0, a Lichii fork with its own patches 0018-0041). It does not bundle or launch other
emulators; old per-game "emulator: ryujinx" choices from NEXUS 1.5-era profiles are migrated into
NEXUS Engine settings, and NEXUS can still import saves in Ryujinx's folder layout (`saves.go`).

| Project | State (checked 18 Nov) | Base / licence | Relevance | Decision |
|---|---|---|---|---|
| **Lichii** | Active. Tag build-341 (17 Nov) | Eden/yuzu fork, GPL-3.0+ | NEXUS Engine base (build 316 + selected 316→341 fixes, patches 0031-0038) | Rebase on build-341 is possible (all NEXUS patches apply with `git am --3way`) but not done: ~40 commits change the texture cache's write-back for every game and need game testing first. See `LICHII.md` |
| **Fritchii** | Active. v6 / v6.1 (18 Nov), now built on Lichii build-341. `linux-port` branch builds an AppImage | Lichii fork, GPL-3.0 | Ocarina of Time tuning for handhelds (ROG Ally X). Its switches (`FRITCHII_*`, EXP48/49/58/61/63) are candidates for the Deck | Nothing ported yet: no Deck logs or measurements to compare against, and Fritchii's own regressions were reverted upstream (EXP47). NEXUS's Deck preset uses the same tuned defaults (low GPU accuracy, async shaders, mailbox). See `FRITCHII.md` |
| **Ryubing** (Ryujinx continuation) | Active; stable 1.3.3 plus Canary builds. Moved to git.ryujinx.app | C#, MIT | Switch 1 only; no Switch 2 emulation. Different codebase (C#), so no engine code can be shared | No change. Save import from its folder layout stays |
| **Ryujinx** (original) | Discontinued (Oct 2024); last official 1.1.1403 | C#, MIT | None beyond save import | No change |
| **yuzu** | Discontinued (Mar 2024) | C++, GPL-2.0+ | Ancestor of Eden → Lichii → NEXUS Engine. yuzu copyright notices are kept (patch 0018) | No change |

## Recommendations

1. Rebase the NEXUS Engine on Lichii build-341 in its own release, with a before/after run of the
   games in `COMPATIBILITY.md`, because the texture-cache write-back series affects every game.
2. Once a Steam Deck log of Ocarina of Time exists, compare it with Fritchii v6.1 on the same Deck
   (it now ships an AppImage) before porting any single Fritchii switch, one at a time, measured.
3. Keep Ryubing out of NEXUS: a second engine would double the settings, shader and save paths
   for no Switch 2 benefit.

No benchmark numbers in this file were measured by NEXUS.
