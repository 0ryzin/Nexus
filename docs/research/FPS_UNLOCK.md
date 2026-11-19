# Per-game frame rate ("FPS unlock") — 0.8.1

Not frame generation. What the NEXUS engine can actually change, from its source:

| Game | Mechanism (engine) | Offered | Status |
|---|---|---|---|
| Ocarina of Time (Switch 2, 0400486024DEC000, build E0F3CCFA…) | Built-in patch "FPS Unlock" (Dark's dynamic step + a >60 tail, `patch_manager.cpp`) keeps real speed; the VSync event rate `nx2_frame_rate_limit` (30–240) sets the rate | Default, 30, 60 / 90, 120, 240 | 30/60 supported; above 60 experimental (the patch re-times sound above 60; earlier issues #716/#1092 were long-session sound drop-outs) |
| Mario Kart World | The engine patches the game's own mode load (`loader/nso.cpp`): mkw_30fps / mkw_60fps (+fallback) / mkw_dynamic_fps | Default, 30, 60, dynamic | supported |
| Hyrule Warriors: Age of Imprisonment | Present-path frame interval (`nx2_frame_interval.h`): hyrule_30fps / hyrule_dynamic_fps | Default, 30, dynamic | supported |
| Other Switch 2 games | They start one fixed step per VSync event: a higher rate runs faster than real time, a lower one slower | Default only | unavailable |
| Switch 1 games | Run at their own rate; the engine's `fps_limit` only caps presents. Raising needs a game-specific mod for the exact build | Default; with a frame-rate mod on, NEXUS says the mod decides | unavailable / experimental (mod) |

- Stored per game (`fps.target`), applied at launch after the global limits; "Game default"
  writes nothing, so the original behaviour returns. The global Switch 2 limit is unchanged.
- NEXUS never downloads or installs patches. The Ocarina built-in is compiled into the engine
  (no files change); when it is off, NEXUS offers a button to switch it on, nothing more.
- Not measured: frame pacing and input latency at each rate on a Steam Deck or a PC. The Steam
  Deck's historic Ocarina rate was 10–20 fps in places, so above 30 is a ceiling there, not a
  promise. Test: `TestFPSUnlock`.
