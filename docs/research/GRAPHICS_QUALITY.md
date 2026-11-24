# Lichii's graphics-quality setting = "ASTC Recompression Method" (research for 0.9.2)

**Exact setting:** `Renderer/astc_recompression` (Qt label "ASTC Recompression Method"; options
"Uncompressed (Best quality)", "BC3 (Medium quality)", "BC1 (Low quality)"). Same code in the
NEXUS engine (`common/settings.h`, `video_core/surface.cpp`, `renderer_vulkan/vk_texture_cache.cpp`,
`texture_cache/util.cpp`).

**What it does:** Switch games store most textures as ASTC. GPUs that cannot sample ASTC (all
desktop AMD/NVIDIA and the Steam Deck's RDNA2) must decode them, on the GPU by default
(`accelerate_astc`). The setting decides what the decoded texture is kept as:
- Uncompressed: RGBA8, 32 bits per pixel — exact decode, best quality, 4× Medium's memory.
- BC3: re-encoded on the GPU to 8 bpp (BC encoder pass), small quality loss.
- BC1: 4 bpp, 1-bit alpha: half of BC3, visible blockiness, banding on gradients, rough alpha edges.
It changes **texture memory and memory bandwidth only** — not render resolution, not CPU work,
not shaders. It helps where video memory or bandwidth is the limit; re-encoding costs a little
GPU time when textures load.

**Engine default and override:** the engine default is BC3. Switch 2 games on a GPU without ASTC
and with a memory budget below 20 GiB (every 8–16 GB card and the Steam Deck) are forced to BC3
for the session even if Uncompressed is chosen (`UseBc3ForSwitch2OnSmallCards`: uncompressed
textures did not fit and Mario Kart World broke). So on the Deck, Ocarina already runs at Medium;
"High" cannot apply there, and only Low can change anything for it.

**NEXUS 0.9.2:** *Graphics › Texture quality* (global and per game): High / Medium / Low, default
**High** as asked. Games already played on the device before 0.9.2 keep Medium as their own
per-game choice (what they ran with), so Final Fantasy Resonance's working configuration is not
changed behind the user's back; the user can raise it. No automatic lowering on any device.

**Not measured:** FPS and visual differences per mode on the Deck. Expectation from the code: no
change for Ocarina between High and Medium on the Deck (both BC3); Low may help only if texture
bandwidth/memory is a limit — test with the performance strip.
