# Platforms

| Platform | Status | Download |
|---|---|---|
| **Windows x64** (10 and 11) | Available (preview) | [Releases](https://github.com/0ryzin/Nexus/releases/latest) |
| **Linux** (x86-64, including Steam Deck) | Planned next — not started | None yet |
| **macOS** (Apple Silicon) | Planned after Linux — not started | None yet |

Only Windows builds are published. Release pages list Linux and macOS under *Other platforms* as
"not available yet" and never link a download that does not exist.

Roadmap: **Windows now → Linux next → macOS later.** Nothing below has been implemented; these are
investigation notes so the work can be planned.

## Linux (next)

**What has to change**
* **App window.** NEXUS's window on Windows is WebView2. On Linux the same web UI would run in
  WebKitGTK (webkit2gtk-4.1), or in a browser window opened on the local NEXUS server. The Go code
  is already split into `_windows.go` / `_other.go` files; the Linux side currently has stubs.
* **Engine.** The upstream engine (Lichii / Eden lineage) builds on Linux with GCC or Clang. The
  NEXUS patches would need a Linux CI job producing `libnexus_engine.so`, the same ABI and tests.
* **Platform features** to reimplement: hardware detection (`/proc`, `/sys`, `vulkaninfo` or
  Vulkan calls), the performance overlay (no layered Win32 window — an in-engine overlay or a
  Wayland/X11 layer is needed), XInput Guide polling (SDL already covers this in the engine),
  single-instance and the data folder (`$XDG_DATA_HOME/NEXUS`).

**Packaging options**
* **AppImage** — one file, runs on most distributions, works on Steam Deck desktop mode and can be
  added to Steam as a non-Steam game. The usual choice for emulators. Needs bundling of
  WebKitGTK and the engine's libraries (or relying on the host's).
* **tar.gz** — a plain folder like the Windows ZIP; simplest, but the user needs the system
  libraries.
* Flatpak is a later option (sandbox needs filesystem permissions for game folders).

**Steam Deck**
* Run as a non-Steam game in Game Mode. Steam takes the Steam/Guide button for itself; NEXUS Home
  would need a different binding there (for example a back button or a button combination), which
  NEXUS already lets users choose.
* Steam Input should be left on; the engine sees the Deck controls as a standard gamepad via SDL.
* The UI must scale to 1280×800 (it already works at small sizes).

## macOS (later)

* **Graphics:** the engine's Vulkan renderer runs on macOS through MoltenVK; upstream support is
  experimental. Apple Silicon only would be the realistic target.
* **App window:** WKWebView.
* **Packaging:** a signed `.app` inside a **DMG**. Distribution outside the App Store needs an
  Apple Developer ID (paid yearly membership) for code signing and **notarisation**; without it
  Gatekeeper blocks the app. Hardened runtime and JIT entitlements are needed for the engine.
* No macOS build will be published until it is signed, notarised and tested.
