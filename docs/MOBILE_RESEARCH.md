# Mobile research: Android (Banana Native) and iOS (nxnative)

**Status: research only.** No Android or iOS code exists in NEXUS, and none is being written. Nothing
here is a commitment or a release plan. Windows NEXUS is not affected by anything in this document.

Sources (read November 2026):
- Banana Native README, on its git.eden-emu.dev mirror, the project's temporary home while its
  GitHub repository is unavailable;
- the Banana Native release notes;
- nxnative's README and press coverage.

The Banana Native source tree itself could not be cloned from this environment. Statements about
its internals come from its README and release notes, not from reading its code.

## 1. What Banana Native is

- **Not an emulator.** Banana Native describes itself as working like Wine: the game's own ARM64
  code runs directly on the phone's CPU. A host supplies the system services the game calls, and
  the game's graphics (NVN, including the Switch 2's task and mesh shaders) are translated to Vulkan.
- **Origin.** It is an Android port of **nxnative** by seedds, which does the same on Mac, iPhone and
  iPad with Metal. It is unofficial and experimental, and not affiliated with Nintendo, nxnative or
  Eden.
- **Per-game support.** Each game is converted once into a **pack**: code rewritten for Android,
  shaders translated for Vulkan. This takes about an hour on the phone, or less on a PC or Mac with
  the project's scripts (which install about 400 MB of tools). The README lists five games:
  - Donkey Kong Bananza, Mario Kart World, Pokémon Pokopia (supported);
  - The Legend of Zelda: Ocarina of Time (experimental, visual glitches);
  - Splatoon Raiders (boots, not yet playable).

  Each is tied to a specific game version.
- **Input format.** Extracted dumps (`base/`, `updates/<version>/`, `dlc/`, each with `exefs/` and
  `romfs/`, or a `.nup` of that folder). **NSP and XCI files are not accepted.**
- **Requirements** (from the README):
  - Android 13 or later;
  - a Snapdragon (Adreno) GPU;
  - at least 12 GB of RAM (16 GB for Mario Kart World);
  - *All files access*.

  It was tested on one phone, with the Qualcomm system driver and with Turnip.
- **App features worth noting:** a *Readiness check* in settings, log sharing with a short digest of
  the likely cause, an in-app updater, and frame generation.

## 2. Licences

| Part | Licence (as stated by the project) |
|---|---|
| Banana Native Android app as a whole | **GPL-3.0-or-later** (`android/NOTICE.md`), because it includes frame generation ported from Eden / lsfg-vk and task/mesh-shader lowering from switch2mac / Eglantine |
| Files from nxnative | **MIT** (repository `LICENSE`) |
| Shader translator from Ryujinx (`dm/third_party/ryujinx`) | **GPL-3.0-or-later** |
| Fixes derived from nx2android | **MIT**, credited in `android/NOTICE.md` |
| Other third-party code (`dm/third_party`) | Each keeps its own licence |

**Compatibility with NEXUS.**
- NEXUS's own code and the NEXUS Engine are distributed under the GPL (see
  `THIRD_PARTY_NOTICES.md`). GPL-3.0-or-later and MIT code can be combined with it.
- Reusing Banana Native code would mean keeping every notice, keeping the combined work under
  GPL-3.0-or-later, and providing source for any APK. That is no different from NEXUS's existing
  obligations for the engine.
- Nothing has been copied.

## 3. How this differs from NEXUS on Windows

| | NEXUS (Windows) | Banana Native / nxnative |
|---|---|---|
| Method | System emulation (engine from the Lichii / Eden / yuzu lineage, JIT for ARM code) | No system emulation: native ARM64 execution, plus a service host and graphics translation |
| Game support | Any game the engine runs, from NSP/XCI/NSZ/NUP/NRO | Only games with a profile and a prepared pack, for specific versions |
| Keys/firmware | The user's own keys and firmware | Banana Native includes neither. nxnative needs the user's `prod.keys` to build an app |
| Preparation | None | One-time per game (an hour on a phone) |

**Consequence:** a NEXUS for Android based on Banana Native's approach would not be "NEXUS with a
different engine". Its library would contain only prepared, supported games. The NEXUS interface,
users, saves, media and settings could be shared, but the game pipeline would be new.

The alternative for Android is the emulation path that Eden's Android build uses: the same engine
lineage as the NEXUS Engine, with Vulkan on Android. That keeps NEXUS's broad game support, at
lower performance than native execution.

## 4. Designs recorded for a possible future (not implemented)

### Device detection
- **Collected:**
  - Android version (API level);
  - SoC and GPU vendor/model, from `Build.SOC_MODEL` and Vulkan `VkPhysicalDeviceProperties`;
  - Vulkan version and extensions: `VK_EXT_mesh_shader`, as Banana Native's release notes show its absence matters;
  - total RAM (`ActivityManager.MemoryInfo.totalMem`);
  - free storage;
  - thermal status (`PowerManager.getCurrentThermalStatus`).
- **Same model as Windows.** NEXUS's hardware panel and "Optimise for my device" would use the same model as on Windows: detect, then offer only what works.

### Readiness check
Each item would be shown as pass / warning / fail, with the reason:
- the OS version;
- the GPU family;
- the required Vulkan extensions;
- RAM against the per-game minimum;
- free space for packs and shader caches;
- storage permission;
- keys and firmware present (emulation path only).

### Preparation workflow (native path)
1. Choose a dump.
2. NEXUS identifies the game and version and checks it against supported profiles.
3. If it is unsupported, say so; never "try anyway" silently.
4. Prepare the pack: on the device (resumable, while charging) or on a NEXUS for Windows PC, then transfer it.
5. Verify the pack.
6. Add the game to the library.

Saves would use the NEXUS layout (`<PLAYER_ID>-<name>\<GAME_ID>-<SHORT>`), so they could move
between devices with NEXUS's existing export/import.

## 5. iOS / nxnative assessment
- **How nxnative works.** It turns a game the user owns (NSP plus `prod.keys`) into a standalone
  IPA, or a Mac `.app`, with the graphics rebuilt on Metal. Reported requirements for iPhone and iPad:
  - iOS/iPadOS 26 or later;
  - an A17 Pro (iPhone 15 Pro) or newer chip, or an M1 or newer iPad.
- **Distribution.** Apps are sideloaded or signed with the user's own Apple developer team.
  - The App Store does not allow this kind of app.
  - JIT, which emulation needs, is restricted on iOS. A native-execution approach avoids JIT, which is why it is feasible there at all.
- **For NEXUS** this would mean a build tool that produces one app per game (as nxnative does), not
  a single NEXUS app with a library.
  - It would need a Mac with Xcode to build.
  - Each user would sign it with their own Apple account.
  - Users would need to understand and accept sideloading.
- **Legal and policy risk.** Press coverage reports Nintendo DMCA takedowns of videos of similar
  native ports. Any NEXUS involvement would need the same strict rules NEXUS already follows:
  - no games, keys or firmware are distributed;
  - users supply their own.

## 6. Recommendation
- **Nothing should start before NEXUS for Windows is stable at 1.0.**
- **If mobile is pursued**, evaluate the two Android paths with a small prototype outside the NEXUS
  repository first:
  - **emulation:** broad support, needs a capable phone;
  - **native packs:** few games, excellent performance.
- **iOS is only realistic** as an nxnative-style per-game build tool with user-side signing.
- **Before anything is announced**, the shared parts (interface, users, saves, media) could be checked for touch input and phone-sized layouts, without adding any platform build to this repository.
