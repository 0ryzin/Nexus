LICENCES AND ACKNOWLEDGEMENTS
=============================

NEXUS - GNU General Public License v3.0 or later (NEXUS-GPL-3.0.txt)
  NEXUS.exe loads the GPL-licensed NEXUS Engine into its own process, so NEXUS as a whole is
  distributed under GPL-3.0-or-later. Source: Source\NEXUS-source.zip.
  NEXUS is not affiliated with or endorsed by Nintendo. "Nintendo Switch" is a trademark of
  Nintendo. Resources\ProfileIcons and some artwork in Resources\ArtworkDB are Nintendo-derived
  artwork supplied by the owner for personal use only - see the NOTICE.txt files there.

NEXUS Engine (Engine\nexus_engine.dll) - GNU General Public License v3.0 or later
  (engine-GPL-3.0.txt)
  The NEXUS Engine is built from the Lichii emulator source (https://github.com/zortos293/Lichii,
  the base named in Engine\engine.json and Source\engine) plus the NEXUS patch series. Lichii is
  based on Eden, Eglantine, yuzu and Sudachi; the engine therefore still contains code from
  those projects, whose copyright notices are kept in the source files. Component licences of
  the engine source tree (dynarmic, Boost, SDL, fmt, nlohmann/json, Vulkan headers, FFmpeg and
  others): engine-third-party\.
  Corresponding source: Source\engine (see Source\README.txt).

NEXUS screen-recording module (nexus_capture.dll) - GPL-3.0-or-later, part of NEXUS
  Original NEXUS code. It uses Windows components only (Windows Graphics Capture, Direct3D 11,
  Media Foundation, WASAPI) and contains no third-party source code.

Ryujinx - MIT (Ryujinx-MIT.txt)
  Used as a technical reference (for example, the layout of save metadata that NEXUS reads
  when importing saves). No Ryujinx program is included or started by NEXUS. The licence is kept
  for attribution of any Ryujinx-derived code.

Libraries compiled into NEXUS.exe
  github.com/jchv/go-webview2   MIT                       go-webview2-MIT.txt
    includes WebView2Loader.dll  Microsoft BSD-style       Microsoft-WebView2Loader-SDK.txt
  github.com/jchv/go-winloader  ISC                       go-winloader.txt
  golang.org/x/sys              BSD-3-Clause              golang-x-sys-BSD.txt
  Go standard library           BSD-3-Clause              Go-BSD.txt

Fonts
  Inter (Rasmus Andersson) - interface        SIL Open Font License 1.1  Inter-font-OFL-1.1.txt
  Outfit (The Outfit Project Authors,
  github.com/Outfitio/Outfit-Fonts) - startup wordmark
                                             SIL Open Font License 1.1  Outfit-font-OFL-1.1.txt

NEXUS includes and downloads no games, keys or firmware.
