# NEXUS Account and synchronisation

> **Since 0.6.0 the NEXUS Account is your GitHub account** (`app/account_github.go`): *Sign in with GitHub* shows a code to enter at github.com/login/device; data is synchronised through a private Gist in your own GitHub account (files `nexus-*.json`, one per kind of data, plus `nexus-devices.json`). No NEXUS server is needed and nothing costs money. The NEXUS OAuth app's client ID is built in (public; device flow uses no client secret). Removing a device marks it removed in the device list and it signs itself out at its next synchronisation; *Revoke NEXUS on GitHub* (github.com/settings/applications) ends every device's access. A Gist has no atomic compare-and-set: NEXUS re-reads before writing and keeps per-item versions, so conflicting edits are still detected, but two devices writing within the same second could overwrite one change. The self-hosted service described below remains available under *Advanced*. Play time now synchronises too (each device's own time per user, added up on every device).

**Status (NEXUS 0.5.0, in development):** the account service (`app/accountd`) and the account
and synchronisation code in NEXUS are implemented and tested with automated tests that run the real
service and two NEXUS instances. **The service has not been deployed anywhere.** Until it is,
NEXUS shows *NEXUS Account › Account service: Not set up yet* and the account buttons are disabled.
Everything else in NEXUS works without an account.

## For users

### Where to find it

- **Settings › NEXUS Account** (the first entry in System Settings).
- The **cloud icon in the Home screen's top bar** shows *Not signed in*, *Synced …*, *Changes
  pending*, *Waiting for connection* or *Conflict needs attention*. Click it to open the page.
- **NEXUS Account** in the Home dock (cloud icon).
- **Settings › Users** and each user's settings have a *NEXUS Account* row.
- Once, on Home, a short note explains what an account is for (*Set up* / *Not now*). It does not
  come back after *Not now*.

### Create an account on your PC

1. Settings › NEXUS Account › **Create a NEXUS account**.
2. Enter your name, e-mail address, a password (at least 10 characters) and a name for this PC.
3. NEXUS shows a **recovery code** once. Write it down: it is the only way to set a new password.
4. The PC is signed in and synchronises at once.

### Link your Steam Deck (or another PC)

Either sign in there with your e-mail and password (*Sign in*), or use a code:

1. On the PC: NEXUS Account › **Link another device…**. NEXUS shows a code such as `K7QM-3XPA`
   (valid 10 minutes, usable once).
2. On the Steam Deck: NEXUS Account › **Link this device with a code**, type the code, and name the
   device ("Steam Deck").
3. On the PC: a prompt shows "Steam Deck (Linux) wants to join your NEXUS account". Choose
   **Approve**.
4. The Steam Deck is signed in and receives your users, settings and amiibo.

A user with the same name on both devices (for example "Tom") is treated as the same person, not
duplicated. If their details differ, NEXUS asks which version to keep.

### What synchronises

| Category | What | Default |
|---|---|---|
| Users | Name, picture, colour and theme (not the wallpaper file), controller layout | On |
| Game library | Names you gave games, hidden games, artwork choices, each game's controller layout | On |
| NEXUS preferences | Sounds, start-up animation, shortcut keys, Home behaviour, game window | On |
| Amiibo library | Names, details and owners of all entries; the data of NEXUS virtual amiibo | On |
| Emulation settings | Global and per-game emulation settings (never the graphics-card choice) | **Off**: a Steam Deck and a gaming PC usually need different settings |
| Game saves (0.8.0, GitHub accounts) | Each user's game saves: every save folder (all slots) and the game's device save | On |

**Never synchronised:** games, updates and DLC, keys, firmware, imported amiibo dumps (only their
details: on another device they show *Imported on Tom's PC*), shader caches, screenshots and
recordings.

Each category can be switched off under *What synchronises*; *Synchronise this device* switches all
of it off for this device.

- **Not synchronised: shader caches.** They contain the game's shader code and can be hundreds of
  MB; use a game's *Export Shader Data* / *Import Shader Data* between your own devices instead
  (docs/SHADERS.md).

### Game saves (0.8.0)

How it works (`app/savesync.go`):
- Each save (one game, one user) is a ZIP in a secret Gist of its own in your GitHub account
  ("NEXUS save: <game> (<user>)"). GitHub keeps every earlier version in the Gist's history.
- An index in the NEXUS sync Gist (`nexussaves-index.json`) lists each save with its version, a
  content hash (every file's path and SHA-256), its size, the device that sent it, and the raw
  address of exactly that version.
- A save is sent only when its contents changed since this device last agreed with the account,
  and received only when the account's changed and this device's did not.
- Before a received save replaces this device's copy, it is unpacked next to it and checked
  against the hash. This device's copy is backed up first (Save Data › Backups, "before-sync").
- If both changed, nothing is overwritten on either side. *NEXUS Account › Game saves* shows it with
  both devices' save times, and you choose (*Keep this device's* / *Use <device>'s*).
- Saves of a user whose game is running wait until it closes.
- Nothing is ever deleted by synchronisation, and saves over 20 MB (ZIP) are not synchronised.
- It runs with the account's synchronisation: when a game closes, at start, on wake, and with
  *Synchronise now*. Offline changes wait for the next one.

Tested with automated tests only: `TestSaveSyncWindowsToDeck` plays a PC and a Steam Deck against
a simulated GitHub. It covers:
- first sign-in on an empty device;
- both directions;
- several slots, users and device saves;
- no resend of unchanged saves;
- a conflict and its resolution;
- offline, then reconnecting.

It has not been tested between real devices yet.

### First start on a new device (0.8.0)

The first screen offers:
- *Sign in with my NEXUS account*: the account's users and their saves come to this device, so no
  throwaway local user is needed.
- *Create a NEXUS account*: a free GitHub account, then sign in.
- *Continue with a local user*.

A local user you created only to get in can be deleted later (Users › the user › Delete). Deleting
a user is never synchronised and does not touch the account's users or saves.

### Offline and conflicts

- NEXUS never needs a connection. Changes are made locally first.
- Changes made while offline are kept and counted as *Changes pending*; the status shows *Waiting
  for connection*. NEXUS retries (30 s, then longer, up to 10 minutes) and sends them when the
  service can be reached. A change is sent about 10 seconds after you make it.
- If the same item was changed on two devices since they last synchronised, nothing is
  overwritten: the item appears under *Needs your choice*, showing what differs, and you choose
  *Keep this device's* or *Use Steam Deck's*.
- Removing a user is never synchronised (their saves are on that device).
- **Staying in sync after time away (0.8.0):** NEXUS synchronises when the computer wakes from
  sleep, when NEXUS comes back to the front after more than a minute, and while a game is kept
  running behind Home (before 0.8.0 a game left in the background stopped synchronisation for as
  long as it ran, and sleep could delay it by hours). Only a game in front waits.
- **GitHub sign-ins do not expire on their own.** NEXUS uses a GitHub OAuth app; its sign-in has
  no expiry and no refresh token (GitHub only ends it when you revoke it, or after a year unused).
  A single "not accepted" answer from GitHub is checked against your GitHub profile first; the
  device is signed out only if GitHub refuses the sign-in there too.

### Devices, sign-out, recovery

- *Linked devices* lists every device with its platform and when it was last seen. Remove one to
  sign it out at once; nothing on that device is deleted.
- *Sign out on this device* keeps all local data.
- **Switching accounts:** sign out, then sign in with another account. If it is a different
  account than this device used before, NEXUS receives that account's data but sends nothing
  from this device until you choose *Send this device's data* or *Keep this device's data here*
  (status *Waiting for your choice*). Data that came from the first account is never copied into
  the second by accident.
- *Change password* signs out your other devices.
- *Forgot your password?* uses the recovery code; all devices are signed out and you receive a
  new recovery code.

## How it works

### The account service (`app/accountd`, `app/cmd/nexus-accountd`)

A small Go HTTP JSON API (no third-party dependencies), protocol version 1:

| Endpoint | Purpose |
|---|---|
| `POST /v1/accounts` | Create an account; signs the device in; returns the recovery code once |
| `POST /v1/sessions`, `DELETE /v1/sessions/current` | Sign in, sign out |
| `POST /v1/recover` | New password with the recovery code; signs out every device |
| `GET/PATCH/DELETE /v1/account` | Account and devices; name, device name, password; delete |
| `DELETE /v1/devices/{id}` | Remove (revoke) a device |
| `POST /v1/pair/codes`, `GET /v1/pair/requests`, `POST /v1/pair/requests/{id}` | Linking codes and approval (signed-in device) |
| `POST /v1/pair/claim`, `POST /v1/pair/claim/{id}` | Enter a code; wait for approval (new device) |
| `GET /v1/sync?since=N`, `PUT /v1/sync/{kind}/{name}` | Changes since a revision; upload based on a version |

**Security.**
- Passwords and recovery codes: PBKDF2-HMAC-SHA256, 600,000 iterations, random salt. Unknown
  e-mail addresses take as long as wrong passwords. Five wrong passwords pause sign-in for 15 minutes.
- Device tokens: 256 random bits, one per device. The service stores only their SHA-256, so a
  copy of its database cannot sign anyone in. They expire after 180 days without use and are
  revoked on sign-out, device removal, password change (other devices) and recovery (all devices).
- Linking codes: 8 characters from a 31-character alphabet (about 40 bits), 10 minutes, single
  use, at most three at a time. The code only creates a *request*: the signed-in device must
  approve it, and only then does the new device receive its own token, exactly once. Neither the
  code nor anything else shown carries a permanent token.
- Rate limits per address on registration, sign-in, recovery and code entry (20 per minute;
  `-trust-proxy` takes the address from the proxy's `X-Forwarded-For`).
- Synchronised items: only the NEXUS kinds above (the service refuses anything else, such as
  saves, keys or game files), at most 1 MB each, 64 MB and 5,000 items per account.
- Nothing secret is logged. The client contains no secrets: only the service address.
- Data is protected in transit by TLS. It is not end-to-end encrypted: the service can read
  synchronised settings and names (it is not given saves, keys or game files).

**Storage.** JSON files in the data folder (`accounts.json`, `items/<account>.json`), written
atomically. That suits a private deployment of up to a few thousand accounts; a larger deployment
would move it to PostgreSQL behind the same API.

### In NEXUS (`app/account.go`)

- State in `<NEXUS data>/System/Account/account.json`; the sign-in token separately in
  `session.bin`:
  - **Windows:** encrypted with DPAPI for the Windows user.
  - **Linux / Steam Deck / macOS:** a file only that user can read (0600 in a 0700 folder). On
    Linux NEXUS data is in `$XDG_DATA_HOME/NEXUS` (default `~/.local/share/NEXUS`). No
    Windows-only feature is used.
- The address must be `https://` (or `http://` on this computer, for testing). Release builds set
  `defaultAccountServer`; advanced users can enter their own service.
- Synchronisation is three-way per item: the version both sides last agreed on (version and hash)
  is stored. One-sided changes are applied or uploaded with the version they are based on; the
  service refuses an upload based on an older version (409), which becomes a conflict for the user.
  Never "last upload wins".
- Background: at start, about 10 seconds after a change in NEXUS, every 2 minutes if changes are
  pending, every 15 minutes otherwise, after waking from sleep and when NEXUS comes to the front
  (at most every 30 s); with back-off while offline (30 s doubling to 10 minutes). Times are
  wall-clock: Windows' monotonic clock stops during sleep.
- A stored sign-in that cannot be read for a moment (another program has the file open, a passing
  DPAPI error) is retried at the next five synchronisations before NEXUS asks to sign in again. Idle cost when nothing changes:
  no measurable CPU (0 ticks in 60 s in the measurement below).

### GitHub sign-in that lasts (`app/account_github_cred.go`, 0.8.0)

- GitHub gives OAuth apps registered since August 2026 (NEXUS's included) **eight-hour access
  tokens** and a **refresh token** valid for six months from its last use. NEXUS asks for
  `gist offline_access` so it always gets the renewable kind.
- `session.bin` holds the whole sign-in as JSON (`v`, `access`, `refresh`, `expires`,
  `refreshExpires`), encrypted with DPAPI on Windows. A `session.bin` from 0.7 or earlier (the
  access token alone) is still read; it cannot be renewed, so it lasts as long as GitHub accepts
  it. No token is ever written to `account.json` or to the log.
- Before each API request the access token is renewed if it expires within 10 minutes; after a
  401 it is renewed and the request is tried once more. GitHub rotates the refresh token at each
  renewal, so the new pair is written at once (and kept in memory and written again later if
  that fails). One renewal at a time.
- The sign-in ends only when the user disconnects, or when GitHub refuses the refresh token
  itself (`bad_refresh_token`, `invalid_grant`: revoked at github.com, or six months unused). A
  network failure, a 5xx answer or an unknown error keeps it and is retried with the offline
  back-off. A token without a refresh token is ended only when GitHub also refuses `GET /user`.
- Status `connection`: `connected`, `notconnected`, `reconnecting` (renewing), `expired` (GitHub
  refused it; the identity stays on screen with *Reconnect*), `problem` (cannot reach GitHub;
  still connected), `syncing`. `POST /api/account/test` checks the connection.
- Reconnecting the same GitHub account keeps this device's ID and its entry in the device list.
- The NEXUS user on screen ("Who's playing?") is separate: switching or adding users never
  touches the GitHub connection.

### Tests

- `app/accountd/accountd_test.go`:
  - registration and validation; no secrets stored in clear;
  - sign-in, lock-out, generic errors, sign-out, 180-day expiry, restart;
  - recovery and password change;
  - linking: wrong, single-use and expired codes, approval and decline, the token handed over
    once, device removal;
  - synchronisation with 409 conflicts; refused kinds and sizes; rate limits.
- `app/account_test.go`: the real service with two NEXUS instances ("Tom's PC" and "Steam Deck"):
  - no service: no pretend account;
  - the token is not in the readable file and has 0600 permissions on Linux;
  - linking by code, with approval;
  - same-name users matched and their conflict resolved;
  - library names and preferences arrive; emulation settings only when switched on;
  - the virtual amiibo's data arrives; the imported dump's file does not, and cannot be placed in
    a game there;
  - no duplicates; offline changes kept and sent later;
  - a two-device conflict kept and resolved; deletion of an amiibo synchronised;
  - a removed device signed out with its data kept; sign-in persists across a restart.

## Deploying the service

You need: a small Linux server (any VPS with Docker), a domain name (for example
`accounts.<your domain>`) pointing at it, and ports 80 and 443 open.

```
cd app
# edit accountd/deploy/Caddyfile: replace accounts.example.org with your domain
docker compose -f accountd/deploy/docker-compose.yml up -d
curl https://accounts.<your domain>/v1/health    # {"protocol":1,"service":"nexus-accountd"}
```

Caddy obtains the certificate. Back up the `accountd-data` volume. Then set
`defaultAccountServer` in `app/account.go` to the address for the release build (or enter it under
*NEXUS Account › Advanced › Account service address* to test).

## Not done yet

- **Deployment.** No public service is running. Someone has to own the server, the domain and the
  backups, and publish a privacy notice (e-mail addresses are personal data).
- **E-mail.** No e-mail verification or password reset by e-mail (no mail service); the recovery
  code replaces it.
- **Saves.** Synchronising saves safely needs per-file version history and object storage on the
  service (see NEXUS_CLOUD.md). Not built. The experimental Local Sync (DEVICE_LINKING.md) can
  synchronise saves between devices on the same network.
- **QR codes.** Linking uses a typed code; a QR code (of the same short-lived code) is not built.
- **Steam Deck.** There is no NEXUS for Linux build yet. The account code is portable and its tests
  run on Linux, but the account flow has not been tried on a Steam Deck.
