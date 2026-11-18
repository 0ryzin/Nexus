# NEXUS 0.4.0 (preview)

NEXUS is a console-style game system for Windows. It starts with a power-on animation, then opens a home screen with your game library, user profiles, themes and controller navigation.

**What NEXUS does:**
- **NEXUS sets itself up and manages its own files.** At the first start NEXUS creates its engine data folder and configuration on its own. It never reads or needs another emulator's folders: keys and firmware are installed in *System files*, saves in *Save Data*, updates and mods per game.
- **Save Data:** every user has `UserData\Saves\<PLAYER_ID>-<name>\<GAME_ID>-<SHORT>\` (for example `0400486024DEC000-OOT` with `slot_00 … slot_05`, `account.sav`, `option.sav`), which games read and write directly. Save Data shows each game's slots and files by name, and imports saves from a folder or ZIP (it tells you which player and game they belong to, from the folder IDs or the save's own metadata), exports, backs up, restores and deletes — never replacing or deleting a save without asking and without a backup.
- **Media:** screenshots (Alt+F12, or your controller's Share / Capture button while playing) in `Media\Screenshots`, and recordings in `Media\Recordings`, shown in the Media tab with their game and date; filter, sort, view, open, show in folder, delete (to the Recycle Bin).
- **Screen recording:** F12 while playing starts and stops a recording of the game window (never the NEXUS screens), with the game's sound and optionally the microphone. A red *REC* indicator with a timer shows while it records; *Recording saved* or the reason it failed afterwards. NEXUS uses the graphics card's encoder (NVIDIA NVENC, AMD, Intel Quick Sync) when there is one and software encoding otherwise. *Settings › Screen recording*: on/off, shortcut, resolution, frame rate, quality / bit rate, H.264 or HEVC, game sound, microphone, folder. Files are fragmented MP4 written as `<name>.mp4.part` until the recording stops, so a crash still leaves a playable file, renamed "… (interrupted).mp4" at the next start.
- **NEXUS Home while playing:** one press of the Home key or the controller's Home/Guide button goes to NEXUS Home. By default the game keeps running in the background, muted and receiving no input (a green dot on its tile and a *running in the background* banner) until *Resume*; *Settings › Game sessions & saves › When you press Home in a game › Pause the game* stops it instead. The button never reaches the game itself. The record and screenshot shortcuts work on Home too.
- **Import Save from ZIP:** *Save Data › Import* takes a ZIP archive of Switch save data from any compatible source: zip the save folders (the game folders named with their 16-character ID, or the player folder that holds them), choose the ZIP, and NEXUS lists every save it recognises, with its player, game, slots, account and option data, so you pick which to import and into which user. The archive is only read; NEXUS refuses unsafe or damaged archives, checks every file before anything changes, and backs up a save before replacing it (only after you confirm).
- **F11** switches the game between fullscreen and a window without restarting it.
- **Controllers:** NEXUS remembers every controller (by serial number where it has one), lets each user choose which controller is player 1, 2 … (multiplayer), and gives a controller its own button layout if you want.
- **Game Updates and Mods** in each game's *Configure Game Settings*: NEXUS reads a package and accepts it only if it really is an update for that game; mods install for that game only and can be switched on and off.
- **NVIDIA DLSS files:** on GeForce RTX cards NEXUS can download NVIDIA's Streamline files from NVIDIA's own release (after showing NVIDIA's licences and asking), checks them and installs them.
- **Automatic updates:** NEXUS checks the official release page, shows what's new, downloads the update, checks its SHA-256, installs it and restarts; if the new version does not start, the previous one comes back. Your data is never touched.
- **Starting a game:** *Who's playing?* (when there is more than one user), the game's artwork opens, the NEXUS loading screen shows the real loading steps, then the game. If a game cannot start, NEXUS says why in plain words; the technical details and a diagnostic log are one button away.
- **Graphics card:** on PCs with two graphics processors (most gaming laptops) NEXUS makes games run on the dedicated card instead of the integrated one; *Emulation settings › Graphics card* can choose integrated graphics to save battery. The hardware report also warns about Vulkan layers from other programs that are known to break games.
- **Frame generation and DLSS:** frame generation (NEXUS's own built-in optical-flow interpolation on any Vulkan graphics card, or Lossless Scaling with your own copy), NVIDIA DLSS in place of a game's own DLSS on GeForce RTX cards, and AMD FSR 4 on Radeon RX 9000 cards, in *Emulation settings › Frame generation & DLSS* and per game. NEXUS detects your graphics card and offers only what it supports.
- **The engine is built and shipped with NEXUS.** The NEXUS engine is compiled on Windows by the project's build (GitHub Actions), its tests run, and it is packed into the same ZIP as `NEXUS.exe`. NEXUS finds it in `NEXUS\Engine`, checks every file and loads it in the background while the interface starts. `NEXUS.exe --engine-check result.json` runs the same check from the command line.
- **First-time setup:** when there are no users yet, NEXUS opens a short setup — name → picture → profile colour → review. Your picture and colour (a ring around your picture, and the highlight colour while you are signed in) appear wherever your user is shown and can be changed later in *User settings*.
- **Up to 8 users.** *Add user* disappears when 8 exist; existing users are kept as they are.
- **Controller layouts for games:** Nintendo positions or Xbox labels, the controller type the game sees (Pro Controller, Joy-Con pair, handheld), and any Switch button moved to another button — for all games (*Controllers*) or for one game (*Configure Game Settings › Controls*). Some games come with a preset (for example, Joy-Con pair for party games). *Restore defaults* is always one step away.
- **Light and dark themes everywhere:** every screen, list, dialog and the setup use one set of theme colours, and text in your profile colour is adjusted to stay readable on both themes.
- **One download.** `NEXUS-<version>-Windows-x64.zip` contains NEXUS *and* its engine (`NEXUS\Engine`). Extract it and run `NEXUS.exe`; nothing else to install. The engine is found relative to `NEXUS.exe`, so the folder works wherever you put it, and it is updated together with NEXUS.
- **Hardware information** and **Optimise for my device** (*Emulation settings*): NEXUS reads your CPU, GPU, memory, Windows version and display, and recommends engine settings with *Performance*, *Balanced* or *Quality* profiles, for all games or for one game. You see every change first, choose which to apply, and can restore the previous settings.
- **Animated wallpapers** (GIF, animated WebP, MP4, WebM) with a preview, and **Home transitions** (how the Home screen appears when you press Home, including from a game); *Resume* takes you back to the game.
- **NEXUS runs games itself.** The NEXUS engine is loaded inside NEXUS (it is built from open-source Switch emulation code; see *Licensing*). When you press Play, NEXUS opens the game window, plays the game and brings you back to the home screen. No other emulator program is started, and there is no emulator to choose.
- **NEXUS Engine** settings: status, supported systems, engine builds with verified install and rollback.
- **Game volume 0–200 %** with a NEXUS slider: in *Game sessions & saves*, *Emulation settings › Audio*, per game, and live on the playing screen. 100 % is the game's own level; above it a soft limiter prevents harsh clipping.
- **Compatibility** per game, from what actually happens when you play it in NEXUS, plus your own rating.
- Updates and DLC in your game folders are applied automatically; player 1 is mapped to your controller automatically.
- Fast library: tiles use cached, downscaled artwork.

> **About this package.** This ZIP was produced by the project's Windows build (GitHub Actions, `.github/workflows/nexus-build.yml`): it compiles the NEXUS engine from the engine source pinned in `engine.lock` plus the NEXUS patches in `engine/patches`, runs the engine's test suite (its full test suite), loads the DLL and checks its interface, runs NEXUS's own tests on Windows, builds `NEXUS.exe`, packs both into this ZIP, then extracts the ZIP into a fresh folder and has `NEXUS.exe --engine-check` find, verify and load the engine there. The build machine has no GPU and no games, so **running real games has not been tested yet** (see *Known limitations*).

NEXUS is an independent project. It is not affiliated with or endorsed by Nintendo. It contains **no games, keys or firmware**.

---

## 1. Quick start

1. Download `NEXUS-<version>-Windows-x64.zip` and extract it anywhere you can write to, such as `D:\NEXUS`. Don't use `Program Files`. Keep the folders together: `NEXUS.exe` loads its engine from `Engine\` next to it.
2. Double-click **`NEXUS.exe`**. The engine is already included; there is nothing to install.
3. Create your user. If NEXUS 1.x had saves, the first user is offered a copy.
4. Put your **own** legally dumped games in `NEXUS\Games\` (or add folders in *System Settings › Game library*).
5. *System Settings › System files*: install the `prod.keys` dumped from **your own** console, then the firmware from the same console. NEXUS validates both and installs them into the engine.
6. Optional: *Emulation settings › Optimise for my device* to pick settings for your PC.
7. Select a game and press **A** or **Enter**.

**Updating NEXUS:** extract the new ZIP and replace the old NEXUS folder (or put it next to it). Your users, saves, settings, keys and firmware live in `%APPDATA%\NEXUS` and are not touched; the engine is replaced together with NEXUS, so they always match.

Updating from NEXUS 1.x: your users, saves, statistics, artwork, settings, keys and firmware are kept. Keys, firmware, installed updates/DLC and controller profiles from `%APPDATA%\eden` are brought into the engine data folder (originals untouched). The old `Emulator` folder is no longer used; you may delete it.

## 2. Requirements

| Component | Notes |
|---|---|
| Windows 10 or 11, 64-bit | |
| Microsoft Edge **WebView2 Runtime** | Built into Windows 11 and current Windows 10. |
| Microsoft **Visual C++ 2015–2022 x64** runtime | Included in `Engine\Libraries` (Microsoft's app-local deployment). If those files are missing, NEXUS says so and links to Microsoft's installer. |
| A GPU with Vulkan 1.3 (recommended) or OpenGL 4.6 | Comes with your graphics driver. If no Vulkan driver is found, NEXUS tells you before the game starts and suggests OpenGL. |
| Your own `prod.keys` and firmware | **Not included.** |

## 3. Where things are stored

NEXUS keeps **your data outside its program folder**. You can delete the NEXUS folder and extract a new version without losing anything.

| What | Where |
|---|---|
| Users, play statistics, per-user saves, settings, artwork, per-game settings, compatibility results, profile icons, wallpapers, logs | `%APPDATA%\NEXUS\` |
| System files: validated master copies of prod.keys, title.keys and firmware | `%APPDATA%\NEXUS\SystemFiles\` |
| **Engine data**: keys and firmware in use, console NAND, installed updates/DLC, shader caches, engine configuration and log | `%APPDATA%\NEXUS\Engine\` |
| NEXUS program, engine builds (`Engine\builds\<version>`), sounds, icon packs | the NEXUS folder you extracted |

**The NEXUS folder** (from the ZIP):

```text
NEXUS\
├── NEXUS.exe
├── Engine\                 the NEXUS engine, matched to this NEXUS version
│   ├── nexus_engine.dll
│   ├── engine.json          version, source commit, SHA-256 of every engine file
│   ├── Libraries\           SDL3, FFmpeg and the other engine DLLs, VC++ runtime
│   └── Licences\            engine licence and component licences
├── Games\                  put your games here (more folders can be added)
├── Resources\              sounds, icon packs, themes, artwork database
├── Licences\  Source\       licences, NEXUS and engine source code
├── README.md  NEXUS_ENGINE.md  RELEASE.json
```

Your own data is deliberately **not** inside this folder (so an update can never overwrite it): users and profiles, **saves** (`UserData\Saves`), **screenshots** (`Media`), settings and per-game configuration, artwork, themes and wallpapers, keys and firmware, logs are in `%APPDATA%\NEXUS\` (*System Settings › System › Storage & data* opens each folder).

**Saves folder layout:**

```text
%APPDATA%\NEXUS\UserData\Saves\
├── C36C52B3FCFA09313E76441554C5F2B5-Tom\
│   ├── 0400486024DEC000-OOT\
│   │   ├── slot_00\ … slot_05\
│   │   ├── account.sav
│   │   └── option.sav
│   └── 0100BBBB00010000-CG\
└── 2FAF7BE653FF9433ACF8395D4EA84182-Jo\
```

**Portable mode:** an empty `portable.txt` next to `NEXUS.exe` keeps all data inside the NEXUS folder.

## 4. Using NEXUS

### Controls

| Action | Controller (Xbox layout) | Keyboard | Mouse |
|---|---|---|---|
| Move | D-pad / left stick | Arrow keys | Hover |
| OK / Play | A | Enter / Space | Click |
| Back | B | Esc / Backspace | Click the dimmed area |
| Options (game details, user settings) | Y | X | — |
| Power menu | Menu (Start) | Esc on the home screen | Power icon |
| Search the library | X | `/` | Search box |
| Change sort or settings tab | LB / RB | Q / E | — |
| Home | Guide (if your pad reports it) | H | — |
| Full screen / windowed | — | F11 | Power menu |
| Start / stop recording (while playing) | — | F12 | — |
| Screenshot (while playing) | Share / Capture | Alt+F12 | — |
| **Quit a running game** | Hold **View + Menu** (Back + Start) for 2 s | *Close game* on the NEXUS screen (Esc while playing) | — |

In *Controllers*, you can switch to Nintendo-style confirm (A on the right) and remap every NEXUS action to any button.

### Home screen
The 1.6 layout takes its structure from modern console home screens but keeps NEXUS's own look: dark theme, NEXUS red accent and its own icons.
- **Top left:** users. Your picture is shown larger. Select another user to switch, or select **yourself** to open your **profile page** (see below).
- **Top right:** controllers, network, battery (laptops) and the clock.
- **Middle:** your 12 most relevant games as large rounded tiles with more space between them. Recently played games come first, marked with a dot, and the order can be changed in *Appearance › Home screen order*.
  - The selected game lifts slightly and gets a red ring with a small gap around it.
  - Its name and play time appear above the row.
  - The last tile opens **All Software**.
- **Dock:** a single pill-shaped bar: Library, Users, Controllers, Themes, System & Emulation, System Settings and Power. The selected item's name appears under the bar.
- **Power** (Esc / Menu): Switch user, full screen or windowed, Minimise, Replay startup animation (plays in place) and Exit.
- No engine version numbers appear on the home screen. Versions are in *NEXUS Engine* and *System files*.

Existing users still on the old default accent (cyan) are moved to NEXUS red once. An accent you chose yourself is kept, and you can change it any time in *Appearance*.

### First start and adding users
When NEXUS starts with no users, it opens **Welcome to NEXUS**: choose a **name** (up to 20 characters, no duplicates), a **picture** (a NEXUS icon with a background colour, a console icon, or a picture from your PC), and your **profile colour**, then check the **review** and press *Start using NEXUS*. The user is created only at that last step. *Add user* on *Who's playing?* opens the same steps. NEXUS supports **up to 8 users**; with 8, *Add user* is hidden and NEXUS explains that a user must be deleted first. Everything can be changed later in *User settings* (name, colour, icon, picture).

### System files
NEXUS keeps its engine data in `%APPDATA%\NEXUS\System\Engine` and creates it, with its configuration, at the first start. Keys and firmware dumped from your own console are installed in *Settings › System files*: NEXUS checks them before anything is replaced and keeps the previous files. NEXUS does not read other emulators' folders and never asks you to open another emulator.

### Choosing a user
Each user on *Who's playing?* has a card of their own, evenly spaced, with the picture centred. The red focus ring, the slight zoom and the hover highlight all stay inside that card, so moving between users never overlaps a neighbour; the change animates smoothly. The user who played last carries a **LAST** label. The cards wrap into more rows as users are added, and shrink on small windows.

### Profile page and play statistics
Select your own picture at the top left. The profile page shows:
- your name and icon, with *Edit profile* and *Change icon*;
- **total play time, games played, number of play sessions and last played date**;
- **recently played** games and a **most played** chart;
- a **play time per game** table. Select any game for its details: hours played, sessions, average session, first and last played, the **most recent session** (start, end, length) and the last 25 sessions. Time played with NEXUS 1.x is kept and labelled as such.

*Game Information* on each game shows the same per-game numbers.

**How play time is measured:** the clock starts only when the NEXUS engine is running the game and presenting frames, not while it loads. It stops as soon as the game stops. Loading time is not counted. It is recorded per user.

**Crash protection:** while you play, NEXUS saves the running session every 30 seconds. If NEXUS, the game or the PC crashes, the next start credits the session up to the last save (at most 30 seconds are lost) and marks it *recovered*. Statistics live in `%APPDATA%\NEXUS\Profiles\<user>\stats.json`, so they survive NEXUS and engine updates.

### Game library
All Software shows every game, with search, sorting (recent, A–Z, play time) and refresh. Press **Y/X** on a game to:
- choose your own artwork or reset it
- rename it
- show the file in Explorer
- see its title ID, version, format and size

Update and DLC files are hidden by default because they are not games you launch. Keep them in one of your game folders: when you start the base game, NEXUS hands its updates and DLC to the engine automatically.

**Supported formats** (checked against the engine's file loader): NSP, XCI, NSZ and XCZ (block-compressed), NUP, NCA, NSO and NRO. Each format can be turned on or off in *Game library › Formats*.

**Where artwork and names come from**, in priority order:
1. Artwork you picked in NEXUS.
2. An image beside the game with the same name, e.g. `Games\My Game.nsp` + `Games\My Game.png`.
3. `%APPDATA%\NEXUS\Artwork\<TitleID>.png` or `%APPDATA%\NEXUS\Artwork\<Game name>.png`.
4. The icon and name **inside the game file**. NEXUS reads the game's control data locally using your `prod.keys`. NRO homebrew icons need no keys.
5. The game-list icon cache of the emulator NEXUS 1.x used, if it was imported.
6. A generated coloured tile.

### Users, saves and isolation
Each user has their own name, picture, theme, wallpaper, controller mapping, play time and recently played list. Each user also has **their own save data**:

- When a game starts, NEXUS gives the engine that user's save folder (`UserData\Saves\<PLAYER_ID>-<name>`), so games read and write only that user's saves. Keys, firmware and installed updates stay shared.
- **Deleting a user never erases saves.** You must type the user's name to confirm, and the user's folder and saves are moved to `%APPDATA%\NEXUS\Archived`.

### Profile icons
NEXUS reads its profile icon gallery from the console's own **installed firmware**: system data archive `010000000000080A`, folder `chara`, Yaz0-compressed 256 × 256 images (the same source the console's avatar picker uses).

How NEXUS sets the gallery up:
1. **Automatically, with no upload.** NEXUS checks for the icons when it starts, after a game ends, and every 20 seconds until firmware and `prod.keys` are present. Then it decrypts the archive with your keys, decodes every image and saves them in `%APPDATA%\NEXUS\Avatars\firmware`. The gallery appears in the picker immediately.
2. **Cached.** After that the icons work offline. If the firmware changes, or the cache is deleted, NEXUS rebuilds it automatically.
3. **Nothing installed yet?** The picker says exactly what is missing (firmware or keys), with *Try again* and *System files* buttons. It never quietly substitutes generic icons.

The picker has:
- categories: *All console icons*, plus groups by series when the firmware's file names allow it;
- a search box;
- a grid of thumbnails with the current pick highlighted;
- a large preview with the icon's name;
- background colours, plus backgrounds if the archive contains them.

Your choice is saved per user and reopens selected after a restart. A new user starts with nothing selected, so you choose the icon yourself.

**This build includes your icon pack:** 148 Switch profile icons in 11 categories (Super Mario, The Legend of Zelda, Kirby, Splatoon, Animal Crossing, Pikmin, Metroid, Star Fox, ARMS, Nintendo Labo, More), plus the console's 72 background colours. They are in `Resources\ProfileIcons`. They are Nintendo artwork from The Spriters Resource sheet you supplied, so this build is **for personal use only**. See `Resources\ProfileIcons\NOTICE.txt`, and delete that folder before sharing NEXUS.

**Built-in icon packs:** any folder in `NEXUS\Resources\ProfileIcons\` appears in every user's picker without importing. Use `<Category>\characters|backgrounds|frames\*.png` or just `<Category>\*.png`. Ship your own icon pack this way.

*More options* (optional):
- **Use a picture from this PC**
- **Import extra icon packs**
- **Reload console icons from firmware**

**Licensing:** the icons are Nintendo's copyrighted artwork. No source permits redistributing them, and the community collections (Spriters Resource, nso-icons) carry no licence. So NEXUS ships none of them. Reading them from your own firmware dump on your own PC is the same approach the emulator itself takes.

### Game options
Open a game's options with right-click, the **Y** button (**X** on a keyboard), or *Game Information › Options*. The menu has:
- **Play**
- **Configure Game Settings**
- **Edit Game Artwork**
- **Game Information**
- **Open Game Location**
- **Refresh Artwork & Information**
- **Remove from Library**

Removing a game only hides it; the file is not deleted. Restore it from *All Software › Removed*.

### Game artwork
**Artwork priority** (highest first):
1. artwork you chose;
2. an image beside the game or in the Artwork folder;
3. the **built-in NEXUS artwork database**;
4. the icon inside the game;
5. downloaded artwork;
6. the imported game-list cache;
7. a placeholder.

Artwork you pick is never replaced automatically.

**Built-in artwork database** (`Resources\ArtworkDB`, available to every user with no download). It currently holds **your artwork for The Legend of Zelda: Ocarina of Time, the 2026 Nintendo Switch 2 remake**. The image file is byte-for-byte the one you supplied. No official Title ID for the remake has been published (the ones on download sites disagree and can't be trusted), so it is applied by a validated rule instead. NEXUS uses the artwork only when one of these is true:
- the game's own data (decrypted with your keys) names it exactly *The Legend of Zelda: Ocarina of Time* (or "… Remake"), the publisher is **Nintendo**, it is a retail Title ID, not homebrew, and the name contains none of: 3D, Master Quest, Majora, Nintendo 64/N64, Ship of Harkinian, Randomizer, Demo. The original N64 game and Ocarina of Time 3D (3DS) were never released as standalone Switch titles, and fan ports are homebrew or have other publishers, so they cannot match; or
- the game's name came from verified online metadata and passes the same checks; or
- NEXUS could not read the game at all, but its file name explicitly contains both "Ocarina of Time" and "Remake" (and not "3D").

If the game's own data gives any other name, the file name is ignored. Anything uncertain gets no built-in artwork. *Game Information* shows why the artwork was applied. A confirmed Title ID can be added to `index.json`.

**Automatic artwork for other games.** When a new file appears in a games folder, NEXUS tries these in order:
1. **The icon inside the game.** It reads the game's control data with your `prod.keys`; NRO homebrew carries its own icon.
2. **The Title ID.** It is read from the decrypted game header, the ticket, or a `[0100…]` tag in the file name.
3. **Online lookup by Title ID.** If the game has no icon of its own, NEXUS asks the **Nlib API** (`https://api.nlib.cc/nx/<TitleID>?fields=name,icon`, then `/nx/<TitleID>/icon/512`). Nlib is an open-source project whose data comes from the community titledb.
   - The response must echo the same Title ID.
   - If the Title ID only came from the file name, the name must also match.
   - The image must decode as a real, square JPEG/PNG of at least 128 px.
   - Failed lookups are retried after 7 days, or straight away with *Refresh*. Only the Title ID is sent.
