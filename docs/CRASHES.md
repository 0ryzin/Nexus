# Crashes: what NEXUS records, and the Ocarina of Time save-load crash

## Why a crash used to leave nothing behind

The engine runs inside the NEXUS process (`nexus_engine.dll`), so a native crash in it ends NEXUS
too. Before 0.8.0 such an end left no trace at all:

- The Go runtime turns off the Windows error dialog and Windows Error Reporting for the whole
  process (`SetErrorMode(SEM_NOGPFAULTERRORBOX)` and `WER_FAULT_REPORTING_NO_UI` in
  `runtime/signal_windows.go`), so no "stopped working" box appears and no
  `%LOCALAPPDATA%\CrashDumps` dump is written, even when LocalDumps is configured.
- `nexus_engine` never installed the engine's crash handler. Lichii's own front-ends do
  (`Common::Windows::InstallCrashHandler` in `lichii_cmd` and `qt_common`).
- A Go fatal error is printed to stderr, which a GUI program does not have.

The engine log simply stops, and `nexus.log` has no "session ended" line.

## What NEXUS records now

| Where | What | When |
|---|---|---|
| `Engine\log\eden_log.txt` | `Common <Critical> Host crash: unhandled exception 0x… at module+offset on thread N "name", info …; stack: module+offset …`, then the log is flushed | Engine patch 0027: any unhandled exception, `abort()` (failed fatal asserts, `std::terminate`) or CRT invalid-parameter failure |
| `Engine\log\eden_crash.dmp` | Minidump with thread information (`MiniDumpWithThreadInfo`); `EDEN_NX2_CRASH_DUMP=<dir>` puts it elsewhere | Same |
| `%LOCALAPPDATA%\CrashDumps\NEXUS.exe.*.dmp` | Windows Error Reporting dump, if LocalDumps is set up on the PC | NEXUS turns WER back on without its dialog (`debug.SetTraceback("wer")`) |
| `Logs\crash-go.log` | Go's own fatal error output with stacks | `debug.SetCrashOutput` |
| `Logs\session.json` | The running session: game and title ID, phase and stage, engine state, load stage and counts, frames, shader preparation, shader counts, GPU and driver, the session's main renderer settings, NEXUS and engine versions | Written when a game starts, on every phase or stage change and at most every 3 s otherwise (a few hundred bytes); removed when the session ends normally or NEXUS is closed on purpose |
| `Logs\crashes\crash-<time>.txt` (+ `.dmp`) | The report: the session record, the engine's `Host crash` lines, the dumps found, Go's crash output, the last 25 engine error lines, the last 60 engine log lines and the session's last NEXUS log lines | At the next start of NEXUS, when `session.json` from another process is still there |

At that next start NEXUS says once that it closed unexpectedly, during which phase, and offers
**Open diagnostic log** (the report) and **Technical details** (the `Host crash` line).

What a report never contains: keys, key file names, account names or e-mail addresses, tokens, or
the Windows user name. Lines mentioning tokens, passwords, accounts or key files are left out, the
home folder is written as `%USERPROFILE%`, and e-mail addresses are replaced. The engine log itself
already writes `C:\Users\user` for the home folder.

Nothing here runs while a game plays except the small session record; the engine's handler only
runs when a crash happens.

### Reading a report

- A `Host crash` line names the faulting module and offset. `nexus_engine.dll+0x…` is in the
  engine: open `eden_crash.dmp` in WinDbg or Visual Studio with the build's PDB (CI artifact) for
  the full stack.
- No `Host crash` line and nothing in `crash-go.log`: a fail-fast the handler cannot catch (stack
  overflow, `__fastfail`), or the process was ended from outside (Task Manager, Windows when memory
  or commit ran out). The Windows event log (*Application*, source *Application Error* or
  *Windows Error Reporting*) then has the exception code.

## Ocarina of Time: NEXUS closes after "Load existing save" (17 Nov 2026)

Reported: the menu runs at about 150 FPS; loading an existing save gives a black screen and NEXUS
closes completely, with no message. About 500 shaders were still being prepared when the game
started.

### What the logs show (the user's PC, NEXUS 0.7.0, engine 0.8.0 on build 316)

- `nexus.log`: shader preparation for `0400486024DEC000` done in 14.6 s (2,916 archives, 4,794
  compute programs, 18 skipped), the engine started, and then nothing: no "session ended", no
  error. The whole process ended.
- `eden_log.txt` ends at 666.7 s, during the save load. Right before the end, from 665.52 s, are
  14 `Unmapped Device ReadBlock` errors from `buffer_cache.h:4005`, three of them over ranges that
  were wholly unmapped (2.5 MB, 4 MB, 1.6 MB). The last line is a play report (`prepo`) whose
  process ID is not a process, which falls back to the application and is harmless.
- No `Host crash` line, no `eden_crash.dmp`, no `NEXUS.exe` dump in `CrashDumps` (that folder has
  WER dumps of other programs, so LocalDumps is on). That matches the missing handler and Go's
  `SEM_NOGPFAULTERRORBOX`, and rules out nothing about the crash itself.

### Cause: what is known and what is not

The exact faulting instruction is not known: no dump was written. The most likely cause is a
defect Lichii fixed after build 316, in code that was active at that moment:

