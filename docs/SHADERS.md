# Shaders in NEXUS: preparation, caches and what still happens while playing

This explains what NEXUS does with a game's shaders before and during play, how that compares with
Fritchii, and what can and cannot be shared between devices. Nothing here claims a measured
reduction in stuttering: that needs your PC and your games (see *Not measured* at the end).

## Words

A Switch game ships **guest shader programs** (Maxwell/NVN machine code) inside its files, usually
in shader archives (BNSH, agl/NDSR packs, effect bundles). Before the GPU can draw with one, the
emulator:

1. **finds** the program (in the game files, or when the game binds it),
2. **lowers and translates** it to SPIR-V, using facts about how the game uses it (constant buffer
   values, texture types, storage pointers),
3. lets the **driver compile** the SPIR-V for this GPU, and
4. creates a **pipeline**: the compiled shaders plus the render state they are drawn with (blend,
   depth, vertex input, attachment formats...). A graphics pipeline needs the render state; a
   compute pipeline needs only its program.

Steps 2-4 are what stutters when they happen in the middle of play.

## What Fritchii does (read from its source, `fritchii-v5.8.4`)

Fritchii's "Preparing shaders" is Lichii #895 (not merged into Lichii's main line) plus its own
tooling (`tools/nx2-precompile`, `src/video_core/nx2_precompile.*`, `nx2_recipe.*`,
`core/file_sys/nx2_shader_extract.*`, `vk_precompiled_kept.*`, about 5,000 lines of C++ and a set of
Python models):

1. **Extract**: every shader program in the game's own files, found by content (zstd with the game's
   dictionaries, Yaz0, SARC, BNSH, BAHS, NDSR, movie files). For Ocarina of Time: 42,848 programs in
   about 30 s.
2. **Scan**: each program lowered as the runtime lowers it, its runtime hash computed, and translated
   against a static "fact oracle" that guesses the facts the translator asks for.
3. **Predict render states**: from recorded sessions' caches and the game's own variation tables
   (Python models), giving a precompiled pipeline file (`.nx2prep`): 133,016 pipelines for OoT.
4. **Recipe**: the same file without the game's code (1.3 MB for OoT), so it can be shared; a player
   assembles it with their own copy of the game.
5. **At boot** every pipeline of the file is built once and dropped; the driver keeps the compiled
   code in its own cache. A real draw still translates its program with real facts, and when its
   pipeline is one the file predicted, the driver finds it compiled. Needs a driver that keeps its
   own pipeline cache (NVIDIA, AMD proprietary) and, on NVIDIA, Shader Cache Size = Unlimited. A
   first preparation of OoT takes about 15 minutes on an 8-core CPU.

Fritchii's recipes are made per game by its developers from recorded sessions.

## What NEXUS does (engine 0.8 on Lichii build 316)

Lichii build 316 already contains a smaller form of step 1 for **compute** programs, the shader
precache (`video_core/shader_warm`): it reads the game's files with the user's keys, finds the
programs in its shader archives and converts the compute programs into a precache file in the
title's shader folder. Lichii's own frontends offer it by hand. NEXUS uses it automatically:

| Before play | What happens | Where |
|---|---|---|
| **1. Find and convert** | The engine's builder reads the game's files (your copy, your keys), finds the shader programs in its shader archives and converts the compute programs into `shader/<title id>/warm-compute.bin`. Loading screen: *Reading the game's files*, *Finding shaders in the game's files* (n of archives), *Converting the shader programs* (n of programs), *Saving the prepared shaders*. Once per game and engine build; a current file is checked and skipped at once. | engine patch 0023, `app/shaderprep.go` |
| **2. Compile before the game starts** | With *Prepare shaders before playing* on, NEXUS sets `precache_shaders = Always`: at boot the engine translates and compiles every program of that file before the game runs (*Compiling the game's shaders*, n of m). A marker remembers it per driver, device and SPIR-V variant, so it runs again only after a driver or engine change, or if it was skipped. *Start now* skips the rest; the next start finishes it. | Lichii's precache, NEXUS turns it on |
| **3. Earlier sessions' pipelines** | Graphics and compute pipelines the game used before (on this PC, or brought from another device, see below) are built at *Building shaders from earlier sessions* (n of m). Lichii #1803: the last two sessions' pipelines before play, older ones in the background. | Lichii |

Settings › Game sessions › *Prepare shaders before playing* (on by default). A game's Options menu
has *Prepare Shaders Now*, which runs step 1 in the background on Home while no game runs.

### What still happens while playing

- **Graphics pipelines the game has never drawn on any of your devices.** Their render state is not
  in the game's files; Fritchii predicts it with recorded sessions and per-game recipes, which NEXUS
  does not have. They are compiled when first drawn (asynchronously where the setting allows, which
  can skip a draw for a moment instead of freezing) and saved, so each happens once.
- Compute programs translated again because the game filled a storage pointer later (Lichii
  `StaleNullPointerSlot`).
- Programs the precache builder could not convert (counted as *skipped* in the result).

The performance bar's *Shaders* reading shows this while playing: *n compiling* (builds the game is
waiting for or will need now: these can stutter), *n preparing* (the background build of cached
pipelines: no stutter) and *ready · n new* (compiled during this session). It never shows a
percentage: how many shaders a game will still reveal is unknown.

