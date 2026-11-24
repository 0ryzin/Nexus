# Regression and bug-fix history

Each entry: problem · diagnosis · cause · files · fix · test · how to recognise it again.
Newest first.

## 0.9.3 (in development)

### Account page blank and Home account indicator missing (Windows, 0.8.1–0.9.2, 24 Nov)
- **Diagnosis:** Tom's `nexus.log` (24 Nov) shows the background synchronisation starting and
  never finishing before NEXUS was closed (27 s and 68 s runs; an earlier one took 1 min 13 s),
  and no interface errors. `Account.Status` counted unsent changes by reading every item, which
  waited for `artSyncMu`, held by `uploadCustomArt` for the whole of each picture upload, and
  read and hashed every custom picture each time.
- **Fix:** the count is worked out in the background (`accountpending.go`, at most 150 ms wait,
  then the last count); picture hashes are cached by path, size and time; the lock is not held
  while a picture is sent.
- **Files:** `account.go`, `accountpending.go`, `artsync.go`.
- **Tests:** `TestStatusFastWhileArtUploads` (about 2 s before, 1 ms after),
  `TestStatusFastWhileGitHubHangs`. Not yet seen on the user's PC.

### Chinese-named folder next to NEXUS.exe (Windows, 0.8.x–0.9.2)
- **Diagnosis:** the name in Tom's screenshot, read as UTF-16 code units, is the ASCII text
  `…sers\tmill\AppData\Roaming…`: WebView2 was given a damaged data-folder path.
- **Cause (likely):** the loader call (`webviewloader`, not a syscall) did not keep the UTF-16
  path alive, so its memory could be reused mid-call.
- **Fix:** `runtime.KeepAlive` on the strings; the folder also given through
  `WEBVIEW2_USER_DATA_FOLDER`; `removeMisnamedWebViewFolders` removes such folders at start
  (all non-ASCII name that decodes to a path, holding only `EBWebView`).
- **Tests:** `TestMisnamedWebViewFolder`. Needs a Windows run to confirm the folder no longer appears.

### Controller input in menus (D-pad reported dead on Windows, 0.8.1–0.9.2)
- **Not yet reproduced.** The log shows the controller detected and no interface errors.
- **Hardening:** `Pad.poll` scheduled the next poll at its end, so one exception stopped all
  controller input until restart; now caught and logged. Non-standard controllers' hat D-pad
  is read. The log records each controller (id, mapping, buttons, axes) and its first 16 presses.
- **Tests:** `tools/uitest/controller_robust.py`.

### Save conflict: the default choice could keep the less advanced copy (0.9.3)
- **Risk found while checking cloud safety:** the conflict dialog's primary (first-focused)
  button was "Keep this device's"; on the PC one press of A kept the PC's Ocarina of Time save over
  the Deck's further-ahead one in the account (the Deck would then have asked in turn; nothing was
  deleted, the replaced copy is backed up). The dialog showed only save times.
- **Fix:** "Decide later" is focused first; each device's play time and last play are shown
  (`SaveSyncConflict.LocalPlay/RemotePlay`, from this device's statistics and the other device's
  synchronised ones), with `Ahead` only when play time and save time agree. Link conflicts: no
  default either.
- **Test:** `TestSaveConflictShowsWhichIsAhead` (PC and Deck both change the save; the Deck has
  10 h, the PC 1 h: the PC's copy untouched, the Deck's marked ahead, choosing it backs up the PC's).

### Shader import: about 5,000 shaders to 0 after importing a 15,000-shader cache (0.9.3)
- **Cause (from the engine's code; the user's engine log not seen):** imports appended the
  source's `Used` records (one per play session: the pipelines it used). The engine keeps the 16
  newest sessions (`UsedPipelinesMaxAge`) and tombstones the rest at the next start, and builds at
  start the pipelines of the newest two sessions. Another device's newer sessions displaced and
  then deleted this device's own lists, so its pipelines were no longer built at start
  ("0 ready from the shader cache").
- **Fix:** `appendPortableStats` adds `Used` records only to a log that has none (the archive's
  programs and pipelines are still added); counts before and after from the file
  (`countShaderRecords`); `RestoreShaderBackup` puts back the pre-import backup (current data
  backed up first), "Undo the last import" in Import Shader Data.
- **Tests:** `TestImportKeepsThisDevicesUsedLists`, `TestRestoreShaderBackup`. Not yet seen on
  the user's PC.

## 0.9.2

### Custom game artwork never reached the Steam Deck (PC and Deck both on 0.8.1, 24 Nov)
- **Cause (reproduced in a test):** a game's pictures travelled inside its library item (title,
  hidden, art preference, fit, controls). When both devices had changed that item (the Deck had
  its own per-game details for Ocarina of Time), synchronisation parked it as a conflict waiting
  for the user's choice, and a parked item received no further updates: the PC's cover reference
  never arrived, however often either device synchronised. Pictures sent from the Deck were held
  back the same way.
