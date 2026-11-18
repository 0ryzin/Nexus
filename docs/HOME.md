# Home screen: layouts, organisation and customisation

*Settings › Home screen*, and a game's Options menu › **Organise** (`app/web/home.js`,
`Profile.home` in `app/profiles.go`).

## Layouts

| Layout | What it is |
|---|---|
| **Classic** | The original NEXUS Home: one row of game icons (6–24, *Games on the Home row*) and the shortcut bar along the bottom. The default. |
| **Cards** | Large game cards in a row. Under them, a frosted panel for the selected game: icon, name, play time, last played, version, **Play**, **⋯** (its menu). Optionally the selected game's background picture fills the screen, with its title picture over it (or its icon, blurred). Shortcuts down the left side. |
| **Grid** | Every game in a grid, with folders first, the panel and widgets under it, and shortcuts down the left side. |

Switching layouts never rescans the library: they all draw the games NEXUS already has.

## Organisation (per user)

- **Order:** recently played, title A–Z, play time, date added (when NEXUS first found the file),
  or **My order**. *Organise › Move earlier / later* builds My order from what Home shows.
- **Favourites:** *Organise › Add to Favourites*. Favourites get a star and a page of their own.
- **Folders:** *Organise › Add to a folder…* (or a new one). A game in a folder is shown in the
  folder, not loose on Home. Opening a folder shows its games; it can be renamed or deleted
  (its games return to Home).
- **Pages:** the main page, Favourites, Recently played, and your own pages (*New page…*, then
  *Organise › Add to a page*). Switch pages with the shoulder buttons (LB/RB, L1/R1, L/R) or Q / E.
- **Hide from Home:** the game stays in All Software; *Games hidden from Home* shows them again.
- **Shortcuts:** the order of the shortcut buttons, and which are shown. *System Settings* always
  stays, so nobody locks themselves out. There are no console functions (sleep, airplane mode):
  only NEXUS's own sections (Library, Users, Account, Media, Controllers, Home screen, Themes,
  Shaders, Performance, System & Emulation, Settings, Power).

## Look

- **Panels:** frosted glass, or flat (no blur: the lightest on the graphics card).
- **Selected game's picture behind the cards** (Cards).
- **Plain background colour**, instead of the wallpaper.
- Wallpaper (your own image, animated or still, dimming, blur, fit), dark or light theme and accent
  colour stay in *Appearance*, as before.
- **Widgets** (Cards and Grid, optional): selected game, recently played, total play time,
  favourites, recently added, library size. They are drawn when Home is drawn, from data NEXUS
  already has: no timers, no requests.
- **Restore the default Home screen:** Classic, default panels, widgets and shortcuts. Folders,
  pages, favourites and hidden games are kept.

## Per game (Options › Edit Game Artwork)

Card picture (as before), **background picture**, **title picture** (a logo, ideally a PNG with
transparency) and **display name**. They are stored in `System\GameProfiles\<title id>\custom-*`.
Game files are never changed. Artwork and names belong to the game, so they are the same for
every user of this NEXUS. Layout and organisation belong to the user.

## Profiles

Everything in *Organisation* and *Look* is stored in the user's profile: one user's Cards and
folders never change another user's Home. Play time, saves, controls and the theme were already
per user. *Ask who's playing a game* (*Playing games*; by default whenever there is more than
one user) chooses the user when a game starts.

## Controller

Every layout works without a mouse:

- the D-pad or stick moves through cards, the grid and folders;
- Down from the lowest games goes to the selected game's **Play**, and Up goes back;
- the shoulder buttons switch pages;
- **Y/X** (Options) opens the game's menu;
- the settings are ordinary NEXUS rows.

Focus is a ring; the mouse only shows a hover.

## Images

Home uses the downscaled, cached thumbnails NEXUS already makes (`/art/<key>?w=…`, `thumbs.go`),
loaded lazily. Background and title pictures load only for the selected game.

## Tested

- `home_test.go`: per-user preferences, validation, kept on disk.
- `tools/uitest/home_layouts.py` (CI): the three layouts, a background picture, folders, pages,
  favourites and widgets, controller movement, the shoulder buttons, opening a folder, kept after
  a reload, Restore.
- Idle cost: docs/PERFORMANCE.md.
- **Not tested on Windows with WebView2** (Linux, headless Chrome only).

## HomeNX: what was taken and what was not

HomeNX (github.com/ad3333m/homenx, GPL-3.0) is a HOME menu replacement for modded Switch consoles,
written in C++. It has two looks, CardsNX and the "Wii U look". It was studied from its README,
docs (`cards-personalization.md`, `cards-artwork-performance.md`) and screenshots (`cardsnx.jpg`
and `1.jpg`–`5.jpg` in its repository).

Ideas taken, in NEXUS's own design and code:

- big cards with a frosted panel for the selected game, and that game's art behind them;
- per-game card, background and title pictures, and rename;
- custom pages switched with the shoulder buttons;
- hiding a card from Home;
- a grid with folders and widgets, and a configurable side bar;
- glass effects that can be turned off;
- sorting.

Not taken:

- its code and assets (themes, icons, sounds): none were copied, so its licence places no
  obligations on NEXUS;
- console functions (sleep, airplane mode, Wi-Fi, reboot, eShop, Mii Maker, Discord, Spotify,
  lock screen): they do not belong in a Windows emulator front end;
- SteamGridDB downloads.

## Game menu with a controller (18 Nov)

On Home, the menu a right-click opens on a game (Play, Game Settings, Manage Mods, Organise,
Save Data, Game Information, shaders, More…) opens with the controller's **X** (the Search button,
`homeContextMenu` in `web/app.js`) on the focused game, and also from the Play button of the
Cards / Grid details panel. Options (Y) and the keyboard's X still open it; right-click is
unchanged. The hint bar shows "X Game menu". X elsewhere on Home still opens the Library search.
Tested with a scripted controller (`tools/uitest/oot_menu.py`), not with a physical pad.

## OOT Launch startup animation (18 Nov)

`web/bootoot.js`, chosen in Settings › General › Startup animation (`startup.animation`: `nexus`
default | `oot`; anything else falls back to `nexus`). Modelled on the timing of the reference
video of a special-edition console's launch (about 6.7 s): 0.2 s red power-on; 0.7–1.9 s a pixel
emblem changing colour four times with four notes; 2.5 s it becomes a white shield; 2.8 s the
shield fills green and gold (40TH / ANNIVERSARY) over a mosaic of triangles; 3.6 s a light sweep;
4.45 s white; 5.3 s gold; 5.65 s a dark gold triangle pattern; 6.25 s Home. Any key or button
skips it. With *OOT Launch background on Home* (`startup.ootHome`, on by default) the dark gold
pattern stays behind Home unless the user chose a wallpaper (drawn once, about 20 ms).

Everything is drawn with CSS, SVG and canvas; the sound (`web/assets/sounds/oot_launch.wav`) is
synthesised by `tools/brand/make_oot_sound.py` and follows the same marks. The reference video's
audio and artwork are not used, and Nintendo's characters, logos and wording are not reproduced
(the user asked for Link and "The Legend of Zelda" in the crest; that was declined). The sound is
loaded only when this animation is chosen.
