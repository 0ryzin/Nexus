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
