# NEXUS application performance (0.4.0)

## How it was measured

- **Setup.**
  - NEXUS ran with the stand-in engine (`NEXUS_FAKE_ENGINE=1`) and a portable data folder.
  - The interface ran in Chrome through Playwright, with the startup animation off.
  - Each figure is the median of 5 runs.
  - Two libraries were used: the normal test library (8 entries) and a large one (408 entries).
- **Scripts.** `app/tools/perf/ui_timings.py` and `app/tools/perf/idle_cpu.py`.
- **Limits.**
  - The engine and games are not part of these numbers.
  - The test machine is a Linux container, not a Windows PC with WebView2. The absolute times will differ on a user's PC, but the before/after comparison is like for like.

## Results

### Startup and navigation

| | 0.3.0 | 0.4.0 |
|---|---|---|
| NEXUS server ready after start | 10 ms | 10–14 ms |
| Interface loaded with data (8 games) | 248 ms | 260–304 ms |
| Interface loaded with data (408 games) | — | 333 ms |
| `/api/state` response | 4 ms | 3–5 ms |
| Open Home | 23 ms | 23–29 ms |
| Open Library (8 / 408 games) | 22 ms / — | 21 ms / 71 ms |
| Open System Settings | 51 ms | 42–73 ms |
| Open Media | 60 ms | 38–62 ms |
| Memory: NEXUS process | 19.2 MB | 19.1–19.3 MB |
| Memory: interface JavaScript heap | 2.6 MB | 2.6 MB |

Startup and navigation were already fast and stay the same within measurement noise. With the
startup animation on, the interface waits for the animation (about 4 s, a user setting): that is
by design, not loading time.

### Idle CPU while NEXUS is open (408-game library)

| Screen | 0.3.0 | 0.4.0 |
|---|---|---|
| Home | 5.3 % of a core, 60 style recalculations/s | **0.3–0.4 %**, 1/s |
| Library | 12.3 % of a core, 20 recalculations/s | **0.3–0.4 %**, 0/s |
| Settings | 1.6 % | 0.2 % |
| NEXUS server (Go), idle | 0 | 0 |

This matters most while a game runs in the background behind NEXUS Home.

## What was changed

1. **The focus highlight animated `box-shadow` and `border-color`.** Every frame was repainted on
   the main thread for as long as a tile had focus, which is always on Home and in the Library.
   It now breathes with `opacity` only, which the GPU compositor handles without a repaint.
2. **Controller polling ran every frame even with no controller connected.**
   - Now it runs every frame only while a controller is connected.
   - Otherwise it checks four times a second, and the browser's `gamepadconnected` event switches back to every frame immediately.
3. **The running-game dot and the recording indicator** pulsed with `box-shadow`. They now use `opacity`.
4. **The loading bar** moved with `left`, which forces a layout on every frame while a game is loading, exactly when the CPU is busy compiling shaders. It now moves with `transform`.

## 0.8.0: idle interface, measured (Linux, headless Chrome, stand-in engine)

Measured with `tools/perf/idle_baseline.py` on the development machine (Linux, headless Chrome
with Playwright, NEXUS with the stand-in engine, 10 s of doing nothing per screen). This is
**not** your Windows PC or WebView2; use it to compare builds, not as Windows figures. The
"before" build is 4b4daa7 (before this work), "after" is the 0.8.0 development build.

Second run, same script for both builds (10 s per screen):

| Screen | Page main thread (% of a core) | Style recalcs / s | Script ms / s | Server CPU % | Server RSS MB |
|---|---|---|---|---|---|
| Home, no controller | 0.31 → 0.19 | 1.0 → 0.0 | 0.77 → 0.70 | 0.0 → 0.0 | 19.2 → 19.2 |
| Home, controller connected | **4.47 → 0.70** | **60.0 → 0.5** | 7.19 → 4.26 | 0.0 → 0.0 | 19.2 → 19.2 |
| All Software, controller connected | **1.60 → 0.87** | **17.9 → 0.0** | 2.44 → 4.10 | 0.0 → 0.0 | 19.2 → 19.2 |
| Settings, controller connected | **1.83 → 0.50** | 0.0 → 0.0 | 5.97 → 3.34 | 0.0 → 0.0 | 19.3 → 19.2 |
| Home, controller removed | 0.45 → 0.34 | 1.1 → 0.7 | 0.77 → 0.69 | 0.0 → 0.0 | 19.3 → 19.2 |
| A game in front (stand-in engine) | 0.19 → 0.09 | 0.1 → 0.0 | 0.78 → 0.59 | 0.3 → 0.4 | 19.6 → 19.6 |

