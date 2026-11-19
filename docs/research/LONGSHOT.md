# Longshot — research only (19 Nov 2026), not integrated

**What it is.** A static recompilation of the Switch 2 *Ocarina of Time*: the game's ARM64 code is
translated once into a native x86-64 Windows or Linux program, with parts of the console OS
re-implemented. Not an emulator; it runs only this game, built from the user's own copy. It is a
GPL-3.0-or-later continuation of Hookshot after that project was taken down (team split; most
active developers moved to Longshot). Repository: git.magicbean.dev/MagicBeanDev/Longshot (the
site could not be fetched from here; sources are third-party pages that link to it).

**State.** Pre-release: trackers list v3.0.0-rc.7 (pre-release) for Longshot and v1.0.0-rc.9
(15 Nov 2026) for its launcher, which downloads the builder, checks each file's SHA-256, and builds
from the user's game (reported 30–45 minutes). Reported features: DLSS/DLAA, FSR 3.1/4,
resolutions to 5K, save import from Hookshot/Lichii.

**Steam Deck.** No published Steam Deck (or other AMD APU) frame-rate data was found. Linux x86-64
is a documented target, so a Deck build is plausible, but nothing shows how it runs there, how
long the build takes on a Deck, or whether audio and the AMD graphics path are correct there.

**Decision.** Not integrated, not recommended, and NEXUS shows no Longshot frame rate. Reasons: no
Steam Deck evidence; pre-release with a recent fork and takedown history; it replaces the game
rather than running it in NEXUS, so NEXUS's saves, profiles and settings would not apply.
Revisit when Deck measurements exist (frame rate *and* frame-time consistency, audio, a full
build on the Deck).
