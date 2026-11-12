# NEXUS for Android (in development, not released)

NEXUS for Android is being developed in this repository: in `android/` and in the shared core
in `app/`. **There is no Android release.** No APK is published. The Windows ZIP does not
contain anything Android-specific. The CI check (`.github/workflows/nexus-android.yml`) builds
the app and tests it, and then discards it.

## What exists today (0.5.0 development)

### Shared core (Go, `app/`): the same code as Windows

| Part | File | State |
|---|---|---|
| Starting NEXUS (data, library, saves, settings, background work, interface server) | `main.go` (`StartCore`) | Shared by Windows and Android |
| Engine host over the NEXUS C interface (ABI v2), loaded with `dlopen` | `enginehost_dl.go`, `enginehost_android.go` | Implemented. It is tested in CI on Linux against a test engine library that implements every export. |
| Mobile engine exports (all optional): `nexus_engine_set_surface`, `nexus_engine_touch`, `nexus_engine_lifecycle`, `nexus_engine_trim_memory` | `enginehost_dl.go` | Host side implemented and tested. The engine side does not exist yet. |
| Device readiness check (minimum and recommended requirements, GPU families) | `platform_mobile.go` | Implemented and unit-tested. Shown in *Settings › System* on Android. |
| Lifecycle: pause a running game when the app leaves the screen, resume it when it returns (unless the user paused it) | `platform_mobile.go` | Implemented. Tested with the stand-in engine. |
| Thermal state, with a warning from *severe* | `platform_mobile.go`, `app.js` | Implemented |
| Memory pressure: drop pending save imports, return memory to the system, tell the engine | `platform_mobile.go` | Implemented and tested |
| JNI entry points (`dev.nexus.android.NexusCore`) | `platform_android.go`, `android_jni.c` | Implemented. CI checks that all 8 are exported from the arm64 library. |
| Games folder in shared storage (`NEXUS/Games`) | `main.go` (`CoreOptions.GamesDir`) | Implemented |

### Android app (Kotlin, `android/`): a thin platform layer

