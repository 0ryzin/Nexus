# Lossless Scaling frame generation: setup

NEXUS's frame generation can use the shaders of the user's **own** Lossless Scaling. That is the
paid Steam app 993090. The engine reads them from `Lossless.dll` in `<NEXUS data>\System\Engine\lossless`
(`video_core/frame_gen/lossless_dll.cpp`). NEXUS never downloads, bundles or shares that file. It
only copies it from the user's installation on the same PC (`app/lossless.go`).

## Finding it (no default paths assumed)

1. Steam's folder:
   - from the registry: `HKCU\Software\Valve\Steam\SteamPath`, then `HKLM\…\Valve\Steam\InstallPath`;
   - then the usual folders, in case the registry is missing.
2. Every Steam library, from `steamapps\libraryfolders.vdf`, so other drives are included.
3. In each library, `appmanifest_993090.acf` names the app's install folder. NEXUS looks for
   `Lossless.dll` there.
4. **Locate Lossless Scaling installation…** takes any of these and finds the DLL in it:
   - the Lossless Scaling folder;
   - a Steam library;
   - Steam itself.

## Checking it before anything is copied

The checks follow the way the engine reads the file:

- It must be a Windows PE file.
- Its RCDATA resources must hold every shader the engine needs, as SPIR-V modules: ids 255, 256
  and 280–302, at the native FP16 offset +49.
- The DLL is never loaded or run, so its CPU architecture does not matter. It is shown (x64 for
  current versions).
- An identical file (SHA-256) already in NEXUS is not copied again.
- A different valid file replaces the installed one **only when the user chooses** *Update the
  copy in NEXUS*. A working file is never overwritten silently.
- A cached translation of an older DLL is removed when the DLL changes.

## States shown

| State | Message |
|---|---|
| ready | Lossless Scaling integration ready (with the file's details) |
| found | Lossless Scaling detected (with its path, and *Set up Lossless Scaling*, which copies the file) |
| update | Ready; a different version is in your Lossless Scaling (optional update) |
| notfound | Lossless Scaling files not found, with the Steam libraries searched and *Locate…* |
| invalid | The file found cannot be used, and why (not a DLL, shaders missing) |
| unsupported | Frame generation needs a Vulkan GPU the engine can use |

## On the user's PC (17 Nov, read only)

Steam is at `C:\Program Files (x86)\Steam`, with one library. Lossless Scaling (993090) is not
installed in it. The new setup screen therefore shows "Lossless Scaling files not found", names
that library, and offers *Locate…* for a copy installed elsewhere.

## Tested

- `lossless_test.go`:
  - Steam's folder supplied by the test instead of the registry;
  - a libraryfolders.vdf with two libraries, and the app manifest's install folder;
  - PE resource check with good, missing and damaged shaders;
  - no copy when identical, update only when asked.
- **Not tested** with a real Lossless.dll: none is installed on the test PC.
