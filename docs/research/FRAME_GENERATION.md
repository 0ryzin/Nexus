# Frame generation — Lichii, Eglantine, others (research for 0.9.2, 24 Nov 2026)

Sources read: the Lichii source (github.com/zortos293/Lichii, branch nx2, up to 6595a54 of
24 Nov), the Eglantine repository (github.com/Eglantine-Community/Eglantine, HEAD 835019f of
22 Nov, tag v0.1.0-rc4), and the NEXUS engine (Lichii build 316 base + engine/patches).

## What exists — the same code family in all three
Lichii, Eglantine and the NEXUS engine all descend from Eden and share one frame generation
system in `src/video_core/renderer_vulkan/present/` (setting `Renderer/frame_gen`, type
`frame_gen_type`, multiplier 2–4, target rate, flow scale, a pacer):

| Method | What it is | GPU / OS | In NEXUS |
|---|---|---|---|
| **Lichii Optical Flow** (was labelled "FSR3"; Lichii renamed it in #1633 because it is not AMD FSR 3 FG) | The engine's own compute shaders: block-matching optical flow on the presented frame (no game motion vectors), then warped interpolation. Since Lichii #2386 (22 Nov): luma pyramid, coarse full search, refinement, sub-pixel fit, validity checks; shows the nearer real frame where warps disagree. | Any Vulkan GPU with storage writes to the swapchain format — AMD, NVIDIA, Intel; Windows and Linux; **includes the Steam Deck** (RDNA2, RADV). | Yes (0.8.x had the older matcher; 0.9.2 has #2386 as engine patch 0047). |
| **LSFG** (Lossless Scaling Frame Generation) | Lossless Scaling's shaders, translated from the user's own `Lossless.dll` (a paid Windows app) to SPIR-V at run time (lsfg-vk approach). | Vulkan, Windows and Linux, AMD/NVIDIA; needs the user's copy of Lossless Scaling. | Yes ("your own copy"). NEXUS ships nothing from it. |
| **NVIDIA DLSS Frame Generation** | NVIDIA Streamline (DLSS-G), fed with Ocarina of Time's depth, motion vectors and camera. | NVIDIA RTX 40/50, Windows builds only, Ocarina only, needs DLSS on. | Not offered by NEXUS on Linux; on Windows only with Streamline. Not the Deck. |
| **FlyHigh** (Lichii #2406, 21 Nov) | Windows-only opt-in. | Windows. | Not taken: not usable on Linux/Deck. |
| NVIDIA Smooth Motion | Driver-level frame generation (NVIDIA Linux driver, RTX 40/50). | NVIDIA only. | Yes, as a session option. |

**Eglantine** has the same LSFG and optical-flow ("FSR3") code (added 30 Oct, last frame-gen
change 14 Nov) plus Streamline DLSS; it bundles AMD's FidelityFX SDK, but for **FSR 3 upscaling**
(`ffx_fsr3upscaler_*`), not AMD's frame interpolation. Nothing in Eglantine is newer than what
NEXUS now has; its frame generation is not a separate technology.

**AMD's own frame generation** (FSR 3 FG / AFMF) cannot be used: FSR 3 FG needs the game's motion
vectors and a DirectX 12/Vulkan integration in the renderer of a PC game; AFMF is a Windows driver
feature. The Steam Deck can also use gamescope/Decky-based lsfg-vk outside NEXUS — that is the
Steam-side route the user wants to avoid relying on; the same shaders run inside the engine.

## How it behaves (from the code)
- Generated frames are presented between the game's own frames: the game's simulation, input,
  audio and speed are unchanged (it never touches the emulated GPU or timing).
- Latency: it must hold the newest real frame until the generated one is shown, about one real
  frame of extra latency (at 15 fps base ~67 ms more).
- Artefacts: worst at low base rates (large motion between frames), around fast motion, HUD
  text and transparencies. At Ocarina's 8–20 fps on the Deck, interpolation guesses across
  50–125 ms gaps: smoother-looking motion, visible warping, more latency. It does not make the
  game render faster and can cost a little GPU time (the flow runs at reduced resolution; flow
  scale sets it).
- NEXUS shows what it really does while playing (engine status: running / unsupported surface /
  missing DLL / unsupported GPU) — never just the setting.

## Steam Deck verdict
Technically possible (Vulkan compute, RADV, no vendor extension). Not measured on a Deck: FPS,
frame times, latency, artefacts, power and thermals all need the user's Deck (performance strip).
Expected: perceived smoothness up, native frame rate unchanged or slightly down, latency up; at
base rates under ~30 fps generally not recommended. Off by default.
