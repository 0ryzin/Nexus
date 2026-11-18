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

## The 12 GB question

The emulated console's memory layout (*Emulated memory (DRAM)*, engine `memory_layout_mode`)
sets how much memory the emulated system reports and maps. Switch 2 games size their heap from it
and need 12 GB; the engine switches to 12 GB itself for a Switch 2 program, *if* its check of
the game file before start-up recognises it (`FindSwitch2Program`). The user sees a black picture
below 12 GB on both the Deck and a PC, so that check is missing their copy; the game process then
still runs as Switch 2 (it reads its own metadata) but with the configured memory and the
Switch 1 guest clock (19.2 MHz instead of 31.25 MHz), which also stretches frame waits 1.63x.
Engine patch 0042 makes NEXUS's title id the fallback. Not confirmed with the game: the engine
log line `Switch 2 program ... 12 GiB` or the patch's warning will show it.

12 GB is not "expensive" by itself: memory is reserved as address space and only pages the game
touches use RAM. It is not forced on Switch 1 games.

Sources: lichiiemulator.com/compatibility-status, github.com/Syyrhc/Lichii-Ally/issues/4,
fritchii.net, hookshotpc.com/hookshot, gamerbrowser.com (Fritchii v6), notebookcheck / XDA /
Wccftech launch coverage (15-60 fps on desktop PCs).
