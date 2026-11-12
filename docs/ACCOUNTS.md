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

**Never synchronised:** games, updates and DLC, keys, firmware, imported amiibo dumps (only their
details: on another device they show *Imported on Tom's PC*), shader caches, screenshots and
recordings. **Game saves** also stay on each device for now (see *Not done yet*).

Each category can be switched off under *What synchronises*; *Synchronise this device* switches all
of it off for this device.

### Offline and conflicts

- NEXUS never needs a connection. Changes are made locally first.
- Changes made while offline are kept and counted as *Changes pending*; the status shows *Waiting
  for connection*. NEXUS retries (30 s, then longer, up to 10 minutes) and sends them when the
  service can be reached. A change is sent about 10 seconds after you make it.
- If the same item was changed on two devices since they last synchronised, nothing is
  overwritten: the item appears under *Needs your choice*, showing what differs, and you choose
  *Keep this device's* or *Use Steam Deck's*.
- Removing a user is never synchronised (their saves are on that device).

### Devices, sign-out, recovery

- *Linked devices* lists every device with its platform and when it was last seen. Remove one to
  sign it out at once; nothing on that device is deleted.
- *Sign out on this device* keeps all local data.
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
  pending, every 15 minutes otherwise; with back-off while offline. Idle cost when nothing changes:
  no measurable CPU (0 ticks in 60 s in the measurement below).

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
