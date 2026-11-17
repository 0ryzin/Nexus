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
- **17 Nov report: NEXUS closes completely after "Load existing save"** (black screen, then no
  NEXUS, no message). Logs read, cause most likely Lichii #2041 (ported as patch 0025), not
  confirmed; crash reporting added so a repeat leaves a dump. Details and what was and was not
  tested: docs/CRASHES.md.
- **Exact report (13 Nov):** the title screen and save selection work; after a save is loaded the
  screen stays black, loading, and gameplay never starts.
- **Leads fixed by moving the engine to Lichii build 177 (not yet confirmed with the game):**
  - #1504: an update's ExeFS was applied over the NUP program, running one version's code against
    the NUP's own (other version's) data. NEXUS passes updates found in the game folders to the
    engine, so an OoT update next to the NUP triggers exactly this; code and data then disagree
    once gameplay data is read after the save loads.
    Checked in the build-177 source: the update lookup goes through the system's content
    provider union, which includes the slot NEXUS registers game-folder updates in
    (`nexus_engine.cpp`, FrontendManual), and the NUP loader builds its RomFS from the NUP's own
    packages, so the fix covers NEXUS's path too. It only applies when the game file is a NUP;
    for an NSP/XCI with an update the mismatch could not happen this way.
  - #1413 (GPU thread waiting on kernel locks for sparse blocks), #1512 (syncpoint waits answered
    before the increments were mirrored), #1590 (vi2 timestamp recorders): waits that never end.
- **Since 17 Nov (engine on build 316):** two more leads are fixed. (1) NEXUS registered only
  update and DLC files in the engine's content provider, not the booted NSP/XCI itself, so a
  game started from an NSP or XCI read an empty NACP (its control data, which includes the save
  data sizes); Lichii fixed the same gap in its CLI (#1935), NEXUS now does too (patch 0022).
  (2) Lichii #1901 answers command 903 that OoT sends after its intro movie. Neither is proven
  to be your black screen until the game is run.
- **NEXUS-side changes (0.8.0):**
  - The optimiser no longer recommends settings for a Switch 2 game that differ from the
    engine's tested configuration: reactive flushing stays **on** (Lichii's desktop default;
    NEXUS used to turn it off for most profiles, and games that wait on GPU read-backs while
    loading are the ones it matters for), video memory stays **conservative**, and resolution
    scaling stays at **1x**. Not proven to be the cause; it removes a difference from the
    configuration the engine is tested with. Settings applied by an older NEXUS stay until the
    optimiser is run again or *Restore* is used.
  - Every Switch 2 session writes one log line with those settings and the update/DLC file
    names passed to the engine (`engine: Switch 2 session ...` in the NEXUS log).
  - A Switch 2 game on a PC with less than about 16 GB of RAM starts with a warning.
- **What to check if it still happens:** the NEXUS log line above, and in the engine log
  (`Engine/log/eden_log.txt`) the warning "ExeFS: Ignoring installed update ... the container
  carries its own program and RomFS" (#1504 working) and the last service calls before the
  stall.

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
