# Ocarina of Time (Switch 2 remake) on the Steam Deck: research (18 Nov 2026)

The game: *The Legend of Zelda: Ocarina of Time* for Nintendo Switch 2 (released 5 Nov 2026),
title id `0400486024DEC000`. Not Ship of Harkinian / OpenOcarina (decompilations of the 1998 N64
game) and not Ocarina of Time 3D, which search results often mix in.

## What runs it

| Project | What it is | Steam Deck / handheld data (as published) |
|---|---|---|
| **Lichii** (zortos293) | The Switch 2 emulator the remake was first run on; NEXUS Engine base (build 316) | Lichii's compatibility notes (build 228, #823): Kokiri Forest on a **Steam Deck OLED 11.62 → 14.74 fps**, peak GPU RAM ~1.7-1.8 GB (was 2.3-2.9). Build 228 also fixed Deck ASTC (#948) and suppressed the 16 GB warning on the Deck (#1506). Open #871: AMD GPUs can lose the Vulkan device after the intro movie or a save load |
| **Fritchii** (Frizti) | Lichii fork tuned for OoT; v6/v6.1 (18 Nov) on Lichii build-341, first Linux builds | Aimed at 60 fps in Castle Town on its maker's PC. No Deck figures published |
| **Lichii-Ally** (Syyrhc) | Lichii builds for ASUS handhelds, AVX-512 variants | Issue #4, ROG Xbox Ally X (Z2 Extreme, Radeon 890M, 24 GB, Windows 11), handheld 1x, 40 fps target: opening ~20-22 fps (Lichii-Ally v5.1, Accurate), ~30 fps (Fritchii v5.5), Kokiri Forest ~21 fps (Lichii build 95); all three froze later. GPU accuracy **Fast gave a black screen before the title menu** there |
| **Hookshot** | Native PC recompilation of the remake from the user's dump (not an emulator) | v2.1.0 (11 Nov): runs natively on SteamOS; one report ~20 fps in the Great Deku Tree. Maintainer unnamed, licence and source not stated on its reference page: not integrated |
| Ryubing / Ryujinx / yuzu | Switch 1 only | Cannot run Switch 2 games |

No published Ally X figure in the 30-40 fps range with settings and build could be found; the
one detailed Ally X report shows ~20-30 fps in the opening and freezes. 30-40 fps reports are
plausible on the Ally X for some areas, but unverified here.

## Why the Ally X does better (hardware, not settings)

| | Steam Deck (LCD / OLED) | ROG Ally X (Z1 Extreme) | ROG Xbox Ally X (Z2 Extreme) |
|---|---|---|---|
| CPU | 4 Zen 2 cores | 8 Zen 4 cores, AVX-512 | 8 Zen 5 cores, AVX-512 |
| GPU | 8 RDNA 2 CUs | 12 RDNA 3 CUs (780M) | 16 RDNA 3.5 CUs (890M) |
| RAM | 16 GB LPDDR5 (shared) | 24 GB LPDDR5X (shared) | 24 GB LPDDR5X (shared) |
| OS / driver | SteamOS, Mesa RADV | Windows, AMD driver | Windows, AMD driver |

