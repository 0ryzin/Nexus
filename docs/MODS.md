# Mods (Manage Mods)

Each game's **Options** menu has **Manage Mods** (`app/mods.go`, `app/gamecontent.go`,
`web/app.js` `ModsUI`).

## How mods are loaded

NEXUS uses the engine's own add-on mechanism: layered mods. It adds no new loader.

- Each mod is a folder in `<NEXUS data>\System\Engine\load\<GAME ID>\<mod name>\`, which can
  contain:
  - `romfs/` and `romfs_ext/`: replaced or added game files;
  - `exefs/`: replaced ExeFS files, or `.ips` / `.pchtxt` code patches named after a build ID;
  - `cheats/`.
- The game's own files are never changed. The engine layers the mods over them while the game runs.
- Mods are per game: the folder is the game's ID, so a mod of one game cannot reach another.
- Switching a mod off adds it to the game's list of disabled add-ons, which the engine skips. The
  mod stays installed.

## Installing a mod

You can import a ZIP or a folder.

- **Checks before installing.** The whole archive is checked before anything is written:
  - an entry with `..`, an absolute path, a drive letter, a link or a special file refuses the
    whole archive;
  - so does more than 16 GB unpacked.
- A mod whose folder names another game's ID is refused.
- A mod with the same name as an installed one is refused; nothing is overwritten.
- **Removing** moves the mod to the Recycle Bin. Saves are not touched.
- Installing and removing need the game closed. Switching a mod on or off is allowed while the game
  runs and takes effect at its next start.

## What NEXUS shows for each mod

- **Information.** From an optional `nexus-mod.json` in the mod folder, with these fields:
  - `name`, `description`, `version`, `author`;
  - `category`: fps, performance, resolution, visual, gameplay or other;
  - `gameVersion`, `buildIds`, `requires`, `website`.
  
  Otherwise NEXUS reads what it can from the mod itself: files replaced, and the builds its code
  patches are for.
- **Compatibility.** Code patches apply to one build of the game.
  - NEXUS learns the game's main build ID from the engine log of its last session and compares.
  - "Made for this game's build", "Made for build X, but this game is build Y" (the engine skips
    it), or "checked once the game has been played". Nothing is guessed.
  - Mods that only replace files have no build to check.
- **Conflicts.** Two switched-on mods that replace the same file. Only one of them takes effect.
- **Requirements.** A required mod that is missing or switched off is named.
- **Last start.** Whether the engine applied the mod or skipped it, and why, as read from its log.

## Built-in mods

The NEXUS engine includes Lichii's built-in patches (`core/file_sys/patch_manager.cpp`,
`BUILTIN_PATCHES`). They are listed only for the game and the build they were made for. Today
that is only Ocarina of Time (`0400486024DEC000`, build `E0F3CCFAF50065AE…`):

| Patch | What it does (from the engine source) |
|---|---|
| Max Resolution | The 3D renders at full scale instead of dynamic resolution. Needs Texture Pool x2. |
| Texture Pool x2 | Doubles the game's texture pool. Without it, Max Resolution crashes in the guard house. |
| FPS Unlock | Lets the game run above its frame-rate cap at its real speed. Contains Dynamic FPS. |
| Dynamic FPS by Dark | Game speed follows frame time. Skipped while FPS Unlock is on. |

- They can be switched off, but not removed.
- No frame-rate or performance gain is claimed for them; none has been measured by NEXUS.
- The user's 17 Nov engine log shows the first three applied to build `E0F3CCFAF50065AE…`, and
  Dynamic FPS skipped as included.
- NEXUS does not download or bundle third-party mods.

## Tested

- `mods_test.go`:
  - details, metadata, build compatibility from a log;
  - conflicts and requirements;
  - built-ins only for their game;
  - unsafe archives refused whole.
- The UI was checked with the stand-in engine.
- **Not tested** with a real mod in a game.
