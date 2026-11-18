# NEXUS on Linux and the Steam Deck

## One build for every AMD64 Linux PC and the Steam Deck

The Steam Deck runs SteamOS, which is an AMD64 Linux. It gets **the same AMD64 build** as every
other AMD64 Linux PC: the same files, from the same CI run, with the same SHA-256. Nothing is
compiled specially for the Deck. At run time NEXUS notices Gaming Mode (see below) and only
changes how the window behaves.

| File | For |
|---|---|
| `NEXUS-<v>-Linux-amd64.AppImage` | AMD64 Linux PCs and the Steam Deck (recommended) |
| `NEXUS-<v>-Linux-amd64.tar.gz` | AMD64 Linux PCs with WebKitGTK 4.1 installed |
| `NEXUS-<v>-Linux-arm64.AppImage` | ARM64 Linux (aarch64) |
| `NEXUS-<v>-Linux-arm64.tar.gz` | ARM64 Linux with WebKitGTK 4.1 installed |

Run `uname -m` to check: `x86_64` means AMD64 and `aarch64` means ARM64.

## How it is built (`.github/workflows/nexus-linux.yml`)

1. **Engine.** Upstream Lichii (`app/engine.lock`) plus `engine/patches` is built natively on
   `ubuntu-24.04` (AMD64) and `ubuntu-24.04-arm` (ARM64). It is built as `nexus_engine.so`
   (`-DNEXUS_ENGINE=ON`, no Qt, position-independent). The engine's unit tests and the ABI smoke
   test run on both.
   - The Linux engine has its own copies of the libraries whose versions differ between
     distributions:
     - SDL 3.4.20 with libusb;
     - FFmpeg, static, with only the H.264, VP8 and VP9 decoders and no VA-API or VDPAU.
   - It links only libraries every desktop and SteamOS has: glibc, libstdc++, libudev and
     libdrm. Graphics and windowing libraries (X11, Wayland, Vulkan) are loaded at run time.
   - libusb-1.0, OpenSSL 3 and lz4 are copied next to the engine in both packages, with their
     licence texts in `Licences/linux-libraries`.
   - CI fails if the engine links FFmpeg, Boost, SDL or another version-specific library from
     the system.
   - Known gap: no AV1 decoder, so Switch 2 games' AV1 movies do not play on Linux yet. The
     Windows engine has one.
2. **NEXUS.** `go build -tags webkitgtk`. The window is GTK 3 with WebKitGTK 4.1
   (`app_linux.go`). The engine is loaded with dlopen (`enginehost_linux.go`, the same loader as
   Android).
3. **Packages** (`tools/release/make_linux.py`):
   - **tar.gz:** the `NEXUS/` folder. The engine's libraries have an `$ORIGIN` RUNPATH. It needs
     the system's WebKitGTK 4.1.
   - **AppImage:** built with linuxdeploy and its GTK plugin. WebKitGTK's helper processes,
     GStreamer and GDK-pixbuf loaders are inside. `AppRun` clears `LD_LIBRARY_PATH` and points
     GStreamer, GSettings and pixbuf at the bundled copies. The engine is copied in last and
     checked against `Engine/engine.json`.
4. **Checks before upload:**
   - the tar.gz is extracted fresh: files present, RUNPATH, no missing libraries in `ldd`, and
     `--engine-check`;
   - the AppImage is executable and passes `--engine-check`;
   - `tools/release/linux/smoke.sh` starts it under Xvfb on the runner and in clean
     `ubuntu:24.04` and `archlinux` containers that have no WebKitGTK installed;
   - each start must reach Home or the profile picker and save a screenshot;
   - the second run uses `SteamGamepadUI=1 SteamDeck=1` and must log "Gaming Mode true".

ARM64: the engine's unit tests run on ARM64 except `[jit_codegen_options]`. That test compares
dynarmic's x86-64 code generation and aborts in dynarmic's ARM64 backend; upstream runs it on
x86-64 only. Games on ARM64 Linux use that backend and are less mature than on AMD64. No game has
been tested on ARM64 here.

Requirements: glibc 2.39 or newer, a Vulkan driver (Mesa RADV, NVIDIA) or OpenGL 4.6.

## Data

- Settings, users, saves and logs are in `~/.local/share/NEXUS` (`$XDG_DATA_HOME/NEXUS`).
  `NEXUS_DATA` overrides this.
