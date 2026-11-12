# NEXUS Device Linking and Local Sync (experimental, hidden)

**Status: implemented and tested in code, switched off for everyone.** Nothing of it is visible or
listening in a normal NEXUS. Testers switch it on with `NEXUS_EXPERIMENTAL=devicelink` or a line
`devicelink` in `%APPDATA%\NEXUS\System\experimental-features.txt`.

## What works (automated tests: `app/devicelink_test.go`)

- **Identities.** Every device has its own Ed25519 key (`System\DeviceLink\identity.json`, never leaves the device). Every NEXUS user has a User ID `NX-XXXX-XXXX-XXXX` (derived from the profile id; *not* a secret).
- **Pairing with approval.**
  - The trusted device shows a one-time code (8 characters, about 40 bits). It is valid for 5 minutes, works once, and is cancelled after 5 wrong attempts.
  - The new device enters the User ID and the code. **SPAKE2** (RFC 9382, P-256) proves that both devices know the code, without the code ever being sent, and nobody can test guesses offline.
  - A wrong code never reaches the user. With the right code, the trusted device asks its user to **Approve / Decline**, showing the device's name and network address.
  - After approval, the devices exchange their identity keys inside the SPAKE2-protected channel.
  - A User ID alone grants nothing. Tested: no code, wrong codes and the lock-out, declined, approved, code reuse.
- **Connections** after pairing: mutual TLS 1.3 with both device keys pinned. A removed device is refused at once (tested).
- **Discovery** on the local network by UDP broadcast, only while a code is shown. Nothing listens on the internet and nothing needs port forwarding.
- **What is synchronised** for a linked user:
  - **Saves:** the user's save folder.
  - **Profile:** name, picture, colour, theme and controller layout.
  - **Play time:** each device's own numbers travel as a snapshot and are added up for display. No device overwrites another's play time.
  - **Never:** games, updates/DLC, keys, firmware or engine and graphics settings.
- **Save safety.** Every file is compared three ways: this device, the other device, and the common version from the last synchronisation.
  - **Changed on one side:** that version is copied over.
  - **Changed on both:** a conflict. Nothing is overwritten, both versions are kept, and the user chooses later (*Keep this device's* or *Use the other's*).
  - **Deletions:** never propagated.
  - **Each transfer:** checked by SHA-256, the replaced file is backed up first (`Archived\sync-backups`), then the new file is written under a temporary name and renamed, so an interrupted transfer never leaves a partial save in place.
  - **While a game is running:** saves are not touched.
- **Interface (only with the feature on):** *Settings › NEXUS Sync*:
  - your devices, with their status, last synchronisation and errors;
  - *Synchronise now* and *Remove*;
  - save conflicts;
  - *Link another device*, which shows the code;
  - *Link this device*, where you enter the User ID and code;
  - an approval prompt when a new device asks to link;
  - first-time setup offers **Create an account** or **Link an existing account**.

## What remains before it can be switched on

1. **Two real devices.** Tests run two NEXUS instances on one machine. Pairing and syncing between two real PCs (Windows firewall prompt on port 47913, Wi-Fi with client isolation, several network adapters) have not been tried.
2. **Background / automatic synchronisation.** Today synchronisation runs when asked, and right after linking. A scheduler (after a game closes, at start, periodically while a linked device is reachable) and a retry policy are still to do.
3. **Conflict propagation.** A conflict is shown on the device that found it. Resolving it there makes the next synchronisation consistent. A second conflict notice on the other device is possible and should be suppressed.
4. **Large saves.** Files travel whole (base64 in JSON). Saves of a few MB are fine. Streaming and resumable transfers are needed for very large saves (limit currently 512 MB per file).
5. **Engine-version compatibility.** Saves are the games' own data, the same in every engine version, so they are compared by content. The protocol has a version (`linkProto`), and a device with another version refuses with a message. Engine *configuration* is deliberately not synchronised.
6. **Other devices.** The Device Linking code is portable Go with no Windows dependencies; NEXUS itself is released for Windows only.
7. **NEXUS Cloud** for devices on different networks. Designed in [NEXUS_CLOUD.md](NEXUS_CLOUD.md), not built.
