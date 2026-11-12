# NEXUS Cloud — architecture proposal and costs (not built, nothing deployed)

> **Update (0.5.0 development):** the account part now exists as the NEXUS account service (`app/accountd`, see [ACCOUNTS.md](ACCOUNTS.md)): accounts with passwords, device tokens, linking codes with approval, and synchronisation of settings, users and amiibo. It is not deployed. Save synchronisation with version history (below) is still a proposal.

NEXUS Cloud would let linked devices synchronise when they are **not** on the same network.
Nothing here is implemented or running, and NEXUS never claims cloud synchronisation works.
Local Sync ([DEVICE_LINKING.md](DEVICE_LINKING.md)) needs no server at all.

## Design

- **No passwords, no shared secrets.**
  - Devices sign in to NEXUS Cloud with the same Ed25519 device keys they already use for Local Sync, by challenge and response.
  - A new device is added only through the existing pairing flow (code + approval on a trusted device). The trusted device then registers it with the cloud.
  - Removing a device in NEXUS revokes it in the cloud.
- **End-to-end encryption.**
  - Every user gets an account key that only the user's devices hold. It is handed over inside the SPAKE2 pairing channel.
  - Saves, profile and play time are encrypted on the device before upload, so the server stores only encrypted blobs and metadata (sizes, timestamps, hashes of the encrypted data). A breach of the server does not expose saves.
- **Same safety rules as Local Sync.** Three-way comparison, conflicts kept, backups before replacing, atomic writes on the device.
  - The cloud keeps **version history**: the last N versions of each save, plus 30 days.
- **Automatic mode.** Prefer Local Sync when the other device answers on the local network; otherwise use the cloud.
- **Never uploaded:** games, updates/DLC, keys, firmware.

### Components

| Part | Choice | Why |
|---|---|---|
| API | A small Go service (reuses NEXUS's own crypto and sync code) | One language; small and fast |
| Database | PostgreSQL (SQLite is possible for a first private deployment) | Accounts, devices, file versions, conflicts |
| Save storage | S3-compatible object storage (Cloudflare R2 recommended) | Cheap, no egress fees, versioning |
| TLS | Caddy (automatic Let's Encrypt certificates) | No certificate handling by hand |
| Deployment | Docker Compose on one small VPS | Simple to run, move and back up |
| Backups | Nightly database dump to the object store, plus a copy to a second provider; object versioning | Restore without losing saves |

Secrets (database password, storage keys) live only on the server, in environment variables. The
NEXUS app contains no secrets; it only knows the server's address and checks its certificate.

## Costs (approximate list prices, November 2026; check before buying)

Saves are small, typically 1–50 MB per game. A user with 20 games and version history is roughly
0.2–1 GB.

| Option | Monthly cost for a private release (≤ 50 users, ≤ 25 GB) | Notes |
|---|---|---|
| **A. VPS + Cloudflare R2** (recommended) | Hetzner CX-class VPS about €4–6 + R2: first 10 GB free, then $0.015/GB, no egress fees; about €5–6 in total | You maintain the server (updates, monitoring). Scales to thousands of users cheaply. |
| B. Supabase Pro (managed Postgres + storage + auth) | $25 base + usage | Less to run yourself; more expensive; auth model differs from device keys. |
| C. Self-hosted on your own PC/NAS | €0 | Needs the machine always on and reachable from the internet (port forwarding or a tunnel). Not recommended for other people's saves. |

Also needed: a domain (about €10–15 per year).

## What you would need to provide

1. A decision on option A, B or C, and who operates it (you).
2. A domain name, such as `cloud.<your domain>`.
3. A VPS account (option A) and a Cloudflare account with an R2 bucket and an API token. The token stays on the server.
4. Rules for how long version history and inactive accounts are kept, and a privacy notice (saves are personal data in some jurisdictions).

Once those exist, the order of work is:
1. Server API.
2. Device registration through pairing.
3. Encrypted upload and download with version history.
4. Automatic mode in NEXUS.
5. A staged rollout behind the same experimental flag.