- A `portable.txt` file next to the `nexus` binary keeps everything in the `NEXUS/` folder.
- Games folder in the AppImage: `~/.local/share/NEXUS/Games`. In the tar.gz: `NEXUS/Games`.
- Files are created with the user's umask. NEXUS never needs root.

## Steam Deck: Gaming Mode

NEXUS treats the session as Gaming Mode when any of these is true:
- `SteamGamepadUI=1`;
- `SteamDeck=1` and the desktop is not KDE;
- `XDG_CURRENT_DESKTOP` contains `gamescope`;
- `GAMESCOPE_WAYLAND_DISPLAY` is set.

In Gaming Mode:
- the window always opens full screen with no window decoration;
- F11 does nothing (gamescope owns the display);
- NEXUS starts no window manager tricks.

The game runs in the engine's own SDL window on top. When the game closes, NEXUS gets focus
again (`app.focus`) and shows Home.

Add NEXUS to Steam:
1. In Desktop Mode, put the AppImage somewhere permanent and make it executable.
2. Right-click it › *Add to Steam*.
3. Start it from the library in Gaming Mode.

Steam Input presents the Deck's controls (and any paired controller) as a standard gamepad. The
engine reads that through SDL. Keep Steam Input enabled for NEXUS.

## Updates

On Linux, NEXUS checks for new versions and shows them. Its button opens the release page; it
does not replace its own files. AppImages can be in read-only places, and the tar.gz may be
packaged by a distribution.

## Switch 2 controllers

| Controller | Linux status |
|---|---|
| Switch 2 Pro Controller over **USB** | Experimental. SDL 3.4.20 with libusb (engine patches 0031/0032, from Lichii build-341). Needs the optional udev rule `72-nexus-switch2.rules` (shipped, never installed by NEXUS). Not yet tested on hardware. |
| Switch 2 Pro Controller over Bluetooth | Not supported on Linux. Lichii's Bluetooth backend uses WinRT and is Windows-only. |
| Joy-Con 2 (L, R, pair) | Not supported. |
| Any controller through Steam Input (Gaming Mode) | Works the same as Steam's own handling of that controller. |

The Bluetooth controllers need more than NEXUS should do without asking.
[jc2mouse](https://github.com/coffincolors/jc2mouse) (MIT), the reference for Linux, needs:
- root for uinput and BlueZ control;
- on some systems, a patched `bluetoothd` that replaces the system Bluetooth service.

NEXUS will not install kernel modules, replace Bluetooth components or create virtual input
devices behind the user's back.

## Frame generation on Linux

**Built in (any Vulkan GPU).** The engine's own optical-flow interpolation: vendor-neutral Vulkan
compute shaders, not AMD FidelityFX code. It is built for Linux the same as Windows and needs
only Vulkan. While a game runs, NEXUS shows the engine's real state: running, or why it is not
(engine patch 0033).

**NVIDIA Smooth Motion (`smoothmotion.go`).** The NVIDIA Linux driver's own frame generation
for Vulkan programs.
- Requirements: driver 575 or newer, a GeForce RTX 40 or 50 card, and the driver's implicit
  layer `VK_LAYER_NV_present`.
- NEXUS offers it as a frame generation method only when it finds all three. For such a session
  it sets `NVPRESENT_ENABLE_SMOOTH_MOTION=1` in its own process before the engine starts, and
  turns the built-in method off; NVIDIA says the two must not be combined.
- The layer does not report to the program, so NEXUS shows "requested", never "running".
- Not tested on an RTX card here.

**Lossless Scaling (lsfg).** Needs the user's own `Lossless.dll`, as on Windows.

Not used by NEXUS:
- AMD Fluid Motion Frames is a feature of AMD's Windows driver; Mesa (RADV) has nothing like it.
- [afmf-linux](https://github.com/serialexperimentslainnnn/afmf-linux) (MIT, community) is an
  implicit Vulkan layer that does something similar on RDNA 3 and 4 cards. A user who installs it
  can use it with NEXUS like with any Vulkan program. NEXUS does not install it, and the Steam
  Deck's GPU (RDNA 2) is not its target.
- DLSS Frame Generation and FSR 3 frame generation need the game's motion vectors, which an
  emulator does not have.

On the Steam Deck the choices are the built-in method or Lossless Scaling with the user's own
`Lossless.dll`.