Switch 2 emulation is CPU-heavy (the game's CPU code is recompiled at run time and the GPU
command stream is translated on more threads), and the remake is GPU-heavy. The Deck has half the
CPU cores of an older generation, no AVX-512, and two thirds of the GPU units of the Ally X.
With the 12 GB layout the emulated console's memory plus SteamOS, NEXUS and the GPU share 16 GB on
the Deck; on the Ally X there are 24 GB. None of that is a setting. What transfers: lower
internal resolution, background shader compilation, a frame cap, and keeping GPU memory down
(Lichii #823), all of which NEXUS's Deck preset and engine (build 316 + patches) already have.

**Realistic Deck limit today:** about 12-20 fps in demanding areas (Lichii's own Deck figure and
the Hookshot report), possibly more in small interiors. 25-40 fps is not achievable on the Deck
with current Switch 2 emulation; NEXUS does not claim it.

## The 12 GB question (updated 18 Nov, 22:30, with the user's PC result)

New evidence: on the user's PC the game now renders at **4 GB** and runs at the **same frame rate
as at 12 GB**. In the NEXUS engine (`core.cpp`, `System::Load`) a Switch 2 program is always
switched to the 12 GB layout (`memory_layout_mode` = 12 GB) before the kernel starts, whatever the
setting says, when the pre-start check (`FindSwitch2Program`) recognises the game. Identical frame
rates at "4" and "12" mean that recognition works for the user's copy: both runs used 12 GB.
Therefore:

- **The DRAM setting was never the variable** for this game in NEXUS. 12 GB is the game's real
  requirement (Switch 2 consoles have 12 GB; the game sizes its heap from it), and it is applied
  automatically. Lowering it does not make the game faster or lighter. NEXUS now shows the row as
  "12 GB (set by the engine)" for Switch 2 games.
- Engine patch 0042 (title ID fallback) was written for the case where recognition fails; the PC
  result shows it does not fail for this copy. It stays as a safety net (no change when the check
  works).
- The earlier black screen therefore came from something that differed between runs, not memory.
  Candidates with evidence, in order: (1) first-run pipeline building on AMD: Lichii recorded the
  intro freezing for 6-18 minutes on AMD while the driver optimises two compute shaders (closed
  issue), and NEXUS builds the game's pipelines on the first run; the Deck's 4-core CPU makes this
  much longer than on a PC; (2) AMD-specific rendering faults in this game: Lichii tracks AMD black
  screens and device losses at the intro-to-3D handover (#871, open; #249, #503, #669 fixed in
  builds before NEXUS's base 316); (3) the earliest test used an older emulator build ("when I first
  started using emulators"), from before those fixes.
- Why the Deck differs from the PC: AMD RDNA 2 graphics with SteamOS's Mesa RADV (the PC's GPU
  vendor and driver differ), task/mesh draws on the compute fallback on RDNA 2 (the engine does
  this on purpose: native mesh draws hang RDNA 2, `vulkan_device.cpp`), 4 Zen 2 cores, and 16 GB
  shared with the GPU (the emulated 12 GB is reserved address space; only pages the game touches
  use RAM, but Longshot's README warns that 16 GB systems may need swap for this game).
- Without a Deck log the exact Deck cause stays unconfirmed. The engine log (`System/Engine/log/
  eden_log.txt`) of a black-screen run would show a device loss (`Device loss occurred`), pipeline
  building, or neither.


## Complete black screen in 0.8.0 (19 Nov) — investigation, not yet solved

**Report:** with 0.8.0 on the Deck, Ocarina of Time shows nothing at all (no intro, no menu, no
background, no audio). The build before (CI run 44412179858, commit b0160e4) showed the menu
without its background. No Deck log yet.

**What changed between the two builds (complete list):** engine patch 0043 (L4/R4 Home: reads
one SDL button event, cannot affect rendering), engine patch 0044 (AV1 through libdav1d: the
intro and title movies are now *decoded* instead of skipped), and on the NEXUS side the Linux
performance strip, Add to Steam, sounds, keys, diagnostics, artwork sync — none of which touch
the engine's launch except the strip.

**Ruled out, with evidence:**
- The performance strip covering the screen: gamescope (source read, `steamcompmgr.cpp`) paints
  `GAMESCOPE_EXTERNAL_OVERLAY` windows with `PaintWindowFlag::NoScale` and its own opacity, and
  `GetPossibleFocusWindows` skips them, so the strip can neither be stretched over the game nor
  take focus from the game window. It only exists when the performance monitor is on.
- AV1 decoding too slow to show anything: measured here (2 server cores, AVX2) with libdav1d,
  3840×2160 10-bit AV1: 30 fps with the engine's low-delay mode, 44 fps without; the conversion
  to NV12 adds ~7 ms a frame. A Deck (4 cores/8 threads) should be in the same range: slow movies
  would be late, not black. (Not measured on a Deck.)
- dav1d with one frame of delay cannot hold a picture back for ever (dav1d decodes synchronously
  when it has one frame context).

**Most likely remaining cause:** the AV1 path itself (0044) — the only change on the game's
video path. What exactly fails needs the Deck's engine log.

**0.8.1 makes this decidable in one test:**
- *Emulation settings › Advanced › Play Switch 2 movies (AV1)* (per game too). Off = exactly the
  pre-0044 behaviour. If OoT's menu returns with it Off, AV1 is the cause; if it stays black,
  it is not.
- Engine patch 0045 logs each decoder's first picture (size, pixel format) and the decode thread's
  average/longest decode time at frame 1, every 600 frames and at the end of each movie.
- *Settings › Storage & data › Send this device's logs* on the Deck, then *Fetch* on the PC.
