# Android — feasibility note (research only, 0.9.2)

- **Engine:** yes. Lichii (and so the NEXUS engine's base) has an Android front end
  (`src/android`, Kotlin + JNI, Vulkan, Adreno/Mali/Turnip drivers, the LSFG quick setting).
  The engine code runs on arm64 Android today.
- **NEXUS:** the front end is Go + a system WebView (WebView2 on Windows, WebKitGTK on Linux)
  loading the engine in-process. Android has a WebView and Go builds for android/arm64 (NEXUS
  already has an "android (development check)" CI build), but the window, input (Android
  gamepad events), file access (Storage Access Framework), process model and packaging (APK,
  JNI bridge to the engine) are all different: a new platform layer, not a port of a flag.
- **Hardware:** Switch 2 titles need a recent flagship (Snapdragon 8 Gen 2+) and 12 GB+ RAM;
  Switch 1 titles are realistic on mid-range devices.
- **Compared with macOS:** macOS needs MoltenVK (Vulkan on Metal) and has no supported Switch 2
  path in the upstream projects; Android has an existing upstream front end to learn from.
  Android looks the more realistic next platform.
- **Recommendation:** a separate project after 1.0, starting from the engine's Android front end
  with NEXUS's library/profiles/sync logic as a shared Go core. Not started; nothing in the
  Windows/Linux architecture was changed for it.
