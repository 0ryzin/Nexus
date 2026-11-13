# Development notes

## Layout

| Path | What |
|---|---|
| `app/` | NEXUS (Go 1.24): `main.go` starts the core (`StartCore`), `server.go` the local HTTP API, `web/` the interface (vanilla JS, `app.js`, `app.css`) shown in WebView2 (Windows) |
| `app/engine*.go` | Game sessions: `engine.go` (launch, monitor loop, Home, F11, background limit), `enginehost_windows.go` (loads `nexus_engine.dll`, C ABI v2), `enginehost_dl.go` (Linux/Android dlopen), `enginehost_fake.go` (tests) |
| `app/enginesettings.go`, `engine_options.go` | NEXUS settings → engine ini keys (`writeSessionConfig` writes `config/session.ini` per launch) |
| `app/systemfiles.go`, `systemvault.go` | Keys and firmware install; the per-user vault that keeps them across versions |
| `app/library.go`, `gameinfo*.go`, `statsmerge.go` | Game scanning, identification (Title ID), play statistics |
| `app/saves*.go` | Save organiser, ZIP importer |
| `app/account*.go`, `accountd/` | NEXUS Account (GitHub device flow + private Gist; or the self-hosted service) |
| `app/amiibo.go` | Amiibo library, virtual amiibo, hand-off to the engine |
| `app/overlay_windows.go`, `perf.go` | In-game HUD (performance strip, REC) |
| `engine/patches/` | NEXUS changes to the engine, applied by CI on top of Lichii `build-160` |
| `android/` | Internal Android source (no release) |

## Where user data lives

- `%APPDATA%\NEXUS` (Linux `$XDG_DATA_HOME/NEXUS`), or the NEXUS folder with `portable.txt`.
  `System/Engine` is the engine's data (keys, NAND with firmware, shader caches, config);
  `System/UserData` profiles, saves, amiibo; `System/Account` the account state.
- Keys and firmware are also kept in the vault `%LOCALAPPDATA%\NEXUS\SystemFiles` (Linux
  `~/.local/share/NEXUS-SystemFiles`) and restored into any copy of NEXUS at start
  (`SyncSystemVault`): a new version in another folder, portable or not, finds them.

## Game launch (engine.go)

`Launch` → `writeSessionConfig` (global ini + per-game values + save directory) → engine
`nexus_engine_start` (JSON request: game, contents, controllers, keys, embed, owner window) →
monitor loop polls `nexus_engine_status` (phase, frames, embedded, presses, amiibo) → reveal after
the first frame → Home (`Suspend`: background or pause) → stop. The engine host is
`src/nexus_engine/nexus_engine.cpp` in the engine (patches 0001-0017).

## Build and test

- `cd app && go test ./...` (fast, fake engine) and `go test -race ./...`.
- Windows: CI `nexus-build` builds the engine (about 45-60 min, cached) and NEXUS, runs both test
  suites, packs `NEXUS-<VERSION>-Windows-x64.zip`. A release is published only when
  `docs/releases/v<VERSION>.md` exists (pre-release if it contains `<!-- nexus:prerelease -->`).
- Engine patches: edit the fork (branch `nexus` on top of `build-160`), then
  `git format-patch --binary --no-signature -o engine/patches build-160..nexus` and check they
  apply with `git am` on a clean `build-160` worktree.
- Real games cannot be run in CI (no keys, no GPU): anything game-specific needs a test on a PC.
