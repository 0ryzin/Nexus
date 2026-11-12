# Plan: from 0.x to NEXUS 1.0.0

**This is a plan. Nothing in it has been done yet.** NEXUS stays on 0.x version numbers until the
conditions below are met. No release, tag or asset is deleted without the owner's explicit
confirmation.

## Where things stand (12 November 2026)

| Tag | Release | Assets |
|---|---|---|
| v0.0.0 – v0.4.0 | Published, Windows x64 | 2 each (ZIP and checksum) |

There are no engine-v tags; the engine version is recorded in `app/engine.lock` and inside each
package. 0.5.0 is in development and not released.

## Conditions for 1.0.0

1. **Windows, tested with real games** on at least one NVIDIA, one AMD and one Intel PC:
   launch, one-window play, F11, Home (key and controller), background sessions, saves, updates
   and mods, recording, screenshots, amiibo with a real dump, the automatic updater from the
   previous release.
2. **No known data-loss bug.** Save import, migration and updates verified, with backups.
3. **Settings stable.** The settings file format is migrated automatically from every 0.x release.
4. **Documentation** matches the release: README, platform page, release notes, licences.
5. **Licences and source.** The complete corresponding engine source and third-party notices
   published alongside the binary.
6. Two consecutive 0.x releases with no regression reported in the areas above.

## Platforms

- **Windows x64:** the only platform 1.0.0 is planned for.
- **Linux and macOS:** listed as available only once a real build exists, has been tested with
  games and is attached to a release. Until then the platform page says "planned".
- **Android:** internal. No APK, release or download link until the owner decides otherwise.

## Releases page tidy-up (needs the owner's confirmation at each step)

- Keep every 0.x tag and its source history. Tags are not deleted.
- Older 0.x releases can be marked **pre-release** (they already say "preview"), so 1.0.0 is
  shown as the latest release. Their assets stay downloadable. This hides nothing and deletes nothing.
- If the owner prefers a shorter page, old releases can instead be summarised in one "Earlier
  previews" section of the 1.0.0 notes, with links. Release entries are only deleted with
  explicit confirmation for each one; deleting a release does not delete its tag.
- The automatic updater keeps working across the change: 0.x installs see 1.0.0 as newer
  (semantic version comparison), which must be tested before publishing.

## Version steps

1. 0.5.0, 0.6.0 … as features land, each with the usual CI checks.
2. 1.0.0-rc.1 (pre-release) once the conditions are met: real-game testing on the three GPU
   vendors, updater from the latest 0.x.
3. 1.0.0, from the same commit as the last release candidate if nothing changed.

The engine keeps its own version (currently 0.6.0) and does not jump to 1.0 with NEXUS.
