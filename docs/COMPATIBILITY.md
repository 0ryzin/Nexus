# Game compatibility

NEXUS runs games with the NEXUS Engine = Lichii build 177 (since 0.8 development; build 160 before)
+ NEXUS's host patches. Game fixes in Lichii up to build 177 are therefore in NEXUS; differences in behaviour come from the
NEXUS host (window embedding, hidden start, input, save folder per user) or from settings.

## Switch 1 and Switch 2

| | Switch 1 | Switch 2 |
|---|---|---|
| Memory layout | 4 GB unless the user chooses more | the engine forces 12 GB for Switch 2 programs (`core.cpp`) |
| Clock | 19.2 MHz counter | 31.25 MHz counter (`Common::WallClock::Switch2CNTFRQ`) |
| GPU | Maxwell (NVN) | Ampere-class native macro engine ("native" draws, `native_turing`) |
| Services | standard | additional nx2 services (`nx2_gsv`, `nx2:vi2`) |
| Packages | NSP/XCI | NUP/NSP (NEXUS reads NUP since 0.3.0) |

A fix for one generation says nothing about the other. Test plan: `docs/PERFORMANCE.md` (benchmark
scenes) and the list below.

## Known issues

### The Legend of Zelda: Ocarina of Time (Switch 2 edition, 0400486024DEC000) - black screen after loading a save
- **Exact report (13 Nov):** the title screen and save selection work; after a save is loaded the
  screen stays black, loading, and gameplay never starts.
- **Leads fixed by moving the engine to Lichii build 177 (not yet confirmed with the game):**
  - #1504: an update's ExeFS was applied over the NUP program, running one version's code against
    the NUP's own (other version's) data. NEXUS passes updates found in the game folders to the
    engine, so an OoT update next to the NUP triggers exactly this; code and data then disagree
    once gameplay data is read after the save loads.
  - #1413 (GPU thread waiting on kernel locks for sparse blocks), #1512 (syncpoint waits answered
    before the increments were mirrored), #1590 (vi2 timestamp recorders): waits that never end.
- **What to check if it still happens:** the engine log (`Engine/log/eden_log.txt`) shows
  "Skipping ... update ExeFS" for #1504, and the last service calls before the stall.

#### Earlier notes (stuck loading, 12-13 Nov)
- **Reported:** NEXUS 0.6.0 stops on the game's loading screen; Lichii build 160 continues.
- **Ruled out by source comparison:**
  - The built-in patches (Max Resolution, Texture Pool x2, Dynamic FPS/FPS Unlock) are in the
    NEXUS Engine unchanged (`patch_manager.cpp`) and apply by build id.
  - Texture Pool x2 fixes a *guard-house* freeze in Castle Town (a null allocation after the
    render targets outgrow the allocator's initial pool when Max Resolution is on), not the
    first loading screen (Lichii #1313).
  - Engine settings: NEXUS seeds the same first-run values as Lichii's frontend and its
    recommended settings (GPU accuracy Low, NVDEC on the GPU, async presentation and shaders);
    Fritchii also tunes OoT with these. Not the difference.
- **Remaining candidates (NEXUS host):** the game window starting hidden and inside the NEXUS
  window (patches 0010-0017), the per-user save folder (`Data Storage/save_directory`), applets
  answered by the engine's default front ends (no Qt dialogs).
- **Next test (needs the game):** start OoT once with *Settings › Game sessions › Game window ›
  Separate window*; if it still stops, send `System/Engine/log/eden_log.txt` (or the file
  *Open diagnostic log* shows) from the attempt. The log shows the last service calls, shader
  and pipeline activity and whether the cores are still running.

### Fixes in Lichii (and so in NEXUS) worth knowing
See `docs/research/LICHII.md`: OoT guard house (Texture Pool x2), OoT out-of-memory on Windows
(texture collector within the commit limit), OoT ocarina crackle (ADPCM sign), Yoshi and the
Mysterious Book black 3D world (shader forms), The Duskbloods network test (memory, reactive
flushing, 3D clears), Mario Kart World lighting (compute binding size).