| Part | File | State |
|---|---|---|
| Entry point, the NEXUS interface in a WebView, immersive full screen | `MainActivity.kt` | Implemented. CI builds it. |
| Lifecycle (resume, pause, stop, destroy), memory callbacks and the thermal listener, forwarded to the core | `MainActivity.kt` | Implemented |
| Controllers in the interface (A/B/X/Y/L/R/Home → NEXUS's keys); Back button | `MainActivity.kt` | Implemented |
| Resources and licences copied from the APK on first start or after an update | `BundledFiles.kt` | Implemented |
| Device detection: Android version, SoC, GPU (GL renderer), Vulkan version, RAM, storage, file access, thermal status | `DeviceInfo.kt` | Implemented |
| Storage access for game folders (*All files access*, with an explanation first) | `MainActivity.kt` | Implemented |
| Game surface (`SurfaceView`) passed to the engine; audio focus while a game is in front | `MainActivity.kt` | Implemented on the app side. It needs the Android engine. |
| On-screen controller: sticks, D-pad with diagonals, face buttons, shoulders, −/+, Home; multi-touch; haptics | `ControllerLayout.kt`, `OnScreenController.kt` | Implemented. The layout logic is unit-tested on 5 screen shapes. The engine input path needs the Android engine. |

The app is built for **arm64-v8a only**, with `minSdk` 33 (Android 13) and Vulkan 1.1
required in the manifest.

## Not done yet (the blockers)

1. **The NEXUS Engine for Android (`libnexus_engine.so`). This is the main blocker.**
   - The engine source (the upstream base in `app/engine.lock`) has Android support in its upstream
     tree, but the NEXUS patches have only been built for Windows.
   - Needed:
     - an NDK build of `nexus_engine` for arm64;
     - the four mobile exports above;
     - drawing to the `ANativeWindow` the app passes instead of an SDL window;
     - touch and controller input from NEXUS;
     - the engine's Android audio output.
   - **Until then the app starts, shows NEXUS (library, users, saves, settings, media), and its
     readiness check says that games cannot be started yet.**
2. **Shader and pipeline caches on Android.** They live in the engine data folder (app storage)
   and need the engine.
3. **Controller input inside games.** Android delivers controllers as `InputDevice` key and
   motion events, which the engine has to receive (through NEXUS or through SDL's Android
   backend).
4. **Picking more game folders with the system file picker.** Today games are read from
   `NEXUS/Games` and from folders typed in *Settings › Game library*.
5. **App icon, store listing, signing.** Not wanted until a release is planned.

## Device requirements

The readiness check (`CheckReadiness`) uses these figures. They are also shown on the device.

| | Minimum (below it, games cannot start) | Recommended |
|---|---|---|
| Android | 13 (API 33) | 14 or newer |
| Processor | 64-bit ARM (arm64-v8a) | Snapdragon 8 Gen 2 class or newer |
| Graphics | Vulkan 1.1 | Vulkan 1.3; Adreno 730 or newer |
| Memory | 8 GB | 12 GB; 16 GB for the most demanding games |
| Storage | — (warning below 4 GB free) | |

GPU families (`ClassifyGPU`):

| Family | Level | Reasoning |
|---|---|---|
| Adreno 730+ (Snapdragon 8 Gen 2 and newer) | recommended | Recent Qualcomm drivers, and the open-source Turnip driver, have the most complete Vulkan support for this workload. |
| Adreno 640–72x | supported | Older high-end parts; lighter games only. |
| Adreno 610–63x | limited | Mid-range parts. |
| Older Adreno | unsupported | |
| Mali-G710 and newer, Immortalis | supported (with a warning) | These work, but they have more driver-specific rendering problems. Recent emulator releases still carry Mali-specific fixes. |
| Mali-G77/G78 | limited | |
| Older Mali (Bifrost) | unsupported | |
| Samsung Xclipse, PowerVR, others | unknown (with a warning) | Not tested with NEXUS. |

**Not every Android phone can run NEXUS, and NEXUS does not claim otherwise.** The check
reports exactly what is missing.

## Design decisions

- **One NEXUS, not a second app.**
  - The interface is the same web interface as on Windows, served by the same Go core.
  - Android-specific code is limited to what only Android can do: the activity, the surface, the
    system callbacks and device facts.
  - Everything that can be decided in Go is decided in Go, and tested on every platform. That
    covers readiness, lifecycle policy, memory handling, saves, the library and settings.
- **The same engine interface.**
  - The Android host speaks the same C ABI as the Windows host.
  - Mobile needs are added as *optional* exports, so a Windows engine without them keeps
    working and an Android engine can add them one at a time.
- **The native-execution approach (per-game prepared packs) was studied but not adopted.** This is the approach used by
  projects that run Switch games natively on phones, without emulation.
  - It only runs games that have been specifically prepared, one version at a time.
  - NEXUS is an emulator with a library of the user's own games, so the Android path is the
    engine built for Android.
  - None of that code is used in NEXUS.
- **iOS.** Emulation on iOS needs JIT, which Apple restricts outside the App Store's rules. A
  NEXUS iOS app is not technically realistic now. The shared core does build for other
  platforms, but nothing iOS-specific exists or is planned.

## Building (development)

Requirements: Android SDK (API 35), NDK (r26 or newer), JDK 17, Go 1.24 and Gradle 8.14.

```
export ANDROID_NDK_HOME=/path/to/ndk
gradle -p android :app:testDebugUnitTest :app:assembleDebug
```

`:app:buildNexusCore` runs:

```
GOOS=android GOARCH=arm64 CGO_ENABLED=1 CC=<ndk>/aarch64-linux-android33-clang go build -buildmode=c-shared
```

It packages the result as `libnexus_core.so`.

The debug APK is for development devices only.
