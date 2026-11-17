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

## Ocarina of Time loading, measured from the user's logs (17 Nov 2026)

Source: the engine log (`eden_log.txt`) and `nexus.log` of two real sessions on the user's PC.

- **PC:** RTX 4060, driver 621.14, 6.3 GiB VRAM shown to Vulkan.
- **Game and engine:** NEXUS Engine 0.8.0 on build 316.
- **Settings:** resolution setup 3, async shaders on, shader preparation "Always", FSR3 frame
  generation on.

Times are seconds; the Play click is at most 0.2 s before the session starts (`nexus.log`).

| Step | 13:31 session (first after preparing) | 14:39 session |
|---|---|---|
| Shader preparation (convert the game's shaders) | 14.3 s | 0.0 s (already current) |
| Game loading starts (after the session starts) | 2.5 s | 2.0 s |
| Pipelines from the shader cache (1,304) | 75.0 s | 5.8 s |
| **Boot precache of compute programs** | **512.8 s**: 4,141 of 4,671 built, then *Start now* | **81.9 s**: 4,250 of 4,671 built, then *Start now* |
| First frame (title screen), after the session started | 600 s | **99.3 s** |
| Loading the save: from the first unmapped reads to normal frames | — (NEXUS closed; fixed since) | about 12 s (191.7 → 203.6 s); one 8.7 s frame |

### What these show

- **The boot precache was most of the wait, and its work was thrown away.**
  - It builds the game's 4,794 compute programs once to fill the GPU driver's cache.
  - Only a precache that ran to the end was remembered. After *Start now*, the next boot again
    had "4,671 of 4,794 to build", although 4,141 and then 4,250 had already been built.
  - The second run was 6× faster only because the driver's own cache answered.
- **Fix: engine patch 0028.** The programs built are recorded next to the precache file, every
  20 s and when it ends early. The next boot builds only what is left, about 420 programs here.
  - Safety: the same checks as the "done" marker (precache contents, driver, device, cache
    version, settings variant).
  - **Not measured yet:** the expected next boot is roughly 10 s plus the remaining programs,
    instead of 100 s. That is not claimed until a session on that PC shows it.
- **Pipelines from the cache:** 75.0 s the first time (cold driver cache), then 5.8 s. Nothing to
  change.
- **Save load (about 12 s).** In the log:
  - 277 ASTC textures decoded on the GPU (11.9 s of decode time over the window);
  - five graphics pipelines not in the cache, built asynchronously but finishing together after
    5.5–7.9 s;
  - 1.5 s trimming zero pages.
  
  The pipelines reach the cache for the next load. Why their builds waited is not known from the
  log (no stutter trace). Nothing was changed there without a measurement.
- **CPU and RAM:** the engine log records memory, not CPU. About 4.2 GB of guest memory was
  resident after the load, with 2.9–3.1 GB of Windows commit left. NEXUS's own interface process
  was idle.
- **Not measured:** time from selecting a save to control of Link, CPU use per thread. A stutter
  trace (`EDEN_NX2_STUTTER_TRACE=1`) of one save load would show the second.

## 0.8.0: startup, measured (17 Nov)

**On the user's PC (Windows, from `nexus.log`, before the change; not re-measured after):**

| Start (17 Nov) | core ready | window created | window shown | WebView2 ready | interface loaded |
|---|---|---|---|---|---|
| 13:30 (first after a new download) | 527 ms | 535 ms | ~1.26 s | +2.58 s | 4.16 s |
| 14:50 | | | | +518 ms | 1.15 s |
| 18:02 | 82 ms | 89 ms | 1.64 s | +927 ms | 3.14 s |
| 18:22 | 16 ms | 24 ms | 180 ms | +1.47 s | 1.85 s |
| 22:03 | 16 ms | 23 ms | 753 ms | +870 ms | 1.90 s |

With the startup animation on (the default) Home appears about 4.3 s after the interface loads.
The account synchronisation already ran in the background and did not delay any of these.
From 0.8.0 the log also records *home interactive*, *library available*, *account restored* and
*first background sync finished*, and how long showing the window and bringing it to the front
took (the 0.15–1.6 s between "window created" and "window shown" is not explained yet).

**Interface part, before and after** (`app/tools/perf/startup_timings.py`: Linux build, stand-in
engine, Chrome opened as soon as the server answers; median of 9 starts; ms from process start):

| | before | after |
|---|---|---|
| Server answers | 12 | 11–13 |
| Interface has its data | 76–80 | 70–84 |
| Game list available (animation off) | 183 | 79 |
| Home interactive (animation off) | 235 | 147 |
| Home interactive (animation on) | 4394 | 4407 (the animation) |
| Account restored | 1 | 1 |

Causes and changes: the 14 interface sounds were requested before the game list, and the browser
opens six connections to NEXUS at a time, so the game list waited behind them; without the
animation start-up still waited up to 0.6 s for the sounds and decoded the animation's artwork;
two requests were made only after the animation. Now the first `/api/state` (`?boot=1`) carries
the game list of the user picked automatically, sounds load in the background unless the
animation needs them, and the remaining requests are made at once.

## 0.8.0: Home layouts, idle cost (Linux, headless Chrome, stand-in engine, 17 Nov)

`tools/perf/idle_baseline.py`, 10 s of doing nothing on each screen, 6 games. It measures this
machine and headless Chrome, not WebView2 on Windows, so compare builds with it; do not quote
these as Windows numbers. The GPU cost of the blur cannot be measured headless: *Flat* panels
avoid it.

| Screen | Page main thread (% of a core) | Script ms/s | Layouts/s | JS heap | NEXUS server CPU / RSS |
|---|---|---|---|---|---|
| Home (Classic) | 0.13–0.19 | 0.57–0.67 | 0.0 | 2.5 MB | 0.0 % / 19 MB |
| Home Cards (picture behind, 3 widgets) | 0.23 | 0.63 | 0.1 | 2.1 MB | 0.0 % / 19 MB |
| Home Grid (glass, 4 widgets) | 0.18 | 0.64 | 0.1 | 2.2 MB | 0.0 % / 19 MB |
| Home Grid, flat | 0.14 | 0.56 | 0.0 | 2.4 MB | 0.0 % / 19 MB |
| Home with a controller connected | 0.65 | 3.84 | 0.2 | 2.4 MB | 0.1 % / 19 MB |
| Game in front | 0.09 | 0.57 | 0.0 | 2.1 MB | 0.6 % / 19 MB |

The layouts add no timers or polling. Their small difference is within run-to-run noise.

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

## 0.8.0: after closing games, measured (Linux, stand-in engine)

What NEXUS itself keeps after a game closes, with the stand-in engine (17 Nov). The real engine's
own teardown (its threads, GPU memory, Lichii #1932 closing kernel objects at stop) runs in the
engine library and needs a Windows run with a game.

- Goroutines: 2 before, 2 after five launch/close cycles (`TestSessionCleanup`, in CI).
- NEXUS server memory (resident), 20 launch/close cycles: 13.3 MB idle before; 13.9 after the first
  close, 15.7 after 5, 16.6 after 10, 17.0 after 15 and still 17.0 after 20. It levels off (Go's
  allocator and NEXUS's caches settling), no growth per session after that.
- Leak check of the interface (`tools/perf/nav_soak.py`, 60 rounds of 8 screen changes): DOM nodes
  1,070 and event listeners 215 before and after; JS heap +0.19 MB.

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
