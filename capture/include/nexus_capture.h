// SPDX-FileCopyrightText: 2026 NEXUS project
// SPDX-License-Identifier: GPL-3.0-or-later
//
// NEXUS capture module (Windows): records a game window to MP4.
//
//   video: Windows Graphics Capture of one window (only the game, never the NEXUS interface)
//          -> Direct3D 11 video processor (BGRA -> NV12, scaling, letterbox) on the GPU
//          -> Media Foundation H.264/HEVC encoder (hardware: NVENC, AMD AMF, Intel Quick Sync —
//             whatever the driver registers; software otherwise)
//   audio: the NEXUS process's own sound (process loopback, Windows 10 2004+; falls back to the
//          whole PC's sound on older Windows), optional microphone as a second audio track
//   file:  fragmented MP4, so a recording cut off by a crash still plays up to that point
//
// All functions are thread-safe. Strings are UTF-8. Settings are "key=value" lines.

#pragma once
#include <stdint.h>

#ifdef NEXUS_CAPTURE_BUILD
#define NXCAP_API __declspec(dllexport)
#else
#define NXCAP_API __declspec(dllimport)
#endif

#ifdef __cplusplus
extern "C" {
#endif

#define NXCAP_API_VERSION 1

NXCAP_API int32_t nxcap_api_version(void);

/// Video encoders on this PC, as JSON:
/// {"encoders":[{"name","vendor":"nvidia|amd|intel|microsoft|other","hardware":true,"codec":"h264|hevc"}],
///  "windowCapture":true,"processAudio":true}
/// Returns the length needed (write again with a larger buffer if it was too small).
NXCAP_API int32_t nxcap_capabilities(char* buf, int32_t buf_len);

/// Starts recording. Settings (one per line):
///   hwnd=<window handle, decimal>   path=<output file; written as path + ".part" until stopped>
///   width=<0 = window size> height=<…> fps=<24..120> bitrate=<kbit/s> codec=h264|hevc
///   audio=1|0  mic=1|0  hardware=1|0 (allow hardware encoders)  pid=<process for game audio>
/// Returns 0, or -1 with a reason in err.
NXCAP_API int32_t nxcap_start(const char* settings, char* err, int32_t err_len);

/// Status as JSON: {"state":"idle|starting|recording|stopping|error","seconds","frames","dropped",
///  "width","height","encoder","gpu":true,"audio":"process|system|off","mic":true,"path","error"}
NXCAP_API int32_t nxcap_status(char* buf, int32_t buf_len);

/// Stops and finalises the recording (renames path.part to path). Returns 0 or -1 with a reason.
NXCAP_API int32_t nxcap_stop(char* err, int32_t err_len);

/// Self-test without a window: encodes `seconds` of generated video and audio through the same
/// colour conversion and encoder path into `path`, reads the file back with Media Foundation and
/// reports what it found as JSON ({"ok","encoder","gpu","videoSamples","audioSamples","duration","error"}).
NXCAP_API int32_t nxcap_selftest(const char* path, int32_t seconds, char* buf, int32_t buf_len);

#ifdef __cplusplus
}
#endif
