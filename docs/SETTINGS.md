# System Settings: structure, search and markers (0.8.0)

How Settings is organised since the refinement of 17 Nov 2026, and where everything moved. No
setting, stored value or per-game override was removed, renamed in storage or reset; defaults are
unchanged. Category ids are unchanged, so links and Home shortcuts keep working.

## Groups and categories

| Group | Category (id) | What is there |
|---|---|---|
| General | General (`general`, new) | Startup animation, choosing the user at start, full screen, interface sounds |
| | Users (`users`) | Users, the account row, archived data |
| Personalisation | Appearance (`appearance`) | Theme, accent, interface animations, Home transition, wallpaper |
| | Home screen (`home`) | Layout, order, look, widgets, folders and pages, shortcuts |
| Games | Game library (`library`) | Folders, formats, artwork (sources in *Advanced*) |
| | Playing games (`emulator`, was *Game sessions & saves*) | Game volume, who plays, Home while playing, shortcuts; *Advanced*: game window, fade-in, quit shortcut, pausing a forgotten game |
| | Save data, Amiibo, Screen recording | unchanged |
| Controllers | Controllers | opens the Controllers screen |
| Graphics & performance | Graphics & emulation (`emusettings`, was *Emulation settings*) | Hardware, optimiser, every engine section |
| | Shader management (`shaders`, new) | Prepare shaders before playing, the shared shader cache |
| | Performance monitor (`perf`) | unchanged |
| Accounts & cloud | NEXUS Account & GitHub (`account`) | Connection status and actions, devices, what synchronises |
| Advanced | NEXUS Engine, System files | unchanged |
| | Storage & data (`system`, was *System*) | Data folders, storage, maintenance |
| About | Updates (`updates`), About NEXUS (`about`, new) | Version, legal, licences |

Moved rows: *Startup* (its own category before) and the interface sounds (were in Appearance)
are in General; full screen (was in System) is in General; *Prepare shaders before playing* and
the shared cache (were in Game sessions) are in Shader management; version and legal (were in
System) are in About. The old `startup` id opens General.

Removed duplicate: *Home screen order* in Appearance was the same setting as *Order of games* in
Home screen; it is only in Home screen now.

## Search

The box above the categories. It indexes every panel by building it off-screen (nothing is
saved), including rows inside *Advanced* sections, plus every engine setting from
`/api/engine-settings` (all sections, not only the one open) and a short list of other words
(`SEARCH_EXTRA` in `web/settings.js`: FPS, GitHub, background, shader cache, Lossless Scaling,
…). A result shows where the setting is (*Group › Category › Heading*); choosing it opens the
panel, opens the *Advanced* section if needed, focuses the setting and highlights it.

## Changed and Advanced

- **Changed:** a small mark next to a setting that differs from NEXUS's default. The defaults come
  from NEXUS (`GET /api/settings/defaults`, the same values *Reset NEXUS settings* uses), and the
  mark follows the value as it changes. Applied to General, Playing games, Game library and Shader
  management settings; the engine settings show their own source (game, NEXUS, engine).
- **Advanced:** a row that shows or hides the less-used options of a panel. Opening one is
  remembered while Settings is open.

## Tests

`app/tools/uitest/settings_search.py` (CI, Linux): every category opens without script errors;
groups; the old Startup id; the six searches named in the brief; a result inside *Advanced*; an
engine setting opening its section; *Changed* following a value; the account panel.

## Restoring defaults (17 Nov, usability update)

- **One setting:** a restore button (circular arrow) next to the value, shown while the setting
  differs from its default; its tooltip names the default. Controller: press Options on the
  focused setting (the hint bar then reads *Reset to default*); keyboard: X. The default is
  saved at once and the panel redrawn. Covered: every NEXUS setting with a stored default
  (General, Playing games, Game library incl. each file format, Shader management, Screen
  recording, Performance monitor, Amiibo, Updates), the user's Appearance, Home screen and
  Controllers settings, each global engine setting (back to the engine default), each per-game
  setting (back to the global one), and each button in the game controller layout editor.
  Not covered: lists of things (folders, pages, devices), the account's sync categories.
- **Reset all settings** (General › Restore defaults, and Storage & data › Maintenance):
  `POST /api/settings/reset-all` (`resets.go`). Resets every NEXUS setting, the global engine
  settings and the current user's appearance, Home look and controller settings. Keeps users,
  saves, play time, games, library folders, keys, firmware, engine files and shader caches, the
  account and GitHub sign-in, wallpapers and artwork files (and which ones are chosen), Home
  folders, pages, favourites, order and hidden games, per-game settings and other users'
  preferences. The theme and window apply at once.
- **Keybinds:** NEXUS's keyboard shortcuts (Home, full screen, screenshot, recording) each have a
  restore button; restoring one whose default another shortcut now uses is refused with an
  explanation. Each NEXUS menu button in Controllers has one (a button another action took is
  swapped back). *Restore all keybinds* (Playing games, General, Controllers;
  `POST /api/keybinds/reset`) resets the shortcuts, the controller Home/Capture buttons, the quit
  combination, and the user's menu buttons and controller layout for games, and nothing else.
  The engine's own saved controller profiles and per-game layouts are not touched.

## On-screen keyboard (`web/osk.js`)

One keyboard for every text field (settings search, Library search, a game's settings search,
dialogs, sign-in forms). It opens when a text field is chosen with a controller (A on the field,
or the Search button on Settings and Library); clicking a field or using a physical keyboard never
opens it, and pressing a physical key while it is open closes it and types the key. Controller:
D-pad/stick move, A types, B closes, X deletes, Y space, LB/RB move the cursor, Menu is Enter,
View is Shift (twice: caps lock); `&123` switches to symbols. Mouse: click keys, *Close*, or
outside the keyboard. Tested on Linux with a scripted controller (Gamepad API) and Playwright;
**not tested on a Steam Deck** (Gaming Mode) or with a physical controller.

## A game's settings: search and overrides

*Configure Game Settings* has a search box and an *Only this game's* filter above its sections.
Results come from every section, grouped by section; each row says **This game** (an override,
stored for the game only) or **Global** (follows Settings › Graphics & emulation). Changing a
row there creates or changes the game's override only; its restore button removes the override.
The existing per-game storage and the engine's precedence (game > NEXUS global > engine) are
unchanged.