Run-to-run variance is large for small values: in the first run "Home, controller connected"
was 2.11 → 0.80 % with 59.9 → 1.5 recalculations a second. The consistent result is the
recalculation rate with a controller connected (about 60 a second, one per display frame,
before; about 0-2 after).

- NEXUS server (Go) while idle: 0.0-0.1 % CPU, 19-20 MB resident, both builds.
- Server ready after start: 8-15 ms warm (the first, cold start of the "before" run took 987 ms:
  disk cache); interface loaded with its games: 180-430 ms before, 200-250 ms after.
- **What changed:** with a controller connected the interface polled it on every display frame
  (requestAnimationFrame), which made the page produce a frame on every refresh even when
  nothing moved. It now polls every frame only while the controller is in use (input in the
  last 1.5 s) and 30 times a second otherwise (first press within ~33 ms).
- The performance bar's refresh timer no longer runs while no game is running (Windows only;
  not measurable here).
- The clock is written only when the minute changes (it caused the one recalculation a second
  on Home). While a game is in front, animations behind it are paused (the running light's
  pulse kept running there) and the 15-second system poll waits.
- Not measured yet: Windows/WebView2 idle CPU and GPU, memory after a long session, resource use
  while a game runs or prepares shaders. These need your PC.

## 0.8.0 (engine on Lichii build 177): changes, not measurements

None of these has a before/after measurement yet; they are listed so a measurement can be made
with the method below.
- **Texture-cache host-memory cap on Linux** (engine patch 0020). The cap Lichii applies on
  Windows (cache ≤ device use + commit room − 2 GiB, never under 3 GiB) now also works on Linux,
  where it reads `/proc/meminfo`. Expected effect: on a 16 GB shared-memory device a Switch 2
  game's cache collects before RAM and swap run out instead of the session being killed. Steam
  Deck and ROG Ally X are **not tested**; NEXUS has no Linux release.
- **Optimiser limits for Switch 2 games**: reactive flushing on, conservative video memory,
  resolution scale at most 1x, matching the configuration the engine is tested with. This
  favours stability and memory headroom over sharpness on large GPUs.
- **Engine changes inherited from build 177** (listed in `research/LICHII.md`): x86-64 code
  generation, GPU split and buffer-cache work, memory reuse, Windows commit-limit handling.

## Not changed (checked)

- **No per-frame work in the server.** It has three timers: the engine status (4/s, only while a game runs), the stand-in engine (tests only) and the event-stream keep-alive (every 20 s).
- **Animated wallpapers** already pause while a game runs, while NEXUS is in the background or minimised, and on screens where they are not visible (0.2.0).

## Benchmarking method (0.7.0)

Report a change only with numbers from this method:
1. Same PC, power plan and driver; NEXUS performance monitor on; game and scene fixed (OoT:
   Kokiri Forest standing still, then panning; Castle Town market; a Switch 1 game's first
   area).
2. Warm the shader cache once; then three 60-second runs before and three after the change.
3. Record average fps, 1 % low (from frame times), CPU %, GPU %, RAM and VRAM from the monitor.
4. Write the result in this file with the commit, hardware and settings.

## Handheld plan (Steam Deck, ROG Ally X)
- Steam Deck needs the Linux build first (engine host for Linux exists as `enginehost_dl.go`;
  packaging, Steam Input and SteamOS game mode are not done). No handheld has been measured.
- Candidates once measured: texture-cache ceiling and render-target collection (shared memory),
  resolution 0.75× with FSR, Mailbox or a 40 fps limit on 40 Hz displays, shader cache on
  internal storage.