- **Fix:** each game's pictures are their own account item (`library/<TID>.art`), merged picture
  by picture: the newer one wins (time it was chosen, carried with it), an older picture never
  replaces a newer one, replaced pictures are kept as backups, and such an item never becomes a
  conflict. A conflict about the game's other details no longer holds pictures back either.
  Unchanged pictures are not sent or downloaded again (SHA-256).
- **Files:** `account.go`, `artsync.go`.
- **Tests:** `TestArtworkRealOrder` (the user's order: both linked, then a cover chosen on the PC
  through the interface, Deck has its own details: failed before the fix), `TestArtworkNewerKept`
  (a newer Deck cover stays and reaches the PC), `TestArtworkPCToDeckAndBack`. Not yet seen on
  the user's two devices.

## 0.8.1

### Steam Deck Desktop Mode: some controller buttons did nothing (report, 19 Nov)
- **Cause:** started outside Steam in Desktop Mode, NEXUS receives no controller at all: Steam
  holds the Deck's controller and turns it into the keys of its Desktop configuration (A Enter,
  B Escape, Y Space, X Steam's keyboard, Select Tab, Start Escape, L1 Ctrl, R1 Alt, R4/R5 Page
  Up/Down, L4 Shift, D-pad arrows, triggers mouse clicks). NEXUS read Enter/Escape/arrows/Page keys,
  but a lone Ctrl or Alt (L1/R1) did nothing, Space (Y) acted as A, and X can only open Steam's
  keyboard.
- **Fix:** on a Steam Deck outside Gaming Mode, lone Ctrl/Alt switch tabs (Ctrl+key shortcuts
  untouched), Space is Y (options), a lone Shift is L4 (Home, when L4 is the Home button), and
  these keys are ignored while a real controller is in use (no double presses). A one-time tip says
  that X belongs to Steam and that starting NEXUS from Steam gives every button.
- **Test:** `tools/uitest/deck_desktop_keys.py`. Not yet tried on the Deck.

### A deleted save came back: the game still offered Continue (Steam Deck report, 19 Nov)
- **Problem:** *Settings › Users › profile › Save Data › Delete* showed the save as deleted, but
  Final Fantasy Resonance still offered Continue.
- **Cause (code path, reproduced in a test):** Delete removed `Saves/<user>/<game>` (correct: the
  engine reads exactly that folder, `save_directory` + patch 0019 folder names). But save sync's
  three-way decision treated "not on this device, in the account" as *a save this device never
  had* and installed the account's copy again at the next sync (start, wake, game closed, every
  15 min). Two smaller gaps: a stale copy in the engine's own layout inside the user's folder
  (`user/save/0000…/<user>/<title>`) was moved back by Tidy; and the game's *device save* (a
  separate entry) stayed, which some games read for Continue.
