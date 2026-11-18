# Achievements: feasibility (investigation only, 18 Nov 2026)

Nothing was implemented. This answers "could NEXUS have achievements?".

## RetroAchievements

RetroAchievements does not support the Switch or Switch 2. Its newest supported console is the Wii
(added March 2026). Its achievements are written by its community against fixed memory addresses
and run by `rcheevos`, a C library an emulator embeds: every frame the emulator lets rcheevos read
guest memory, and rcheevos checks the conditions. That works for consoles whose games keep their
state at stable addresses.

Switch games are different: they are large, position-independent, use a heap whose layout varies
between runs, and are patched by updates. RetroAchievements has not added the console, so even if
NEXUS embedded rcheevos there would be no achievement sets to load, and users could not sign in to
anything that tracks Switch games.

## Options for NEXUS

| Option | Effort | Notes |
|---|---|---|
| Wait for RetroAchievements Switch support, then embed rcheevos (MIT) in the engine | Medium, but not possible now | Needs memory-read hooks in the engine's guest memory; the set authors decide which games |
| NEXUS's own achievements from save data (e.g. "finished the game" read from a save file) | High per game | Needs a per-game save parser; must never write to saves; game-specific and fragile across updates |
| NEXUS's own achievements from engine hooks (pointer chains per game and version) | Very high per game | The same work RetroAchievements' authors do, without their tools or community |
| Play-time milestones (hours played, games started) from NEXUS's own statistics | Low | Not real game achievements; could be shown as "milestones" so it is not mistaken for them |

## Recommendation

Do not build per-game achievements now. If wanted, play-time milestones are the only honest option
that works for every game today. Revisit if RetroAchievements announces Switch support.