4. **Extra providers.** Optional image URL templates containing `{tid}` (*Settings › Game library › Extra artwork providers*) are tried next.
5. **A neutral placeholder** if nothing is found.

**Local artwork database.** Every image downloaded for a Title ID is stored once in `%APPDATA%\NEXUS\Artwork\Database` together with the game's metadata (name, publisher, release date). Nlib is asked for `name,icon,publisher,releaseDate`.
- If the game file moves, a second copy appears, or its folder in GameProfiles is lost, the artwork is restored from the database without going online.
- *Refresh* still asks the server again.
- *Settings › Game library › Artwork database* shows its size, lists the built-in entries and the games they matched, and can clear the downloaded copies. Your own artwork and the built-in artwork are kept.

*Edit Game Artwork* shows every source side by side: the NEXUS database (when it applies), the game file, downloaded artwork, and your image. You can:
- pick an image from your PC (any size; the original is kept at full quality, and 512 × 512 or larger is ideal);
- choose *Fill the tile* or *Show whole image*, so non-square images keep their aspect ratio;
- reset to the original embedded icon or to the downloaded artwork;
- download again;
- remove your image or the downloaded image.

Images beside a game (`My Game.png`) or in `%APPDATA%\NEXUS\Artwork\<TitleID>.png` are still used too.