- **Lichii #2041 (patch 0025).** The device and GPU memory tables (`DeviceMemoryManager`'s
  `compressed_device_addr` and `tracked_entries`, the GPU `MemoryManager`'s `big_page_table_dev`,
  `Common::PageTable`) are `SparseLargeVector`s: reserved address space whose pages are committed
  on demand. On Windows a vectored handler (`FakePageFaultHandler`) commits a page when something
  *reads* an uncommitted one. It committed it read-only, and when the page was already committed
  it called `VirtualProtect(PAGE_READONLY)` on it. A read of unmapped device memory (exactly the
  burst in the log, while the save loads a scene) can race with a thread that has just committed
  the same page read-write and is storing entries in it (mapping the new scene's memory): the
  handler takes write access away, the store faults, and the handler does not handle write faults.
  The access violation is unhandled and, before 0.8.0, ended the process silently. Windows only,
  timing-dependent, and only when unmapped reads happen, which fits a crash at the save load and
  never in the menu.
- **Lichii #2062 (patch 0026).** The texture cache removed an image's pages from the page table
  of the channel bound at the time instead of the one the image was registered in. Ocarina of Time
  uses more than one GPU channel; the stale entries can point at freed images later.

### Why the menu works and the save load does not

The menu draws a small, stable set of resources. Loading a save loads a scene: new GPU memory is
mapped, buffers and textures are created, and the game's renderer reads memory that is not mapped
yet (the unmapped reads). That is the only point in this session where mappings change quickly
while other threads read them.

### What shader preparation had to do with it

Nothing found. Preparation finished 10 minutes before the crash (it ran before the engine
started); the remaining shaders compile on worker threads, and none of the last log lines is a
shader build or a pipeline error. The "500 remaining" were pipelines still being built in the
background, which is normal. Shader preparation stays on.

### Tested and not tested

- Engine: patch 0025 adds a test (`src/tests/common/sparse_large_vector.cpp`) for a page committed
  by one thread and faulted in by another; it runs in the Windows CI `tests.exe`.
- NEXUS: the session record and report (`crashreport_test.go`), the remaining-time estimates
  (`tools/uitest/eta_test.js`, `shader_progress.py`).
- **Not tested: loading the save on the user's PC.** That needs the game, the save and the PC. The
  crash is not claimed fixed until that has been done. If NEXUS still closes, the next start shows a
  report with the engine's `Host crash` line, and `eden_crash.dmp` is kept with it.

## After the fix: what the 17 Nov 14:39 session showed

- **The save loaded** and the game ran for 8 minutes (28,505 frames). The save-load crash did
  not happen again in that session.
- **NEXUS closed when the game was closed (14:47).** The new handler caught it:
  - `0xc0000005` at `nvoglv64.dll+0xe4e577` on the engine's `VulkanWorker` thread, 12 s after
    "stopping";
  - stack: `nvoglv64.dll`, `nvoglv64.dll`, `graphics-hook64.dll+0xe99c`, then `nexus_engine.dll`
    frames.
  - `graphics-hook64.dll` is OBS Studio's game-capture hook. It loads into Vulkan programs while
    OBS captures (or with OBS's Vulkan capture layer installed).
  - The crash is inside the NVIDIA driver, reached through that hook, while the engine shuts its
    Vulkan workers down.
  - The engine frames cannot be named: release builds kept no symbols. CI now also publishes the
    engine's link map (`NEXUS-<version>-engine-map`), so the next report's `nexus_engine.dll+0x…`
    frames can be looked up.
  - **Not fixed:** the cause is not known. If it happens again, try once with OBS closed. That
    would show whether the hook is needed for it.
- **The next start failed (14:50).** WebView2 reported success without a controller, and the
  WebView2 library NEXUS uses dereferenced it, ending NEXUS (`Logs\crash-go.log`). A start 30 s
  later worked.
  - NEXUS now carries that library with a fix (`app/third_party/go-webview2`): a failed WebView2
    start returns an error instead.
  - NEXUS then tries twice more, 2 and 3 s apart, before falling back to Edge's app window.

## Engine symbols (0.8.0)

Every Windows build ships `Engine\nexus_engine.symbols.gz`, the engine's functions by address.
NEXUS uses it to name the engine frames of a crash.

- **Made in CI** from the link map of exactly that DLL (`lld-link -MAP`) by
  `app/tools/release/make_symbols.py`. Names are undecorated with `llvm-undname`, or a simple
  form when that is not available.
- **First line:** `NEXUSSYM 1 <SHA-256 of nexus_engine.dll> <image base> <count>`.
- **The build fails** when:
  - fewer than 1,000 symbols are read;
  - any function the DLL exports does not resolve to a symbol of its own name (the map is not
    that DLL's, or was misread);
  - the table is missing (`NEXUS_REQUIRE_SYMBOLS` in `package_engine.py`, engine patch 0029);
  - the packaged table's hash does not name the packaged DLL (fresh-extract check).
- **Packaged** next to the DLL and hashed in `engine.json`, so NEXUS's engine check covers it. No
  separate download.
- **In a crash report** ("Engine functions in the crash stack"):
  - Each `nexus_engine.dll+0x…` frame becomes `Function+0x…  (object.obj)`.
  - This happens only when the table's SHA-256 is the one recorded for the engine that crashed
    (the session record keeps the engine's folder, DLL hash and commit).
  - A table of another build is refused ("for another engine build"). So are a missing table and a
    damaged one ("not decoded: …").
  - The decoded lines are also in the report's technical details.
- **Limits:**
  - Function and object file, not source line: release builds carry no line tables.
  - Frames in other modules (the GPU driver, overlays) are named by module and offset only.