### Not taken from Fritchii (yet), and why

- Render-state prediction and recipes (steps 2-4 above): about 5,000 lines plus Python models, and
  a recipe has to be recorded for each game. Possible later; the precompiled-file format and the
  recipe assembly could then be used with recipes made for NEXUS's engine build.
- Fritchii code was not copied into NEXUS. NEXUS's preparation uses Lichii's precache builder (part
  of the engine NEXUS is based on) and NEXUS's own engine interface (`nexus_engine_prepare_shaders`).

## A precache that is skipped keeps its work (0.8.0, engine patch 0028)

The boot precache ("Compiling the game's shaders") builds the precache file's compute programs
only to fill the GPU driver's cache, and a `.done` marker skips it next time. Until 0.8.0 the
marker was written only when the precache ran to the end. *Start now*, or closing the game during
it, threw its work away, and every boot started from zero. On the user's PC that meant 4,671
programs every time (docs/PERFORMANCE.md).

Now the built programs are written to `<precache>.partial`:

- **When:** every 20 s, and when the precache ends without finishing.
- **What it records:** each program's key hash, under the same stamp as the marker (precache
  contents, driver, device, cache version, variant).
- **Next boot:** it leaves these out, and the loading screen counts only the rest.
- **Ignored when:** the stamp differs, or the file is cut short.
- **Removed:** with the precache files, and when a precache finishes.

## Remaining time (0.8.0)

The loading screen and *Prepare Shaders Now* show how long the measured shader work has left
(`app/web/eta.js`). The estimate is the remaining count divided by the throughput measured over
the last 15 seconds of this run; nothing is assumed or carried over from earlier runs.

- Each stage has its own estimate (reading files, converting programs, compiling at boot, building
  cached pipelines): a new stage starts a new measurement, because their speeds differ by orders
  of magnitude.
- Less than 3 seconds and 3 samples of data, or no progress yet: *Calculating remaining time...*
- No progress for 6 seconds: *Waiting: no progress for ...* instead of an old estimate.
- After the preparation: *Shader preparation complete.*
- Stages without a known total (reading the game's files) show no time.
- Pipelines still building in the background while the game starts get an estimate only from
  the counts the engine reports; shaders the game has not asked for are unknown and not counted.

## Caches on disk, and what is portable

| File (per game, in the engine's `shader/<title id>/`) | Holds | Portable to another device? |
|---|---|---|
| `lichii-cache.v2.bin` | Records: guest programs, pipeline keys with their render state and shader environments, which pipelines each session used | **Yes.** No GPU-specific data. Records lowered by another engine build are detected (lowering stamp) and translated again. Framed with CRC-32 and per-record hashes; damaged records are skipped. |
| `lichii-cache.v2.driver.bin`, `vulkan_pipelines.*.bin`, pipeline binaries | Driver-compiled code | **No**, only to the same GPU and driver (the engine drops a driver file with another device identity; the Vulkan driver validates its cache header) |
| `warm-compute.bin` | Compute programs converted from the game files | Rebuilt from the game on each device (it contains the game's code; never synchronised) |
| `warm-compute.done` | The precache marker | No (driver-specific) |

## Moving shader data to another device (0.8.0)

A game's Options menu has **Export Shader Data** and **Import Shader Data…**:

- Export writes a *shader pack* (`<title> shaders [<title id>].nxshaders`) with the live portable
  records of the game's `lichii-cache.v2.bin`: shader programs, graphics and compute pipelines (with
  their render state) and which pipelines each session used. Driver-compiled data is never in it.
  A description records the title, log format, record counts, source device, NEXUS version and a
  SHA-256 of the log.
- Import checks the pack whole before changing anything (format, size against its description,
  SHA-256, log format, and that it is this game). It **adds only records this device does not
  have**: a record present here (even another version) is kept, one removed here stays removed, a
  cut-off tail of the local log is left out, and the old log is kept as
  `lichii-cache.v2.bin.nexus-backup`. The new log is written to a temporary file and renamed. Nothing
  happens while a game runs or shaders are being prepared. Importing the same pack again adds
  nothing.
- On the next start the engine builds the imported pipelines at *Building shaders from earlier
  sessions*, with this device's driver. Pipelines whose render state this GPU cannot use are skipped
  by the engine (`KeyFitsDevice`); programs lowered by another engine build are translated again.

**Why not through the NEXUS Account:** the account keeps its data in a private GitHub Gist, which
holds text files and truncates contents over 1 MB in its API, while a game's shader data is binary
and can be hundreds of MB. More importantly, the portable records contain the game's own shader
code, and NEXUS does not upload game content to the cloud. A cloud-safe version would carry only
pipeline keys and render states (like Fritchii's recipes) and rebuild the code from each device's
own copy of the game; that needs the recipe assembly described above and is future work.

## Not measured

No game was run for this work: there are no before/after numbers for stuttering. To compare on your
PC: play the same route twice, before and after preparing, with the performance bar's *Shaders*
reading on, and note how often it shows *compiling* and the frame time spikes; or set
`EDEN_NX2_STUTTER_TRACE=1` for the engine's per-frame stutter log.