### System & Emulation settings
Everything to configure games is in NEXUS.

- **NEXUS Engine:** engine status, supported systems (*Nintendo Switch: engine core*; *Nintendo Switch 2: experimental, untested by NEXUS*), renderers and components, installed engine builds (install from a zip or an https link with optional SHA-256, activate, go back to the previous build, remove), data folders.
- **System files:** install, validate, replace or remove prod.keys, title.keys and firmware, with version detection and clear errors (wrong console's keys, incomplete firmware, missing SystemVersion, keys too old for the firmware). Files are verified before anything is replaced; old copies are archived. NEXUS never includes keys or firmware.
- **Emulation settings:** global Graphics, Advanced Graphics, Audio, Controls and System values. Only settings the engine really applies are offered; others are shown as *Not available* with the reason.
- **Game sessions & saves:** game volume, full screen, quit shortcut, per-user saves.

**Volume** (0–200 %): 0 % is silent, 100 % is the game's own level, up to 200 % boosts quiet games. The value is applied inside the engine's audio output (not a Windows mixer trick) and above 100 % a soft limiter keeps peaks below full scale. It is saved and restored on the next start, and can be changed while playing from the NEXUS playing screen (drag, click, or D-pad left/right in 5 % steps; click the speaker to mute).

### Hardware information and "Optimise for my device"
*Emulation settings › Hardware* shows what NEXUS detected. Anything Windows cannot report is shown as *Not detected*; nothing is guessed.
- **Processor:** manufacturer, model, cores, threads and instruction sets (SSE4.2, AVX, AVX2, FMA3, AVX-512…).
- **Graphics:** each GPU's manufacturer, model, dedicated or integrated, dedicated video memory, driver version, and Vulkan version (via DXGI and the Vulkan driver).
- **Memory:** installed, available and in use.
- **System:** Windows edition, version and build; each display's resolution and refresh rate; free space.

*Emulation settings › Optimise for my device* (also in each game's *Configure Game Settings › General › Optimise this game*):
1. NEXUS rates the device: GPU tier from dedicated video memory (integrated GPUs separately; modern ones such as Radeon 780M or Iris Xe are treated as entry-level); CPU from threads and AVX2; RAM; display resolution and refresh rate.
2. It chooses values only for settings the engine applies:
   - graphics API, console mode and resolution scaling;
   - upscaling/downscaling filter, anti-aliasing, anisotropic filtering and VSync;
   - shader cache, asynchronous shaders, GPU accuracy and reactive flushing;
   - ASTC decoding, video-memory mode, multicore and CPU accuracy.
3. It applies the profile:
   - *Performance* lowers the rendering load;
   - *Balanced* (recommended) aims for the best overall result;
   - *Quality* uses higher settings where the hardware has headroom.
4. It adjusts for the game, if one is selected:
   - known demanding games get one step less resolution;
   - games that have crashed in NEXUS get accuracy-first settings.
5. It lists every change (current → recommended, with the reason). You switch off any you don't want, and apply.
6. *Restore previous settings* undoes the last optimisation, for all games or for that game. Per-game optimisation only changes that game; settings that can't differ per game are left to the global optimisation.

It is a recommendation based on hardware, not a benchmark: NEXUS does not promise a frame rate.

### Configure Game Settings
A full page per game: **General** (title, version, Title ID, type, file, **compatibility** with your rating), **Graphics**, **Advanced Graphics**, **Audio** (including the game's own volume), **Controls** (a saved controller profile for this game) and **System**. Every value can follow the global setting or be set for this game only; *Restore defaults* returns the game to global. Settings are applied through the session configuration the engine reads at launch, so they also work for homebrew.

**Compatibility** ratings: *Works* (≥ 10 minutes over ≥ 2 sessions without a crash), *Starts*, *Known issues* (has crashed), *Unsupported* (never reached the first frame), *Not tested*. Your rating always overrides the automatic one. NEXUS shows no ratings copied from other projects.

### Saves
*Settings › Save Data* (or *User settings › Save Data*) shows one user at a time: each game with its **game ID**, size, last change and the save's own slots and files (*Slot 00*, *Account Data*, *Option Data* …).

- **Import Save from ZIP** (or a save folder): NEXUS inspects the archive, finds every save in it however deeply it is nested, and shows which **player** and **game** each belongs to (from the folder names' IDs, or from the save's own metadata file). You select which saves to import. Unsafe archives (paths leading outside, links, password protection, unusual compression ratios) and damaged ones are refused with an explanation, and every file is checked before anything changes: if one save cannot be read, nothing is imported. If the game cannot be identified, you choose it — NEXUS never guesses. A game that already has a save is only replaced when you confirm, and a backup is made first.
- **Export** (one game or all), **Back up**, **Restore…** (the current save is backed up first), **Delete…** (backed up first), **Open** the folder.
- Backups: `UserData\Backups\Saves\<player folder>\`.
- If two copies of a game's save exist (for example after an older engine wrote one in its own layout), both are kept: the second is shown as *other copy* and *Use this copy* switches them.
- **From NEXUS 0.2:** at the first start of 0.3 the old `Saves` folders are backed up into one ZIP, then converted to the layout above (renamed, nothing copied or deleted). If the backup cannot be written, nothing is changed and Save Data says so.

### Themes and wallpapers
These settings are per user (*System Settings › Appearance*):
- Dark (default) or light mode, an accent colour and animation level.
- **Animated wallpaper:** import a GIF, animated WebP, MP4 (H.264) or WebM.
  - You see a preview behind sample tiles before applying it; it loops behind the Home screen.
  - It plays only on Home, Library and the user screen while NEXUS is in front. It stops when a game is running, NEXUS is minimised or in the background, you open another screen, or *Interface animations* is Off. GIF/WebP decoding stops completely; videos pause.
  - A still frame is kept for those moments, and as a fallback if the file can't be played.
  - *Play animated wallpaper* turns the animation off for maximum performance.
  - *Use* switches between the static and animated wallpaper; you can remove either.
  - The files are not re-encoded, so a 1080p video is lighter than a 4K one; NEXUS points this out in the preview.
- **Home transition:** Zoom (default), Fade, Slide or Off (*Appearance*). It is used whenever the Home screen opens. Pressing Home (Guide button on Xbox-compatible pads, or H) **during a game pauses the game** (the engine suspends emulation and hides its window; the session stays loaded and paused time isn't counted as play time) and shows Home with a "paused" card: *Resume* returns to the game, *Close game* ends it. Starting another game while one is paused asks first.
- **Wallpaper:** pick any PNG, JPG or WEBP from your PC. It is copied into `%APPDATA%\NEXUS\Wallpapers` and survives restarts. You can set brightness, blur and scaling (fill, fit, stretch, centre, tile), or reset to the default background. A scrim keeps text readable.

Advanced: any `.css` file in `Resources\Themes\` is loaded after the built-in styles. See the example there.

### Startup animation and logo
The startup is built around **the NEXUS SWITCH emblem you supplied**. NEXUS uses it unchanged: original proportions and no extra text on the emblem. It is also the app icon, the window and taskbar icon and the favicon.

The sequence, on red by default:
1. The screen powers on to the startup colour with a soft glow.
2. The emblem appears centred on screen: its two halves slide in from either side and click together, each with a sound.
3. The joined emblem pops and rings of light pulse outward.
4. The logo rises while **NEXUS** fades up and **SWITCH** opens out from the centre beneath it. Both words are set in **Outfit** (SIL Open Font License), a modern geometric typeface; the emblem, timings, layout and colours are unchanged.
5. A light sweep crosses the logo, which then fades smoothly into the home screen.

The full animation is about 3.5 seconds, or about 2.5 with *Short*. In *System Settings › Startup* you can:
- change the background colour (the logo turns dark automatically on light colours);
- choose standard or short length;
- mute the sound or turn the animation off;
- **preview it in place** without restarting.

Any key, button or click skips it. A preview video with sound is in `Resources\Animations\`.


## 5. The NEXUS engine

### Launch flow
1. You press Play. NEXUS checks the engine is installed, the file exists, and warns about missing keys/firmware or the VC++ runtime.
2. NEXUS writes the session configuration: global settings + the game's settings + the user's save folder + controller profile.
3. The engine (inside NEXUS) loads the game with its updates/DLC from your library, opens the game window owned by NEXUS, and starts.
4. The playing screen in NEXUS shows the game, *Return to game*, *Close game* and the volume.
5. When the game exits or you close it, the engine shuts down the session and you are back on the home screen. Play time and compatibility are recorded; if the engine reports an error, NEXUS shows it with the last log lines.

### Engine packaging
The engine ships inside the NEXUS folder (`Engine\`) and is loaded with paths relative to `NEXUS.exe`. Its dependencies are searched only in `Engine`, `Engine\Libraries` and System32 — never the current directory or PATH. At the first game launch NEXUS checks every engine file against `engine.json`.

If something is missing, NEXUS explains it instead of failing silently:
- the engine itself missing or damaged;
- the VC++ runtime missing;
- no Vulkan driver.

Developers can add other engine builds (*NEXUS Engine › Add an engine build*); they live in your data folder and you can always switch back to the included engine.

### Engine builds and updates
NEXUS never downloads or swaps an emulator executable. The engine is built from its source (the base pinned in `engine.lock` plus the NEXUS patches) by CI, which runs the engine's tests and an interface smoke test before a build is published. Upstream changes are merged on a separate branch, built and tested, and only then released (see `NEXUS_ENGINE.md` §4). In NEXUS, a new build is verified file by file against its manifest and installed beside the old one; *Go back to the previous build* restores it.

## 6. Controller support

- **NEXUS menus:** any XInput/standard gamepad, keyboard and mouse; the menu buttons are remappable in *Controllers*.
- **Games, automatically:** the engine detects controllers through SDL (Xbox/XInput, PlayStation, Switch Pro and most USB/Bluetooth pads) and maps player 1 to the first connected one. With no controller, the engine's keyboard keys work.
- **Your layout for all games** (*Controllers › In-game controls*), saved per user:
  - *Button layout*: **Nintendo positions** (A on the right, as on the console; the default) or **Xbox labels** (A is the button printed A);
  - *Controller type*: Automatic, Pro Controller, Joy-Con pair or Handheld;
  - *Customise buttons…*: each Switch button (A, B, X, Y, L, R, ZL, ZR, +, −, stick presses) can be put on any controller button — choose it from the list or press *Press…* and then the button. NEXUS warns when two Switch buttons share one controller button;
  - *Restore default layout*.
- **A game's own layout** (*Configure Game Settings › Controls › Controller layout for this game › Its own layout*): the same options, used only for that game. Some games have a preset that picks the controller type they need (for example, a Joy-Con pair for Super Mario Party, 1-2-Switch, Switch Sports, Ring Fit Adventure and Labo); the note on the page says what the preset does and what cannot work with a normal controller. Order: the game's own layout, otherwise your layout (plus the game's preset), otherwise the default.
- A saved engine controller profile (*Controller profile for games*) still takes priority when you choose one.
- Hold **View + Menu** for 2 s to close the running game.

## 7. Known limitations

- Hardware detection on Windows (DXGI, Vulkan, display settings, registry) was written against the Windows APIs. It compiles, but it has not been run on a Windows PC yet. The optimiser itself was tested with saved hardware profiles of three different PCs.
- **No real game has been run yet.** The engine is built and loads, but window ownership and focus, a second game in one NEXUS session, audio volume on real output, the controller layouts inside a game and the Joy-Con presets need a PC with a GPU and games. These are the first checks to do with this ZIP.
- **Switch 2:** the engine contains experimental upstream code; NEXUS has not run any Switch 2 game and makes no claim. Results appear per game under Compatibility.
- The quit shortcut needs an XInput controller.
- **Screen recording** was tested on Windows in the project's build with Microsoft's software encoder: generated video and sound, a real test window, the microphone track, and reading the files back. Hardware encoders (NVIDIA, AMD, Intel) and recording a real game have not been tested yet. If a recording fails, NEXUS shows why; turning off *Use the graphics card to encode* falls back to software.
- Recording only the game's sound needs Windows 11; on Windows 10 all sound playing on the PC is recorded.
- Input blocking while a game runs in the background is done inside the engine and was tested with the stand-in engine only.
- `NEXUS.exe` is not code-signed (docs/SIGNING.md in the repository).

## 8. Troubleshooting

| Problem | What to do |
|---|---|
| A game does not start | NEXUS shows the reason; *Technical details* and *Open diagnostic log* (`Logs\diagnostics`) have the engine's own report. |
| "The NEXUS engine cannot be used" | The message says why (missing or damaged files). Extract the complete ZIP again. `NEXUS.exe --engine-check result.json` writes the details to a file. |
| An engine build is refused | It is damaged or for a different NEXUS version; the message says which file. Nothing was changed. |
| A game stops with an error | The message shows the engine's last error lines; full log in `%APPDATA%\NEXUS\Engine\log\eden_log.txt`. Usually keys or firmware are missing or too old. |
| A new engine build behaves worse | *NEXUS Engine › Go back to the previous build*. |
| Controller does nothing in game | Connect it before starting the game, or choose a saved controller profile in *Controllers*. |

## 9. How this build was verified

**On Windows, by the build (every ZIP):**
- the engine compiles and links (`nexus_engine.dll`), all upstream engine test cases pass;
- the engine's C interface is called directly (version, build information, status, volume, suspend, refusing a bad request);
- every DLL the engine imports is either in the package (including the complete VC++ runtime) or part of Windows;
- NEXUS's Go test suite passes on Windows (and on Linux);
- after a fresh extract of the ZIP, `NEXUS.exe --engine-check` finds the engine next to it, verifies every file against `engine.json`, loads it into NEXUS and reads its build information.

**In a browser against the stand-in engine:** first-time setup on an empty data folder (name → picture → colour → review, empty and duplicate names refused); the picture and colour saved and shown (colour ring in the top bar); 8 users allowed, a 9th refused and *Add user* hidden; users, colours, pictures and both kinds of controller layout kept after a restart and after replacing the NEXUS folder (data folder kept); the Controllers and game Controls pages and the button editor in dark and light; the play-time list in both themes; an automated contrast check of 20 screens in both themes.

**Earlier checks, in Go tests and a browser walkthrough:** NEXUS ran against a stand-in engine with the same states and audio path:
- **Bundled engine:** found relative to the NEXUS folder after the folder was moved; damaged library files detected; added builds and rollback to the included engine.
- **Release script:** run with a stand-in engine. It produced one ZIP with `Engine\` inside, and its self-checks passed.
- **Optimiser:**
  - recommendations checked for three hardware profiles (weak laptop, mid-range desktop, high-end 4K) and a PC without Vulkan;
  - apply → the engine's session configuration changes;
  - persists after restart;
  - restore puts the earlier values back, including "not set";
  - per-game optimisation leaves other games and the global settings alone.
- **Saves:**
  - migration of 2.0 folders, profile renames and readable game names;
  - a game session moves folders to the engine paths and back, including new saves;
  - crash recovery;
  - drop-in saves placed for the console user;
  - export and import without overwriting;
  - conflicts keep both copies.
- **Animated wallpapers:**
  - MP4, WebM, GIF and animated WebP imported, previewed, applied, looping and removed;
  - paused or stopped on other screens and while a game runs;
  - a single-frame GIF is refused;
  - about 54 frames per second while navigating with a video wallpaper (headless Chrome, no GPU).
- **Home:**
  - Home while playing suspends the game, shows Home with the transition and a paused card;
  - *Resume* returns to the game;
  - paused time isn't counted;
  - transitions can be switched off.

**Not tested yet:** running real games (needs a PC with a GPU, your keys/firmware and games); the game window, controller input in a game and audio output with the real engine; hardware detection was exercised on the Windows build machine (CPU, memory, display, Vulkan loader; it has no GPU).

## 10. Licensing

- **NEXUS is free software under the GNU GPL v3.0 or later** (`Licences\NEXUS-GPL-3.0.txt`), because it loads the GPL engine in-process. Full source: `Source\NEXUS-source.zip`.
- **The NEXUS engine** is GPL-3.0-or-later, built on Lichii (github.com/zortos293/Lichii), Eden, yuzu and other projects. Corresponding source: `Source\engine\nexus-engine-source.tar.gz` (the exact tree that was compiled: the pinned upstream base with the NEXUS patches applied); the patches themselves are in the NEXUS source (`engine/patches`). Component licences: `Licences\engine-third-party\`.
- **Ryujinx** (MIT) is used only as a technical reference; no Ryujinx code is included. Its licence is kept in `Licences\` for any fix ported in future.
- Libraries in NEXUS.exe: go-webview2 (MIT, with Microsoft's WebView2Loader), go-winloader (ISC), golang.org/x/sys and the Go standard library (BSD). Fonts: Inter and Outfit (both SIL Open Font License 1.1).
- "Switch" and "Nintendo Switch" are trademarks of Nintendo. **Personal build:** the NEXUS SWITCH emblem, `Resources\ProfileIcons` and the Ocarina of Time artwork are artwork you supplied (Nintendo designs); replace/remove them before sharing (see the NOTICE files).
- You are responsible for owning the games, keys and firmware you use.

## 11. Building from source

**Release (one ZIP):** the GitHub repository's `.github/workflows/nexus-build.yml` (Windows runner) checks out upstream Lichii at the commit in `app/engine.lock`, applies `engine/patches/*.patch` in order, builds `nexus_engine` and the engine tests with clang-cl (as upstream's Windows builds do), runs the tests and the interface smoke test, packages the engine (`tools/nexus/package_engine.py`, recording the upstream base and the SHA-256 of the patch series), runs NEXUS's tests, builds `NEXUS.exe`, calls `tools/release/make_release.py` (which refuses an engine that is damaged, of another interface version, or not built from this patch series) and finally checks a fresh extract with `NEXUS.exe --engine-check`. The ZIP is the run's artifact; a `v*` tag publishes it as a release.

NEXUS: Go 1.24+; `GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui -s -w -X main.AppVersion=0.0.0" -o NEXUS.exe .`; `go test .` (the tests use the stand-in engine; `NEXUS_FAKE_ENGINE=1 go run .` starts the UI for development on Linux).

Engine by hand: apply the patches to the upstream base in `engine.lock` (currently build-160) and build on Windows (Visual Studio 2022 + LLVM clang-cl + Vulkan SDK): `cmake -B build -G Ninja -DNEXUS_ENGINE=ON -DYUZU_TESTS=ON -DENABLE_QT=OFF -DYUZU_CMD=OFF -DCMAKE_C_COMPILER=clang-cl -DCMAKE_CXX_COMPILER=clang-cl`, then `cmake --build build --target nexus_engine tests`.
