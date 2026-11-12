# NEXUS architecture

NEXUS is released for **Windows x64 only**. This document describes how the code is divided so
that the platform-specific parts are small and replaceable. Android is being developed in the same
repository (`android/`, [ANDROID.md](ANDROID.md)) and is not released.

## Components

```
NEXUS.exe (Go, app/)
├── Local HTTP API + event stream  ─────────►  Interface (app/web: HTML/CSS/JS in WebView2)
├── Library, users, saves, media, settings, updates      (portable Go)
├── EngineHost interface (enginehost.go)
│     ├── Windows: nexus_engine.dll (LoadLibrary, enginehost_windows.go)
│     └── Android: libnexus_engine.so (dlopen, enginehost_dl.go; engine not built yet)
├── captureBackend interface (recorder.go)
│     └── nexus_capture.dll — screen recording (capture/, C++20, Windows APIs only)
└── platform files *_windows.go / *_other.go
```

| Layer | Portable | Platform-specific today |
|---|---|---|
| **Interface** (`app/web`) | Yes: plain HTML/CSS/JS served by NEXUS, scales from 1280×720 up, works with keyboard, mouse and controller | Hosted in WebView2 (`app_windows.go`) |
| **Core** (library, users, saves, media, settings, updater, artwork) | Yes: Go, standard library, no cgo | Paths under `%APPDATA%\NEXUS` |
| **Engine host** (`EngineHost`) | The interface and the request/status JSON | Loading a DLL (`enginehost_windows.go`) |
| **Engine** | Upstream is cross-platform C++ | Built for Windows x64 only (CI) |
| **Capture** (`captureBackend`) | The Go side (settings, files, Media, recovery, events) | `nexus_capture.dll`: Windows Graphics Capture, Media Foundation, WASAPI |
| **Overlay, hardware detection, Recycle Bin, secrets, signature checks** | Interfaces in Go | Win32, DPAPI and WinVerifyTrust in `*_windows.go` |

Every `*_windows.go` file has a `*_other.go` counterpart. The `*_other.go` files are stubs, or
the stand-in used by the tests, so the whole application builds and its full test suite runs on
any OS (CI runs it on Linux and Windows). That is what keeps the Windows-specific code contained.
It is not a port.

## Engine boundary

- `nexus_engine.h`: C ABI version 2. New features are *optional exports* that NEXUS looks up by
  name:
  - 0.4: reveal, controllers;
  - 0.5: background, screenshot, window.
- A missing export disables only its feature, so app and engine can move independently.
- The engine never shows a user interface of its own and never asks the user to install
  anything. NEXUS starts it in-process, gives it its data folder, and talks to it only through this
  ABI.
- See `NEXUS_ENGINE.md` and `docs/SOURCE_AUDIT.md` (what is upstream-derived, and the staged plan
  for replacing it).

## Capture boundary

- `capture/include/nexus_capture.h` (API v1) defines these calls:
  - `nxcap_capabilities` (encoders, window capture, per-process audio);
  - `nxcap_start` (key=value settings);
  - `nxcap_status`;
  - `nxcap_stop`;
  - `nxcap_selftest`.
- On the NEXUS side, everything else lives in portable Go (`recorder.go`):
  - choosing what the PC can do;
  - file naming;
  - `.part` recovery;
  - Media integration;
  - events and the interface.
- Another implementation of `captureBackend` would only need to provide those five calls.

## What a port would involve (engineering notes)

These are the parts that would need new implementations. They are recorded so the Windows code
does not grow in ways that make them harder:
1. **App window.** A web view host instead of WebView2.
2. **Engine build.** An engine build and CI job for the target, with the same ABI tests.
3. **Capture backend.** Another `nxcap_*` implementation.
4. **Platform files.** The `*_windows.go` features: hardware detection, overlay, Recycle Bin, credential storage, signature checks.
5. **Packaging and signing** for that platform.

Android is the first platform built on this division: its app (`android/`) starts the same core (`StartCore`), loads the engine through the same C interface with `dlopen` (`enginehost_dl.go`), and adds only what Android itself requires. See [ANDROID.md](ANDROID.md).

## Rules that keep it portable

- **No new Windows-only calls in shared files.** They go behind an interface, in a `*_windows.go` file with a `*_other.go` counterpart.
- **No paths with `\` in shared code.** Use `filepath`. User-visible Windows paths belong in interface text only.
- **The engine and capture ABIs only grow by optional exports.** The version constant changes only for incompatible changes.
- **No platform build systems or packages are added** until a platform is actually being developed.
