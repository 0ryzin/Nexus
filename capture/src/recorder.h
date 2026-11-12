// SPDX-FileCopyrightText: 2026 NEXUS project
// SPDX-License-Identifier: GPL-3.0-or-later

#pragma once

#include <atomic>
#include <condition_variable>
#include <cstdint>
#include <functional>
#include <mutex>
#include <string>
#include <thread>
#include <vector>

#include <windows.h>
#include <d3d11.h>
#include <mfapi.h>
#include <mfidl.h>
#include <mfreadwrite.h>
#include <winrt/base.h>

namespace nexus::capture {

struct Settings {
    HWND hwnd{};
    std::wstring path; // final file; ".part" is written while recording
    int width{0}, height{0};
    int fps{60};
    int bitrate_kbps{12000};
    bool hevc{false};
    bool audio{true};
    bool mic{false};
    bool hardware{true};
    DWORD pid{0};
    // self-test: generated frames and tone instead of a window and devices
    bool synthetic{false};
    bool gpu_device{false}; // set by the recorder: Direct3D runs on a graphics card (not WARP)
    int synthetic_seconds{0};
};

struct Status {
    std::string state{"idle"};
    double seconds{0};
    uint64_t frames{0}, dropped{0};
    int width{0}, height{0};
    std::string encoder;
    bool gpu{false};
    std::string audio{"off"};
    bool mic{false};
    std::string path, error;
};

/// Converts BGRA textures of any size to NV12 frames of a fixed size (letterboxed) on the GPU.
class Converter {
public:
    bool Init(ID3D11Device* dev, int out_w, int out_h, int fps, std::string& err);
    /// Converts src into the next output texture of the ring and returns it.
    ID3D11Texture2D* Convert(ID3D11Texture2D* src, std::string& err);
    ID3D11Texture2D* Black() { return Convert(nullptr, scratch_err_); }

private:
    winrt::com_ptr<ID3D11Device> dev_;
    winrt::com_ptr<ID3D11DeviceContext> ctx_;
    winrt::com_ptr<ID3D11VideoDevice> vdev_;
    winrt::com_ptr<ID3D11VideoContext> vctx_;
    winrt::com_ptr<ID3D11VideoProcessorEnumerator> enum_;
    winrt::com_ptr<ID3D11VideoProcessor> proc_;
    std::vector<winrt::com_ptr<ID3D11Texture2D>> outs_;
    winrt::com_ptr<ID3D11Texture2D> black_;
    std::vector<winrt::com_ptr<ID3D11VideoProcessorOutputView>> out_views_;
    size_t next_{0};
    int out_w_{0}, out_h_{0}, fps_{60};
    UINT in_w_{0}, in_h_{0};
    std::string scratch_err_;
    bool Prepare(UINT w, UINT h, std::string& err);
};

/// Writes NV12 frames and PCM audio to a fragmented MP4 with Media Foundation.
class Writer {
public:
    bool Open(const Settings& s, ID3D11Device* dev, int w, int h, int mic_rate, int mic_channels, std::string& err);
    bool WriteVideo(ID3D11Texture2D* nv12, LONGLONG time, LONGLONG duration, std::string& err);
    bool WriteAudio(int track, const int16_t* pcm, size_t frames, int channels, int rate, LONGLONG time, std::string& err);
    bool Finish(std::string& err);
    std::string EncoderName() const { return encoder_; }
    bool Gpu() const { return gpu_; }
    bool HasMic() const { return mic_stream_ != MAXDWORD; }

private:
    bool OpenMode(const Settings& s, ID3D11Device* dev, int w, int h, bool gpu, int mic_rate, int mic_channels, std::string& err);
    winrt::com_ptr<IMFSinkWriter> writer_;
    winrt::com_ptr<IMFDXGIDeviceManager> manager_;
    winrt::com_ptr<ID3D11Device> dev_;
    winrt::com_ptr<ID3D11DeviceContext> ctx_;
    winrt::com_ptr<ID3D11Texture2D> staging_;
    DWORD video_stream_{MAXDWORD}, audio_stream_{MAXDWORD}, mic_stream_{MAXDWORD};
    int w_{0}, h_{0};
    bool gpu_{false};
    std::string encoder_;
    std::mutex mu_; // the sink writer is used from the video and audio threads
};

class Recorder {
public:
    ~Recorder();
    bool Start(const Settings& s, std::string& err);
    bool Stop(std::string& err);
    Status GetStatus();

private:
    void VideoThread();
    void AudioThread(bool microphone);
    void Fail(const std::string& why);
    LONGLONG Now100ns() const;

    Settings s_;
    winrt::com_ptr<ID3D11Device> dev_;
    winrt::com_ptr<ID3D11DeviceContext> ctx_;
    Converter conv_;
    Writer writer_;
    int out_w_{0}, out_h_{0};

    // latest window frame (written by the capture callback, read by the video thread)
    std::mutex frame_mu_;
    winrt::com_ptr<ID3D11Texture2D> latest_;
    bool have_frame_{false};
    struct Capture;
    Capture* cap_{nullptr};

    std::thread video_, audio_, mic_;
    std::atomic<bool> stop_{false};
    std::atomic<bool> failed_{false};
    std::atomic<uint64_t> frames_{0}, dropped_{0};
    std::atomic<LONGLONG> written_100ns_{0};
    LARGE_INTEGER qpf_{}, qpc0_{};
    std::mutex st_mu_;
    Status st_;
    std::string audio_mode_{"off"};
    bool mic_on_{false};
};

std::string CapabilitiesJson();
std::string SelfTest(const std::wstring& path, int seconds);
std::string Narrow(const std::wstring& w);
std::wstring Widen(const std::string& s);
std::string HrText(HRESULT hr);

} // namespace nexus::capture