- **Fix:** `SaveSync.MarkDeleted` records the deletion; the next sync writes a deletion record to
  the account (`deleted: true`, no blob URL) and never installs that save here again. Other
  devices do not delete their copy silently: they ask (Delete here too / Keep this device's). A
  new save made after the deletion synchronises normally. `SaveOrganizer.DeleteSave` also deletes
  the engine-layout copies of that game in that user's folder, and, when chosen in the dialog, the
  device save. Everything is backed up first. Refused while that user is playing, with the reason.
  Other users' saves are never touched. Older NEXUS versions cannot install a deletion record
  (no URL), so they do not resurrect it either; if they *play* it, they send it again.
- **Files:** `saves.go`, `savesync.go`, `server.go`, `web/app.js`.
- **Test:** `TestSaveDeleteStaysDeleted` (Deck deletes Tom's save + device save + engine copy;
  two syncs and a listing later it is still gone; Ann's untouched; the account has two deletion
  records; the PC asks, then deletes after agreeing; a new game afterwards syncs normally),
  `TestSaveDeleteOtherCopyNotSynced`. Not yet tested on the Deck with the real game.
- **Recognise it:** a log line `save sync: … 1 save(s) received` right after `saves: deleted …`.

## 0.8.0 (in development)

### Ocarina of Time title screen without its background on the Steam Deck (18 Nov)
- **Problem:** the title menu (Continue, New Game, Options) showed, but not the scene behind it
  (adult Link on Epona, the moon).
- **Cause:** that background is a video (Lichii #2204 describes it), and Switch 2 videos are AV1.
  The Linux engine's FFmpeg (`LICHII_USE_EXTERNAL_FFMPEG`) was built with H.264, VP8 and VP9 only,
  so `avcodec_find_decoder_by_name("libdav1d")` found nothing (a gap recorded in LINUX.md since
  0.7). The Windows engine has dav1d.
- **Fix:** engine patch 0044 + CI: a static dav1d 1.5.1, `--enable-libdav1d`. CI's engine test
  `nx2_video_av1` (skipped before) decodes AV1. Not yet seen on a Deck.
- **Game audio (same report):** not explained by this; cause unconfirmed without a Deck log
  (Settings › Storage › Send this device's logs).

### Installing title.keys "not accepted" and prod.keys lost (Steam Deck report, 18 Nov)
- **Cause:** *Install title.keys* called `InstallKeys(<installed prod.keys>, title)`. InstallKeys
  first moved the installed files to the archive, then copied prod.keys from its installed path,
  which no longer existed: the copy failed ("not accepted") and prod.keys was left in the archive.
- **Fix:** `InstallTitleKeys` checks and installs title.keys alone; prod.keys is never touched.
  `InstallKeys` reads and checks both files before anything changes, copies the installed files to
  the archive (they stay in place) and replaces them atomically. A title.keys file must hold
  `<32-digit rights ID> = <32-digit key>` lines; a prod.keys chosen by mistake is named as such.
- **Test:** `TestTitleKeysNeverBreakProdKeys` (invalid title keys, prod.keys as title keys, valid
  title keys, replacement, the old route).

### Play time nearly doubled on the Steam Deck (15.5 h shown as ~31 h, 18 Nov)
- **Evidence (the user's PC data):** the account listed the PC three times ("Toms_Pc", three
  device IDs, one per earlier sign-in) plus the Deck. Each PC ID's last upload of its play time
  was kept, and every device added up all other devices' uploads. The PC's 55,770 s were added two
  or three times on the Deck. (The PC's `adjusted` field, 53,989 s, is only a note of a manual
  correction and is not counted again.)
- **Fix:** `ProfileStore.Stats` adds another device's record only if it is not an older upload of
  one already counted: records that share a session start second or the first-play second come
  from the same install, and only the newest counts. Applies to this device's own older uploads
  too. Nothing is deleted or rewritten. (Re-sign-in already keeps the device ID since 0.7.)
- **Test:** `TestStatsOneDeviceUnderSeveralIDs` (the user's numbers).

### "MiNS" could not be deleted (Steam Deck report, 18 Nov)
- **Problem:** typing the user's name in the delete dialog and pressing Enter did not delete it.
- **Causes (three):**
  1. Enter in a dialog's text field chose the *primary* button, or else the first button. The
     delete dialog's Delete is a danger button, not primary, so Enter chose Cancel and closed
     without a message.
  2. The name had to match exactly: "Mins" did not delete "MiNS" (said only "did not match").
  3. When deletion was reached (Delete clicked), `ProfileStore.Delete` deadlocked (entry below).
- **Fix:** Enter confirms with the button that returns the text when none is primary; the name
  is compared without regard to capitals or surrounding spaces (interface and server); the
  mismatch message names what to type.
- **Test:** `tools/uitest/profile_delete.py` (CI): keyboard ("mins" + Enter), controller only
  (A on Delete; B cancels), wrong name, restart, data archived. The build before the fix hangs
  in it.

### Settings with a controller: Right went to the search box; Down left the panel (18 Nov)
- **Problem:** at 1280×800 (Steam Deck) Right on a settings category focused the search box above
  the list; Down past the last row of a short panel moved into the category list and switched
  category.
- **Cause:** Nav.move's geometry scored the nearby search box above the panel's first row, and
  nothing kept Up/Down inside the panel.
- **Fix:** category buttons send Right to the panel row nearest them; `#set-panel` is a
  `data-nav-column`, which keeps Up/Down inside it.
- **Test:** `tools/uitest/controller_sweep.py` (CI): every Home shortcut and settings category,
  controller only.

### Deleting a profile hung NEXUS (found by a test, 18 Nov)
- **Problem:** deleting a user never returned; NEXUS stopped responding to that request.
- **Cause:** `ProfileStore.Delete` held the store's lock and called `SaveDir`, which calls `Get`,
  which takes the same lock (Go mutexes are not re-entrant).
- **Fix:** `saveDirFor(profile)` resolves the folder without locking; `Delete` uses it.
- **Test:** `TestTemporaryUserRemoved` deletes a user.
- **Recognise it:** a goroutine dump with `ProfileStore.Get` under `ProfileStore.Delete`.

### Ocarina of Time save missing on the Steam Deck (18 Nov)
- **Problem:** a save made on Windows was not on the Deck after signing in to the same account.
- **Cause:** game saves were never synchronised; the account synced only profiles and settings
  ("Game saves stay on each device for now").
- **Fix:** `savesync.go` (see CHANGELOG 0.8.0). **Test:** `TestSaveSyncWindowsToDeck`.

### No on-screen keyboard on the Steam Deck (18 Nov)
- **Cause:** NEXUS's keyboard opened only after gamepad input; in Desktop Mode the Deck's controls
  arrive as mouse and keyboard, so it never opened. Steam+X depends on Steam Input's own mode.
- **Fix:** on a Deck (DMI Valve Jupiter/Galileo) or in Gaming Mode text fields open the Steam
  keyboard, then NEXUS's own. Not tested on a Deck.

### No interface sounds on the Steam Deck (18 Nov)
- **Cause (probable, not confirmed on a Deck):** the AppImage lacked GStreamer's `interleave`
  plugin, which WebKit's Web Audio needs, and WebKit's autoplay policy could block sound.
- **Fix:** plugins added, autoplay allowed. **Test:** CI records the start-up sound (peak > 500).

### Linux packages that would only have worked on Ubuntu (found by CI, 18 Nov)
- **Problem:** the first Linux packages linked Ubuntu's FFmpeg 6 (libavcodec.so.60 plus about 40
  codec libraries). In the AppImage, NEXUS (in `usr/lib/nexus`) had RUNPATH `$ORIGIN` and found
  the bundled WebKitGTK only on systems that had their own. On Arch-based systems such as SteamOS,
  the engine or the interface would not have loaded.
- **Cause:** the engine used system FFmpeg; linuxdeploy's GTK plugin sets `$ORIGIN` on every ELF
  file under `usr/lib`, including the ones in subfolders.
- **Fix:**
  - FFmpeg is built into the engine (engine patch 0040; `-Wl,-Bsymbolic`).
  - libusb, OpenSSL 3 and lz4 are copied next to the engine.
  - `appimage_rpaths` in `make_linux.py` gives programs and plugins below `usr/lib` a path back to it.
  - The WebKit helper-path rewrite has the right length.
- **Test:**
  - nexus-linux fails if the engine links a version-specific system library.
  - Every smoke test (host, plain Ubuntu, Arch) checks that the engine loads and the interface starts.
- **Recognise it:** "error while loading shared libraries" or "the engine library does not load"
  in the smoke test.

### Linux reported no graphics card (18 Nov)
- **Problem:** on Linux NEXUS never detected a GPU or display. Frame generation was never offered
  there, and the optimiser assumed an entry-level GPU.
- **Fix:** `hardware_sysfs.go` reads graphics cards and displays from sysfs. It recognises the
  Steam Deck's APUs and portrait panel, and skips firmware framebuffers and virtual machines'
  adapters.
- **Test:** `TestSysfsSteamDeck`, `TestSysfsDesktop`, `TestHardwareDetectionHonest`.

### NEXUS volume output one step off Lichii's at 100 %
- **Problem (found 17 Nov, CI on build 316):** Lichii's `nx2_sysmix` test (Orbitals 5.1 audio
  downmixed to stereo) failed in the NEXUS Engine: centre sample 9765, expected 9764.
- **Cause:** NEXUS's gain helper (`audio_core/sink/nexus_gain.h`, patch 0001) rounded samples where
  Lichii truncates, although it promises output identical to Lichii's at or below 100 %.
- **Fix:** truncation as upstream at or below 100 %; rounding only above 100 % (NEXUS's range).
  Engine patch 0024. Inaudible (one step in 32,768), but now exact.
- **Test:** Lichii's `nx2_sysmix` test and NEXUS's `nexus_gain` tests, every 16-bit sample at 100 %.

### Installing keys could race with a background vault copy
- **Problem (found 17 Nov, CI):** `TestSystemFilesKeys` failed intermittently ("keys not
  installed into the NEXUS engine"; locally also "directory not empty" after the test).
- **Cause:** `InstallKeys` started a background copy between the engine's keys and the per-user
  system vault on every call, including rejected files, and these copies ran unsynchronised
  with each other and with the next install, so one could copy keys while another wrote them.
- **Fix:** the vault is synchronised only after a successful install, before `InstallKeys`
  returns; all vault copies (start-up, keys, firmware) take one lock.
- **Files:** `app/systemfiles.go`, `app/systemvault.go`.
- **Test:** `TestSystemFilesKeys` passes 60 runs in a row and under shuffled test order (it
  failed in 4 of 6 shuffled orders before).

### NEXUS Account unsynchronised after a period of inactivity
- **Problem (13 Nov):** after NEXUS had been left alone, the account showed as unsynchronised or
  signed out.
- **Cause (from the code; several):** (1) synchronisation was skipped while any game was running,
  and NEXUS keeps games running behind Home, possibly for hours; (2) "15 minutes since the last
  sync" used Go's monotonic clock, which on Windows stops while the computer sleeps, so after a
  night asleep nothing was pulled until 15 more awake minutes; (3) nothing tried again when the
  computer woke or NEXUS came to the front, so "offline" could stay for 10-15 minutes; (4) one 401
  from GitHub deleted the sign-in; (5) one failed read of the stored sign-in at start signed the
  device out for good.
- **Fix:** only a game in front waits; wall-clock timing; sync after wake and on coming to the
  front (at most every 30 s); a 401 is confirmed with `/user` before signing out; an unreadable
  sign-in is retried five times. Switching to a different account holds this device's uploads
  until the user chooses (data from one account is never copied into another by accident).
- **Files:** `app/account.go`, `app/account_github.go`, `app/web/app.js`.
- **Tests:** `account_session_test.go` (switch hold and release both ways, transient 401 vs
  revoked, unreadable sign-in retried then ended, a background game does not stop sync). Sleep
  detection is not unit-tested (it needs a real suspend); check on your PC.

### Home with a running game: the controller could not reach Close game
- **Problem (13 Nov):** from NEXUS Home with a game running in the background, a controller
  could not get to the Close game button.
- **Cause (found with the controller UI test):** two faults in the same path. (1) Pressing Home
  shortly after a game appeared (within the loading screen's ~2-3 s fade) left the loading
  screen's `launch` layer on the navigation stack, so the controller's focus stayed on that
  invisible layer; its timers then switched the interface to the in-game "playing" screen behind
  the backgrounded game, whose only control is "Return to game". (2) `goHome` closes every dialog
  layer with `layer.close()`, and the loading screen's layer had no `close`, so it threw and
  stopped half-way. Also, Close game existed only on the Home banner, reachable only through
  spatial navigation.
- **Fix:** going Home closes the loading screen, its timers no longer leave Home once the game is
  set aside, and its layer has a `close`. Close game is now in three places, all with one
  confirmation that opens with Cancel focused: the running game's Options menu (Y/X on its
  tile: Resume, Close Game), the Power menu (Start: Resume / Close <game>), and the Home banner,
  which Up from the tiles now always reaches. If the banner disappears while focused, focus
  returns to a tile. Controller disconnection is announced and clears held-button state.
- **Files:** `app/web/app.js`.
- **Test:** `tools/uitest/controller_close.py` (Linux CI job): the real interface in Chrome
  with a scripted controller through the Gamepad API, all three ways, checking Cancel is focused
  first, the session ends, the running light goes and focus stays on a tile. Against the old
  build it fails with `left Home after Home was pressed: screen 'playing'`.

### Loading screen waited forever when a game started quickly
- **Problem (found by the same test):** a second launch in a row never left the loading screen,
  although the engine reported the game running.
- **Cause:** `launchGame` sends the launch, plays the tile animation (up to ~0.9 s), then opens
  the loading screen with the launch reply. State updates that arrived during the animation,
  including "playing", were overwritten by that older reply, and the loading screen waited for a
  "playing" it had already missed. Any start faster than the animation hit it.
- **Fix:** the newest state of the same session (`S.emuLatest`) wins over the launch reply.
- **Test:** the second and third launches in `controller_close.py`.

### Performance bar only appeared after pressing F11 twice
- **Problem (13 Nov):** at game start the performance bar was often missing; leaving and
  re-entering full screen (F11) made it appear.
- **Cause (from the code):** the bar is a separate layered window *owned* by the window the game is
  drawn in. Its owner is set after it is created (`GWLP_HWNDPARENT`), and Windows only re-stacks
  owned windows above their owner when the owner itself is re-stacked. Until that happened the
  bar could sit below the game window, drawn but covered. F11's `SetWindowPos` on the NEXUS
  window re-stacked it, which is why F11 "fixed" it.
- **Fix:** the bar puts itself directly above its owner (just below whatever window is in front of
  the owner, so it never covers another program) whenever it is shown, changes owner, or the
  owner moves, resizes, is restored or comes to the front. Whether it is on screen is asked from
  Windows instead of remembered (Windows hides and re-shows owned windows on minimise). No
  periodic redraw was added. The refresh timer now runs only while a game is running (it used to
  wake the HUD thread twice a second on Home too).
- **Files:** `app/overlay_windows.go`.
- **Test:** `TestHUDStacksAboveItsOwner` (Windows CI, real windows). The full in-game check needs
  your PC: start a game in full screen and in a window, minimise/restore, change resolution.

### NEXUS started without a window (audio played, nothing on screen)
- **Problem (13 Nov):** sometimes NEXUS.exe started, the boot sound played, but no window
  appeared; starting it a second time sometimes made it appear.
- **Cause (from the code):** when NEXUS started in full screen (saved whenever F11 is pressed on
  Home, which is why it was intermittent), `applyFullscreen` wrote `WS_POPUP|WS_VISIBLE` into the
  style of a window that had never been shown and then called `SetWindowPos(SWP_SHOWWINDOW)`.
  Windows treats a window with the WS_VISIBLE bit as already shown, so nothing was shown. WebView2
  still loaded the interface, so the boot sound played. The second NEXUS.exe's `SW_RESTORE`
  sometimes forced it on screen. A second click while the first NEXUS was still starting (no
  window yet) found nothing and exited silently.
- **Fix:** the window is created hidden, laid out (full screen or not) while hidden and shown
  once with `ShowWindow`; the style's WS_VISIBLE bit is never written by hand
  (`windowStyleFor`). After the interface loads, the window is checked and brought back if it is
  hidden or off every monitor. A second NEXUS.exe waits up to 20 s for the window, lets the
  running NEXUS take the foreground and asks it to show itself (it restores its own full screen);
  if there is no window it says so. A panic shows a message instead of vanishing.
- **Files:** `app/app_windows.go`, `app/window_windows.go`, `app/main.go`.
- **Test:** `window_windows_test.go` (runs on the Windows CI runner with real windows):
  full screen before first show leaves the window hidden until shown, then visible; hidden,
  off-screen and minimised windows are recovered; a second instance finds the window.
- **Recognise:** `nexus.log` now has `startup:` lines (core ready, window created, window shown
  with its state, WebView2 ready, interface loaded, and any recovery).

### Ocarina of Time: black screen after loading a save (open)
- **Problem (13 Nov):** Play → load a save → black screen that keeps loading.
- **Diagnosis so far:** source comparison only; the game cannot be run here. The engine moved
  to Lichii build 177, which fixes an update's ExeFS being applied over a NUP program (#1504)
  and several GPU-service waits that could not end (#1413, #1512, #1590). NEXUS's optimiser also
  turned reactive flushing off for most profiles, unlike the engine's tested default.
- **Files:** `engine.lock` (build 177), `app/optimize.go` (`switch2Limits`), `app/engine.go`
  (`switch2SessionSummary`).
- **Test:** `TestSwitch2OptimiserLimits`, `TestSwitch2SessionSummary`. Not confirmed with the game.
- **Recognise:** the NEXUS log line `engine: Switch 2 session ...` shows the settings and update
  files; the engine log shows `ExeFS: Ignoring installed update` when #1504 applies.

### Switch 2 low-RAM warning test passed only on PCs with less than 16 GB
- **Problem:** `TestSwitch2LowRAMWarning` failed in the Linux CI job (16 GB runner).
- **Cause:** the test set the hardware cache directly, but detection replaced it with the real
  machine's memory; on an 8 GB PC the warning appeared anyway.
- **Fix:** the test uses the `NEXUS_HARDWARE` override (`useHardware`) and also checks that a
  32 GB PC gets no warning.

## 0.7.0 (in development)

### Typing in the settings search triggered shortcuts (17 Nov)
- **Problem:** in the Settings search box (added the same day) H went Home, X opened Options,
  Q/E switched tabs; the same in e-mail and password fields of the sign-in forms.
- **Cause:** the keyboard handler treated only `input[type=text]` as a text field.
- **Fix:** `isTextField()` (text, search, e-mail, password, URL, number, phone, textarea) in
  the key handler, focus and Back.
- **Tests:** `tools/uitest/resets_osk.py` types "hex" into the search box.

### Signed out of GitHub again and again (17 Nov)
- **Problem:** NEXUS kept signing the user out of the connected GitHub account (three times
  between 12 and 17 Nov), with nothing in `nexus.log`.
- **Diagnosis:** `account.json`'s own history (read on the user's PC; no credentials read): every
  sign-out was "GitHub no longer accepts NEXUS's sign-in", i.e. a 401 from GitHub confirmed by
  `GET /user`, never NEXUS deleting the token by itself. Sign-in 17 Nov 13:31 still worked at
  18:12 (4 h 41 min) and was refused at 22:03 (8 h 32 min); the other two failures also came
  after more than eight hours. Since August 2026 GitHub gives new OAuth apps (NEXUS's included)
  eight-hour access tokens with a six-month refresh token; NEXUS kept only the access token and
  discarded the refresh token. The new download folders each time were a coincidence (the data
  folder and the DPAPI-encrypted `session.bin` do not depend on it).
- **Files:** `app/account_github.go`, `app/account_github_cred.go`, `app/account.go`,
  `app/web/app.js` (AccountUI).
- **Fix:** the whole sign-in (access token, refresh token, expiry times) is kept, encrypted with
  Windows data protection as before; renewed 10 min before it expires and after a 401, with the
  request retried; the rotated refresh token is saved at once. The sign-in ends only on
  Disconnect or when GitHub refuses the refresh token itself (`bad_refresh_token` /
  `invalid_grant`). Network errors, 5xx and unknown renewal errors keep it. Device flow asks for
  `gist offline_access`. Reconnecting the same account keeps the device's entry (the user's list
  had three "Toms_Pc").
- **Tests:** `account_persist_test.go` (the brief's seven steps against a GitHub stand-in with
  expiring tokens, plus revocation, 503 while renewing, old-format sign-in), run on Linux and in
  the Windows CI job (DPAPI; also checks the file holds no token in plain text). **Not yet
  confirmed on the user's PC against github.com:** the existing sign-in has no refresh token, so
  one more Connect is needed after updating.

### Ocarina of Time: about 100 s (once 10 min) before the title screen at every start (17 Nov)
- **Problem:** every start of Ocarina of Time built the shader precache again, until *Start now*.
- **Diagnosis:** the user's engine logs (two sessions). "Shader precache: 4671 of 4794 compute
  programs to build" both times, although 4,141 had been built the first time. The `.done`
  marker was written only when the precache finished; there was no partial record.
- **Files:** engine `video_core/renderer_vulkan/vk_pipeline_cache.cpp` (PrecacheShaders),
  `video_core/shader_warm/shader_warm_builder.*` (patch 0028).
- **Fix:** `<precache>.partial` with the built programs' key hashes, under the marker's stamp.
- **Tests:** engine `ShaderPrecache: progress of an unfinished precache is kept for the same
  stamp`. **Loading time on the user's PC not measured yet.**

### Controllers shown as "Xbox 360 Controller" (17 Nov)
- **Problem:** an Xbox Series X controller was shown as an Xbox 360 controller in NEXUS menus.
- **Diagnosis:** the menus used the browser Gamepad API's name, which is "Xbox 360 Controller"
  for every XInput device. The engine (SDL) saw `045E:0B12` (`controllers.json` on the PC).
- **Files:** `app/padmodels.go`, `app/controllers.go`, `app/web/pads.js`, `app/web/app.js`.
- **Fix:** names from vendor and product IDs. Browser pads matched to the engine's list; prompts
  per controller family.
- **Tests:** `padmodels_test.go`, `tools/uitest/pads_test.js`, `controller_prompts.py`.
  **Not tested with physical controllers.**

### Ocarina of Time: NEXUS closed without a message after "Load existing save" (17 Nov)
- **Problem:** menu fine (about 150 FPS); loading a save gave a black screen, then the whole of
  NEXUS closed with no dialog, no report and no dump.
- **Diagnosis:** the user's `nexus.log` and `eden_log.txt` (read only). The engine log ends at
  666.7 s right after a burst of unmapped device reads from `buffer_cache.h:4005`; no `Host crash`
  line. Silent because `nexus_engine` never installed the engine's crash handler and the Go runtime
  disables Windows Error Reporting (`SEM_NOGPFAULTERRORBOX`). Likely cause: Lichii #2041, a
  Windows race where the sparse-vector fault handler sets a page read-only while another thread
  writes to it. Not confirmed with a dump.
- **Files:** engine patches 0025 (#2041), 0026 (#2062), 0027 (crash handler); `app/crashreport.go`,
  `app/engine.go`, `app/main.go`, `app/server.go`, `app/web/app.js`, `app/web/eta.js`.
- **Fix:** the two upstream fixes; crash handler, WER, Go crash output, session record and crash
  report at the next start (docs/CRASHES.md).
- **Tests:** engine `sparse_large_vector` test (CI `tests.exe`); `crashreport_test.go`;
  `tools/uitest/eta_test.js`, `shader_progress.py`. **Not tested with the game on the user's PC.**

### Home showed a menu instead of the Home screen
- **Problem:** pressing Home during a game showed a screen with *Return to game / Close game /
  Show performance monitor*. Choosing the running game on Home asked *Cancel / Close / Resume*.
- **Diagnosis:** source reading. `emulator.home` correctly opened Home, but the NEXUS window then
  received focus (`app.focus` event) and `showPlaying()` replaced Home with the "playing" screen,
  even though the game was suspended. Selecting the suspended game opened a dialog.
- **Files:** `app/web/app.js` (`app.focus` handler, `launchGame`).
- **Fix:** `app.focus` only shows the playing screen when the game is in front; selecting the
  running game resumes it directly (closing it stays on the Home banner).
- **Recognise:** any handler that calls `showPlaying()` must check `S.playing.suspended`.

### Keys and firmware missing after extracting a new version
- **Problem:** a new NEXUS folder started without the keys and firmware installed before.
- **Cause:** portable copies (`portable.txt`, recommended for test builds) keep all data in their
  own folder, so a new folder has none.
- **Files:** `app/systemvault.go`, `main.go`, `systemfiles.go`; tests `systemvault_test.go`.
- **Fix:** a per-user vault, synchronised at start in both directions (newer file wins, nothing
  deleted), and a one-time search of sibling NEXUS folders for an earlier copy's keys.
- **Test:** `TestSystemVault`, `TestSystemVaultFindsEarlierCopy`.

### Ocarina of Time stuck on its loading screen (open)
- **Status:** not reproduced (no game in CI). See `COMPATIBILITY.md` for what was ruled out and
  what the next test must collect.

## 0.6.0
- F11 stuck: full-screen state read before the previous change was applied; now atomic
  (`ToggleFullscreen`) and every press counted (`engine.go`, `app_windows.go`).
- Keyboard not reaching a game drawn in the NEXUS window: clicks on a child window do not move
  the keyboard focus; engine patch 0017 gives focus back while the game is in front.
- Loading-screen Cancel: asked a question the game could cover; now stops at once.
- Duplicate play statistics: play time recorded under the old file-name key (`f-…`) before the
  Title ID was readable; merged after each scan (`statsmerge.go`).
- Tests writing into removed folders: online artwork worker and game sessions outliving tests;
  `App.Close` waits for the worker, test apps stop sessions (`artwork_online.go`, fixtures).

Earlier fixes: see `CHANGELOG.md` and `docs/SOURCE_AUDIT.md`.

## Home showed a "Now playing" menu instead of the Home screen (0.7.0 development)
1. **Problem:** pressing Home (or Back/Escape in NEXUS while a game was in front) sometimes showed the
   "Now playing" screen with Return to game / Close game / Performance monitor instead of Home.
2. **Diagnosis:** `goHome()` in `web/app.js` showed that screen whenever a game was running and not
   yet suspended, unless the call came from the Home button itself; Back on that screen resumed the game.
3. **Fix:** `goHome()` always sets a running game aside first (suspend/background per Settings) and
   the `emulator.home` event then shows Home; Back on the "Now playing" view goes Home; that view
   (only visible behind a game in its own window) has no menu any more: the running game's tile
   resumes it and the Home banner offers Close game.
4. **Tested:** UI logic only (headless); not yet with a game.
5. **Recognise it:** a screen with game art and Resume/Close buttons appears where Home was expected.
