# NEXUS: instructions for development sessions

NEXUS is a Nintendo Switch / Switch 2 emulator front end (Go + WebView2 UI, `app/`) around the
NEXUS Engine (a GPL-3.0 fork of Lichii build 316, itself based on Eden/yuzu; built from
`engine/patches` by CI). Read this file first, then the documents it points to.

## Before changing anything

1. Read `docs/DEVELOPMENT_NOTES.md` (how the pieces fit, where things live, how to build and test).
2. Read `docs/REGRESSION_HISTORY.md` for earlier fixes in the same area.
3. For a game problem, read `docs/COMPATIBILITY.md`; for speed, `docs/PERFORMANCE.md`; for
   shaders and their caches, `docs/SHADERS.md`.
4. For engine questions, `app/NEXUS_ENGINE.md` (ABI, patches) and `docs/research/` (what the
   reference projects Lichii and Fritchii changed and why).
5. Investigate (logs, source, history) before changing code. Do not guess.

## Rules that always apply

- Never bundle or upload keys, firmware, games or amiibo dumps. They stay on the user's computer
  (`systemvault.go`).
- Never overwrite or delete save data without the user's confirmation; back up before replacing.
- Do not publish a release, delete releases or tags, or claim something works on hardware or with
  a game it was not tested on. Say what is untested.
- The engine is GPL-3.0: keep upstream copyright notices; when a change is ported from Lichii or
  Fritchii, say so in the commit and in `docs/research/`, with the original commit.
- The project names are **Lichii** and **Fritchii** (exact spelling).
- Commit message footer and PR footer: see `CONTRIBUTING.md`.

## After a verified change

Update `docs/REGRESSION_HISTORY.md` (bugs), `docs/COMPATIBILITY.md` (games),
`docs/PERFORMANCE.md` (measured numbers only) and `CHANGELOG.md`.
