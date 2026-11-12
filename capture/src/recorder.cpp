// SPDX-FileCopyrightText: 2026 NEXUS project
// SPDX-License-Identifier: GPL-3.0-or-later
//
// NEXUS capture module: window capture, colour conversion, encoding and audio capture.
// Written for NEXUS from Microsoft's public documentation of Windows Graphics Capture, Direct3D 11
// video processing, Media Foundation and WASAPI.

#include "recorder.h"

#include <audioclient.h>
#include <audioclientactivationparams.h>
#include <codecapi.h>
#include <d3d10.h>
#include <dxgi1_2.h>
#include <mferror.h>
#include <mmdeviceapi.h>
#include <dwmapi.h>

#include <windows.graphics.capture.h>
#include <windows.graphics.capture.interop.h>
#include <windows.graphics.directx.direct3d11.interop.h>
#include <winrt/Windows.Foundation.h>
#include <winrt/Windows.Foundation.Metadata.h>
#include <winrt/Windows.Graphics.Capture.h>
#include <winrt/Windows.Graphics.DirectX.Direct3D11.h>

#include <algorithm>
#include <chrono>
#include <cmath>
#include <cstring>
#include <filesystem>
#include <sstream>

namespace wgc = winrt::Windows::Graphics::Capture;
namespace wdx = winrt::Windows::Graphics::DirectX;

namespace nexus::capture {

std::string Narrow(const std::wstring& w) {
    if (w.empty()) return {};
    int n = WideCharToMultiByte(CP_UTF8, 0, w.data(), (int)w.size(), nullptr, 0, nullptr, nullptr);
    std::string s(n, '\0');
    WideCharToMultiByte(CP_UTF8, 0, w.data(), (int)w.size(), s.data(), n, nullptr, nullptr);
    return s;
}

std::wstring Widen(const std::string& s) {
    if (s.empty()) return {};
    int n = MultiByteToWideChar(CP_UTF8, 0, s.data(), (int)s.size(), nullptr, 0);
    std::wstring w(n, L'\0');
    MultiByteToWideChar(CP_UTF8, 0, s.data(), (int)s.size(), w.data(), n);
    return w;
}

std::string HrText(HRESULT hr) {
    char buf[32];
    snprintf(buf, sizeof(buf), "0x%08lX", (unsigned long)hr);
    wchar_t* msg = nullptr;
    FormatMessageW(FORMAT_MESSAGE_ALLOCATE_BUFFER | FORMAT_MESSAGE_FROM_SYSTEM | FORMAT_MESSAGE_IGNORE_INSERTS, nullptr, hr, 0,
                   reinterpret_cast<wchar_t*>(&msg), 0, nullptr);
    std::string out = buf;
    if (msg) {
        std::wstring m = msg;
        LocalFree(msg);
        while (!m.empty() && (m.back() == L'\n' || m.back() == L'\r' || m.back() == L' ' || m.back() == L'.')) m.pop_back();
        out = Narrow(m) + " (" + buf + ")";
    }
    return out;
}

#define NX_CHECK(expr, what)                                       \
    do {                                                           \
        HRESULT hr_ = (expr);                                      \
        if (FAILED(hr_)) {                                         \
            err = std::string(what) + ": " + HrText(hr_);          \
            return false;                                          \
        }                                                          \
    } while (0)

static winrt::com_ptr<ID3D11Device> MakeDevice(bool allow_hardware, std::string& err, bool* is_hw) {
    winrt::com_ptr<ID3D11Device> dev;
    const UINT flags = D3D11_CREATE_DEVICE_BGRA_SUPPORT | D3D11_CREATE_DEVICE_VIDEO_SUPPORT;
    const D3D_FEATURE_LEVEL levels[] = {D3D_FEATURE_LEVEL_11_1, D3D_FEATURE_LEVEL_11_0, D3D_FEATURE_LEVEL_10_1};
    HRESULT hr = E_FAIL;
    if (allow_hardware) {
        hr = D3D11CreateDevice(nullptr, D3D_DRIVER_TYPE_HARDWARE, nullptr, flags, levels, 3, D3D11_SDK_VERSION, dev.put(), nullptr, nullptr);
        if (is_hw) *is_hw = SUCCEEDED(hr);
    }
    if (FAILED(hr)) {
        dev = nullptr;
        hr = D3D11CreateDevice(nullptr, D3D_DRIVER_TYPE_WARP, nullptr, flags, levels, 3, D3D11_SDK_VERSION, dev.put(), nullptr, nullptr);
        if (is_hw) *is_hw = false;
    }
    if (FAILED(hr)) {
        err = "Direct3D 11 is not available: " + HrText(hr);
        return nullptr;
    }
    if (auto mt = dev.try_as<ID3D10Multithread>()) mt->SetMultithreadProtected(TRUE);
    return dev;
}

// ---------------------------------------------------------------- converter

bool Converter::Init(ID3D11Device* dev, int out_w, int out_h, int fps, std::string& err) {
    dev_.copy_from(dev);
    dev_->GetImmediateContext(ctx_.put());
    vdev_ = dev_.try_as<ID3D11VideoDevice>();
    vctx_ = ctx_.try_as<ID3D11VideoContext>();
    if (!vdev_ || !vctx_) {
        err = "This graphics driver does not offer Direct3D 11 video processing";
        return false;
    }
    out_w_ = out_w;
    out_h_ = out_h;
    fps_ = fps;
    {
        D3D11_TEXTURE2D_DESC bd{};
        bd.Width = 64;
        bd.Height = 64;
        bd.MipLevels = 1;
        bd.ArraySize = 1;
        bd.Format = DXGI_FORMAT_B8G8R8A8_UNORM;
        bd.SampleDesc.Count = 1;
        bd.Usage = D3D11_USAGE_DEFAULT;
        bd.BindFlags = D3D11_BIND_SHADER_RESOURCE | D3D11_BIND_RENDER_TARGET;
        std::vector<uint32_t> px(64 * 64, 0xFF000000u);
        D3D11_SUBRESOURCE_DATA sd{px.data(), 64 * 4, 0};
        NX_CHECK(dev_->CreateTexture2D(&bd, &sd, black_.put()), "black frame");
    }
    for (int i = 0; i < 6; ++i) {
        D3D11_TEXTURE2D_DESC td{};
        td.Width = out_w;
        td.Height = out_h;
        td.MipLevels = 1;
        td.ArraySize = 1;
        td.Format = DXGI_FORMAT_NV12;
        td.SampleDesc.Count = 1;
        td.Usage = D3D11_USAGE_DEFAULT;
        td.BindFlags = D3D11_BIND_RENDER_TARGET | D3D11_BIND_SHADER_RESOURCE;
        winrt::com_ptr<ID3D11Texture2D> t;
        NX_CHECK(dev_->CreateTexture2D(&td, nullptr, t.put()), "NV12 frame texture");
        outs_.push_back(t);
    }
    return Prepare(out_w, out_h, err);
}

bool Converter::Prepare(UINT w, UINT h, std::string& err) {
    if (proc_ && w == in_w_ && h == in_h_) return true;
    proc_ = nullptr;
    enum_ = nullptr;
    out_views_.clear();
    D3D11_VIDEO_PROCESSOR_CONTENT_DESC cd{};
    cd.InputFrameFormat = D3D11_VIDEO_FRAME_FORMAT_PROGRESSIVE;
    cd.InputFrameRate = {(UINT)fps_, 1};
    cd.InputWidth = w;
    cd.InputHeight = h;
    cd.OutputFrameRate = {(UINT)fps_, 1};
    cd.OutputWidth = out_w_;
    cd.OutputHeight = out_h_;
    cd.Usage = D3D11_VIDEO_USAGE_PLAYBACK_NORMAL;
    NX_CHECK(vdev_->CreateVideoProcessorEnumerator(&cd, enum_.put()), "video processor");
    UINT fmt_flags = 0;
    if (FAILED(enum_->CheckVideoProcessorFormat(DXGI_FORMAT_NV12, &fmt_flags)) || !(fmt_flags & D3D11_VIDEO_PROCESSOR_FORMAT_SUPPORT_OUTPUT)) {
        err = "The graphics driver cannot convert video frames to NV12";
        return false;
    }
    NX_CHECK(vdev_->CreateVideoProcessor(enum_.get(), 0, proc_.put()), "video processor");
    for (auto& t : outs_) {
        D3D11_VIDEO_PROCESSOR_OUTPUT_VIEW_DESC ovd{};
        ovd.ViewDimension = D3D11_VPOV_DIMENSION_TEXTURE2D;
        winrt::com_ptr<ID3D11VideoProcessorOutputView> v;
        NX_CHECK(vdev_->CreateVideoProcessorOutputView(t.get(), enum_.get(), &ovd, v.put()), "video output view");
        out_views_.push_back(v);
    }
    // letterbox: the source keeps its shape inside the output frame, black around it
    D3D11_VIDEO_COLOR black{};
    black.YCbCr = {0.0625f, 0.5f, 0.5f, 1.0f};
    vctx_->VideoProcessorSetOutputBackgroundColor(proc_.get(), TRUE, &black);
    D3D11_VIDEO_PROCESSOR_COLOR_SPACE in_cs{}, out_cs{};
    in_cs.RGB_Range = 0;  // full-range RGB
    out_cs.RGB_Range = 0;
    out_cs.YCbCr_Matrix = 1; // BT.709
    out_cs.Nominal_Range = D3D11_VIDEO_PROCESSOR_NOMINAL_RANGE_16_235;
    vctx_->VideoProcessorSetStreamColorSpace(proc_.get(), 0, &in_cs);
    vctx_->VideoProcessorSetOutputColorSpace(proc_.get(), &out_cs);
    vctx_->VideoProcessorSetStreamFrameFormat(proc_.get(), 0, D3D11_VIDEO_FRAME_FORMAT_PROGRESSIVE);
    vctx_->VideoProcessorSetStreamAutoProcessingMode(proc_.get(), 0, FALSE);
    in_w_ = w;
    in_h_ = h;
    // destination rectangle with the source's aspect ratio
    double sa = double(w) / double(h), oa = double(out_w_) / double(out_h_);
    RECT dst{0, 0, out_w_, out_h_};
    if (sa > oa) {
        int hh = int(std::lround(out_w_ / sa)) & ~1;
        dst.top = (out_h_ - hh) / 2 & ~1;
        dst.bottom = dst.top + hh;
    } else if (sa < oa) {
        int ww = int(std::lround(out_h_ * sa)) & ~1;
        dst.left = (out_w_ - ww) / 2 & ~1;
        dst.right = dst.left + ww;
    }
    RECT src{0, 0, (LONG)w, (LONG)h};
    vctx_->VideoProcessorSetStreamSourceRect(proc_.get(), 0, TRUE, &src);
    vctx_->VideoProcessorSetStreamDestRect(proc_.get(), 0, TRUE, &dst);
    vctx_->VideoProcessorSetOutputTargetRect(proc_.get(), TRUE, nullptr);
    return true;
}

ID3D11Texture2D* Converter::Convert(ID3D11Texture2D* src, std::string& err) {
    const size_t i = next_++ % outs_.size();
    if (!src) src = black_.get(); // no picture from the window yet
    D3D11_TEXTURE2D_DESC d{};
    src->GetDesc(&d);
    if (!Prepare(d.Width, d.Height, err)) return nullptr;
    D3D11_VIDEO_PROCESSOR_INPUT_VIEW_DESC ivd{};
    ivd.ViewDimension = D3D11_VPIV_DIMENSION_TEXTURE2D;
    winrt::com_ptr<ID3D11VideoProcessorInputView> iv;
    HRESULT hr = vdev_->CreateVideoProcessorInputView(src, enum_.get(), &ivd, iv.put());
    if (FAILED(hr)) {
        err = "video input view: " + HrText(hr);
        return nullptr;
    }
    D3D11_VIDEO_PROCESSOR_STREAM stream{};
    stream.Enable = TRUE;
    stream.pInputSurface = iv.get();
    hr = vctx_->VideoProcessorBlt(proc_.get(), out_views_[i].get(), 0, 1, &stream);
    if (FAILED(hr)) {
        err = "colour conversion: " + HrText(hr);
        return nullptr;
    }
    return outs_[i].get();
}

// ---------------------------------------------------------------- writer

static winrt::com_ptr<IMFMediaType> VideoType(const GUID& sub, int w, int h, int fps, UINT32 bitrate, bool output, bool hevc) {
    winrt::com_ptr<IMFMediaType> t;
    MFCreateMediaType(t.put());
    t->SetGUID(MF_MT_MAJOR_TYPE, MFMediaType_Video);
    t->SetGUID(MF_MT_SUBTYPE, sub);
    MFSetAttributeSize(t.get(), MF_MT_FRAME_SIZE, w, h);
    MFSetAttributeRatio(t.get(), MF_MT_FRAME_RATE, fps, 1);
    MFSetAttributeRatio(t.get(), MF_MT_PIXEL_ASPECT_RATIO, 1, 1);
    t->SetUINT32(MF_MT_INTERLACE_MODE, MFVideoInterlace_Progressive);
    if (output) {
        t->SetUINT32(MF_MT_AVG_BITRATE, bitrate);
        t->SetUINT32(MF_MT_MPEG2_PROFILE, hevc ? eAVEncH265VProfile_Main_420_8 : eAVEncH264VProfile_High);
    } else {
        t->SetUINT32(MF_MT_DEFAULT_STRIDE, w);
        t->SetUINT32(MF_MT_ALL_SAMPLES_INDEPENDENT, TRUE);
    }
    return t;
}

static winrt::com_ptr<IMFMediaType> AudioType(bool aac, int rate, int channels) {
    winrt::com_ptr<IMFMediaType> t;
    MFCreateMediaType(t.put());
    t->SetGUID(MF_MT_MAJOR_TYPE, MFMediaType_Audio);
    t->SetGUID(MF_MT_SUBTYPE, aac ? MFAudioFormat_AAC : MFAudioFormat_PCM);
    t->SetUINT32(MF_MT_AUDIO_SAMPLES_PER_SECOND, rate);
    t->SetUINT32(MF_MT_AUDIO_NUM_CHANNELS, channels);
    t->SetUINT32(MF_MT_AUDIO_BITS_PER_SAMPLE, 16);
    if (aac) {
        t->SetUINT32(MF_MT_AUDIO_AVG_BYTES_PER_SECOND, 24000); // 192 kbit/s
    } else {
        t->SetUINT32(MF_MT_AUDIO_BLOCK_ALIGNMENT, 2 * channels);
        t->SetUINT32(MF_MT_AUDIO_AVG_BYTES_PER_SECOND, rate * 2 * channels);
    }
    return t;
}

bool Writer::Open(const Settings& s, ID3D11Device* dev, int w, int h, int mic_rate, int mic_channels, std::string& err) {
    std::string gpu_err;
    // frames stay on the GPU only for a hardware encoder on a real graphics card; otherwise they
    // are read back and given to the software encoder
    bool hw_encoder = false;
    if (s.hardware && s.gpu_device) {
        MFT_REGISTER_TYPE_INFO out{MFMediaType_Video, s.hevc ? MFVideoFormat_HEVC : MFVideoFormat_H264};
        IMFActivate** acts = nullptr;
        UINT32 n = 0;
        if (SUCCEEDED(MFTEnumEx(MFT_CATEGORY_VIDEO_ENCODER, MFT_ENUM_FLAG_HARDWARE | MFT_ENUM_FLAG_SORTANDFILTER, nullptr, &out, &acts, &n))) {
            hw_encoder = n > 0;
            for (UINT32 i = 0; i < n; ++i) acts[i]->Release();
            CoTaskMemFree(acts);
        }
    }
    if (hw_encoder && OpenMode(s, dev, w, h, true, mic_rate, mic_channels, gpu_err)) return true;
    writer_ = nullptr;
    manager_ = nullptr;
    if (!OpenMode(s, dev, w, h, false, mic_rate, mic_channels, err)) {
        if (!gpu_err.empty()) err += " (GPU path: " + gpu_err + ")";
        return false;
    }
    return true;
}

bool Writer::OpenMode(const Settings& s, ID3D11Device* dev, int w, int h, bool gpu, int mic_rate, int mic_channels, std::string& err) {
    dev_.copy_from(dev);
    dev_->GetImmediateContext(ctx_.put());
    w_ = w;
    h_ = h;
    gpu_ = gpu;
    winrt::com_ptr<IMFAttributes> attrs;
    NX_CHECK(MFCreateAttributes(attrs.put(), 6), "MF attributes");
    attrs->SetUINT32(MF_READWRITE_ENABLE_HARDWARE_TRANSFORMS, s.hardware ? TRUE : FALSE);
    attrs->SetGUID(MF_TRANSCODE_CONTAINERTYPE, MFTranscodeContainerType_FMPEG4);
    attrs->SetUINT32(MF_SINK_WRITER_DISABLE_THROTTLING, TRUE);
    attrs->SetUINT32(MF_LOW_LATENCY, TRUE);
    if (gpu) {
        UINT token = 0;
        NX_CHECK(MFCreateDXGIDeviceManager(&token, manager_.put()), "DXGI device manager");
        NX_CHECK(manager_->ResetDevice(dev_.get(), token), "DXGI device manager");
        attrs->SetUnknown(MF_SINK_WRITER_D3D_MANAGER, manager_.get());
    }
    const std::wstring part = s.path + L".part";
    DeleteFileW(part.c_str());
    NX_CHECK(MFCreateSinkWriterFromURL(part.c_str(), nullptr, attrs.get(), writer_.put()), "MP4 writer");
    const UINT32 bitrate = UINT32(std::clamp(s.bitrate_kbps, 1000, 150000)) * 1000;
    auto out = VideoType(s.hevc ? MFVideoFormat_HEVC : MFVideoFormat_H264, w, h, s.fps, bitrate, true, s.hevc);
    NX_CHECK(writer_->AddStream(out.get(), &video_stream_), s.hevc ? "HEVC video stream" : "H.264 video stream");
    auto in = VideoType(MFVideoFormat_NV12, w, h, s.fps, 0, false, false);
    NX_CHECK(writer_->SetInputMediaType(video_stream_, in.get(), nullptr), "video encoder input");
    if (s.audio) {
        auto aout = AudioType(true, 48000, 2);
        NX_CHECK(writer_->AddStream(aout.get(), &audio_stream_), "AAC audio stream");
        auto ain = AudioType(false, 48000, 2);
        NX_CHECK(writer_->SetInputMediaType(audio_stream_, ain.get(), nullptr), "audio encoder input");
    }
    if (s.mic && mic_rate > 0) {
        auto mout = AudioType(true, mic_rate, mic_channels);
        auto min = AudioType(false, mic_rate, mic_channels);
        if (SUCCEEDED(writer_->AddStream(mout.get(), &mic_stream_)) &&
            FAILED(writer_->SetInputMediaType(mic_stream_, min.get(), nullptr))) {
            mic_stream_ = MAXDWORD;
        }
    }
    NX_CHECK(writer_->BeginWriting(), "start writing");
    // which encoder Media Foundation chose
    encoder_ = s.hevc ? "HEVC" : "H.264";
    if (auto ex = writer_.try_as<IMFSinkWriterEx>()) {
        for (DWORD i = 0;; ++i) {
            GUID cat{};
            winrt::com_ptr<IMFTransform> mft;
            if (FAILED(ex->GetTransformForStream(video_stream_, i, &cat, mft.put()))) break;
            if (cat != MFT_CATEGORY_VIDEO_ENCODER) continue;
            winrt::com_ptr<IMFAttributes> a;
            if (SUCCEEDED(mft->GetAttributes(a.put()))) {
                wchar_t name[256]{};
                if (SUCCEEDED(a->GetString(MFT_FRIENDLY_NAME_Attribute, name, 256, nullptr))) encoder_ = Narrow(name);
            }
            if (encoder_ == "H.264" || encoder_ == "HEVC") encoder_ += " (Media Foundation)";
            break;
        }
    }
    if (!gpu) {
        D3D11_TEXTURE2D_DESC td{};
        td.Width = w;
        td.Height = h;
        td.MipLevels = 1;
        td.ArraySize = 1;
        td.Format = DXGI_FORMAT_NV12;
        td.SampleDesc.Count = 1;
        td.Usage = D3D11_USAGE_STAGING;
        td.CPUAccessFlags = D3D11_CPU_ACCESS_READ;
        NX_CHECK(dev_->CreateTexture2D(&td, nullptr, staging_.put()), "staging texture");
    }
    return true;
}

bool Writer::WriteVideo(ID3D11Texture2D* nv12, LONGLONG time, LONGLONG duration, std::string& err) {
    winrt::com_ptr<IMFMediaBuffer> buf;
    if (gpu_) {
        NX_CHECK(MFCreateDXGISurfaceBuffer(__uuidof(ID3D11Texture2D), nv12, 0, FALSE, buf.put()), "frame buffer");
        DWORD len = 0;
        if (auto b2 = buf.try_as<IMF2DBuffer>()) b2->GetContiguousLength(&len);
        buf->SetCurrentLength(len);
    } else {
        ctx_->CopyResource(staging_.get(), nv12);
        D3D11_MAPPED_SUBRESOURCE m{};
        NX_CHECK(ctx_->Map(staging_.get(), 0, D3D11_MAP_READ, 0, &m), "read frame");
        const DWORD size = DWORD(w_) * h_ * 3 / 2;
        HRESULT hr = MFCreateMemoryBuffer(size, buf.put());
        if (FAILED(hr)) {
            ctx_->Unmap(staging_.get(), 0);
            err = "frame memory: " + HrText(hr);
            return false;
        }
        BYTE* dst = nullptr;
        buf->Lock(&dst, nullptr, nullptr);
        const BYTE* src = static_cast<const BYTE*>(m.pData);
        for (int y = 0; y < h_; ++y) std::memcpy(dst + size_t(y) * w_, src + size_t(y) * m.RowPitch, w_);
        const BYTE* uv = src + size_t(m.RowPitch) * h_;
        BYTE* duv = dst + size_t(w_) * h_;
        for (int y = 0; y < h_ / 2; ++y) std::memcpy(duv + size_t(y) * w_, uv + size_t(y) * m.RowPitch, w_);
        buf->Unlock();
        buf->SetCurrentLength(size);
        ctx_->Unmap(staging_.get(), 0);
    }
    winrt::com_ptr<IMFSample> sample;
    NX_CHECK(MFCreateSample(sample.put()), "sample");
    sample->AddBuffer(buf.get());
    sample->SetSampleTime(time);
    sample->SetSampleDuration(duration);
    std::scoped_lock lk{mu_};
    NX_CHECK(writer_->WriteSample(video_stream_, sample.get()), "encode video");
    return true;
}

bool Writer::WriteAudio(int track, const int16_t* pcm, size_t frames, int channels, int rate, LONGLONG time, std::string& err) {
    const DWORD stream = track == 0 ? audio_stream_ : mic_stream_;
    if (stream == MAXDWORD || frames == 0) return true;
    const DWORD bytes = DWORD(frames * channels * sizeof(int16_t));
    winrt::com_ptr<IMFMediaBuffer> buf;
    NX_CHECK(MFCreateMemoryBuffer(bytes, buf.put()), "audio memory");
    BYTE* dst = nullptr;
    buf->Lock(&dst, nullptr, nullptr);
    std::memcpy(dst, pcm, bytes);
    buf->Unlock();
    buf->SetCurrentLength(bytes);
    winrt::com_ptr<IMFSample> sample;
    NX_CHECK(MFCreateSample(sample.put()), "sample");
    sample->AddBuffer(buf.get());
    sample->SetSampleTime(time);
    sample->SetSampleDuration(LONGLONG(frames) * 10'000'000 / rate);
    std::scoped_lock lk{mu_};
    NX_CHECK(writer_->WriteSample(stream, sample.get()), "encode audio");
    return true;
}

bool Writer::Finish(std::string& err) {
    std::scoped_lock lk{mu_};
    if (!writer_) return true;
    HRESULT hr = writer_->Finalize();
    writer_ = nullptr;
    manager_ = nullptr;
    if (FAILED(hr)) {
        err = "finishing the file: " + HrText(hr);
        return false;
    }
    return true;
}

// ---------------------------------------------------------------- window capture

struct Recorder::Capture {
    wgc::GraphicsCaptureItem item{nullptr};
    wdx::Direct3D11::IDirect3DDevice device{nullptr};
    wgc::Direct3D11CaptureFramePool pool{nullptr};
    wgc::GraphicsCaptureSession session{nullptr};
    winrt::event_token token{};
    winrt::Windows::Graphics::SizeInt32 size{};
};

static wdx::Direct3D11::IDirect3DDevice WinrtDevice(ID3D11Device* dev) {
    winrt::com_ptr<IDXGIDevice> dxgi;
    winrt::check_hresult(dev->QueryInterface(IID_PPV_ARGS(dxgi.put())));
    winrt::com_ptr<::IInspectable> insp;
    winrt::check_hresult(CreateDirect3D11DeviceFromDXGIDevice(dxgi.get(), insp.put()));
    return insp.as<wdx::Direct3D11::IDirect3DDevice>();
}

template <typename T>
static winrt::com_ptr<T> SurfaceTexture(const wdx::Direct3D11::IDirect3DSurface& surface) {
    auto access = surface.as<::Windows::Graphics::DirectX::Direct3D11::IDirect3DDxgiInterfaceAccess>();
    winrt::com_ptr<T> out;
    winrt::check_hresult(access->GetInterface(winrt::guid_of<T>(), out.put_void()));
    return out;
}

/// The game's client area inside a captured window frame (no title bar or borders when the game
/// runs in a window; the whole frame in fullscreen).
static D3D11_BOX ClientBox(HWND hwnd, UINT fw, UINT fh) {
    D3D11_BOX box{0, 0, 0, fw, fh, 1};
    RECT frame{}, client{};
    if (FAILED(DwmGetWindowAttribute(hwnd, DWMWA_EXTENDED_FRAME_BOUNDS, &frame, sizeof(frame))) || !GetClientRect(hwnd, &client)) return box;
    POINT tl{0, 0};
    ClientToScreen(hwnd, &tl);
    const LONG x = std::max<LONG>(0, tl.x - frame.left), y = std::max<LONG>(0, tl.y - frame.top);
    const LONG w = client.right - client.left, h = client.bottom - client.top;
    if (w < 16 || h < 16 || UINT(x + w) > fw || UINT(y + h) > fh) return box;
    box.left = UINT(x);
    box.top = UINT(y);
    box.right = UINT(x + w);
    box.bottom = UINT(y + h);
    return box;
}

// ---------------------------------------------------------------- recorder

Recorder::~Recorder() {
    std::string e;
    Stop(e);
}

LONGLONG Recorder::Now100ns() const {
    LARGE_INTEGER now;
    QueryPerformanceCounter(&now);
    return LONGLONG((now.QuadPart - qpc0_.QuadPart) * 10'000'000.0 / qpf_.QuadPart);
}

void Recorder::Fail(const std::string& why) {
    if (failed_.exchange(true)) return;
    std::scoped_lock lk{st_mu_};
    st_.state = "error";
    st_.error = why;
    stop_ = true;
}

static bool EvenSize(int& w, int& h) {
    w &= ~1;
    h &= ~1;
    return w >= 64 && h >= 64;
}

bool Recorder::Start(const Settings& s, std::string& err) {
    s_ = s;
    s_.fps = std::clamp(s_.fps, 24, 120);
    bool hw = false;
    dev_ = MakeDevice(true, err, &hw);
    if (!dev_) return false;
    s_.gpu_device = hw;
    dev_->GetImmediateContext(ctx_.put());

    // source size: the window's client area (or the requested size for the self-test)
    int src_w = 1280, src_h = 720;
    if (!s_.synthetic) {
        if (!IsWindow(s_.hwnd)) {
            err = "The game window is not there (has the game closed?)";
            return false;
        }
        try {
            if (!wgc::GraphicsCaptureSession::IsSupported()) {
                err = "Windows Graphics Capture is not available on this version of Windows (Windows 10 1903 or later is needed)";
                return false;
            }
            cap_ = new Capture;
            cap_->device = WinrtDevice(dev_.get());
            auto interop = winrt::get_activation_factory<wgc::GraphicsCaptureItem, IGraphicsCaptureItemInterop>();
            winrt::check_hresult(interop->CreateForWindow(s_.hwnd, winrt::guid_of<ABI::Windows::Graphics::Capture::IGraphicsCaptureItem>(),
                                                          winrt::put_abi(cap_->item)));
            cap_->size = cap_->item.Size();
            RECT rc{};
            GetClientRect(s_.hwnd, &rc);
            src_w = rc.right - rc.left > 0 ? rc.right - rc.left : cap_->size.Width;
            src_h = rc.bottom - rc.top > 0 ? rc.bottom - rc.top : cap_->size.Height;
        } catch (const winrt::hresult_error& e) {
            err = "The game window cannot be captured: " + HrText(e.code()) ;
            delete cap_;
            cap_ = nullptr;
            return false;
        }
    }
    out_w_ = s_.width > 0 ? s_.width : src_w;
    out_h_ = s_.height > 0 ? s_.height : src_h;
    // H.264 level 5.2 (and most hardware encoders) top out at 4096 wide
    if (out_w_ > 3840 || out_h_ > 2160) {
        const double k = std::min(3840.0 / out_w_, 2160.0 / out_h_);
        out_w_ = int(out_w_ * k);
        out_h_ = int(out_h_ * k);
    }
    if (!EvenSize(out_w_, out_h_)) {
        err = "The game window is too small to record";
        return false;
    }
    if (!conv_.Init(dev_.get(), out_w_, out_h_, s_.fps, err)) return false;

    // microphone format (its own track)
    int mic_rate = 0, mic_ch = 0;
    if (s_.mic && !s_.synthetic) {
        winrt::com_ptr<IMMDeviceEnumerator> en;
        winrt::com_ptr<IMMDevice> devc;
        winrt::com_ptr<IAudioClient> ac;
        if (SUCCEEDED(CoCreateInstance(__uuidof(MMDeviceEnumerator), nullptr, CLSCTX_ALL, __uuidof(IMMDeviceEnumerator), en.put_void())) &&
            SUCCEEDED(en->GetDefaultAudioEndpoint(eCapture, eConsole, devc.put())) &&
            SUCCEEDED(devc->Activate(__uuidof(IAudioClient), CLSCTX_ALL, nullptr, ac.put_void()))) {
            WAVEFORMATEX* mf = nullptr;
            if (SUCCEEDED(ac->GetMixFormat(&mf))) {
                if ((mf->nSamplesPerSec == 48000 || mf->nSamplesPerSec == 44100) && mf->nChannels >= 1) {
                    mic_rate = mf->nSamplesPerSec;
                    mic_ch = mf->nChannels >= 2 ? 2 : 1;
                }
                CoTaskMemFree(mf);
            }
        }
    } else if (s_.mic && s_.synthetic) {
        mic_rate = 48000;
        mic_ch = 1;
    }
    if (!writer_.Open(s_, dev_.get(), out_w_, out_h_, mic_rate, mic_ch, err)) {
        delete cap_;
        cap_ = nullptr;
        return false;
    }
    mic_on_ = writer_.HasMic();
    QueryPerformanceFrequency(&qpf_);
    QueryPerformanceCounter(&qpc0_);
    {
        std::scoped_lock lk{st_mu_};
        st_ = Status{};
        st_.state = "recording";
        st_.width = out_w_;
        st_.height = out_h_;
        st_.encoder = writer_.EncoderName();
        st_.gpu = writer_.Gpu();
        st_.path = Narrow(s_.path);
    }
    if (cap_) {
        try {
            cap_->pool = wgc::Direct3D11CaptureFramePool::CreateFreeThreaded(cap_->device, wdx::DirectXPixelFormat::B8G8R8A8UIntNormalized, 2, cap_->size);
            cap_->session = cap_->pool.CreateCaptureSession(cap_->item);
            try { cap_->session.IsCursorCaptureEnabled(false); } catch (...) {}
            try { cap_->session.IsBorderRequired(false); } catch (...) {} // Windows 11: no yellow border
            cap_->token = cap_->pool.FrameArrived([this](const wgc::Direct3D11CaptureFramePool& pool, const winrt::Windows::Foundation::IInspectable&) {
                try {
                    auto frame = pool.TryGetNextFrame();
                    if (!frame) return;
                    const auto size = frame.ContentSize();
                    auto tex = SurfaceTexture<ID3D11Texture2D>(frame.Surface());
                    D3D11_TEXTURE2D_DESC d{};
                    tex->GetDesc(&d);
                    // copy the game's client area into our own texture (the pool reuses its buffers)
                    D3D11_BOX box = ClientBox(s_.hwnd, std::min<UINT>(UINT(std::max(size.Width, 2)), d.Width),
                                              std::min<UINT>(UINT(std::max(size.Height, 2)), d.Height));
                    const UINT cw = (box.right - box.left) & ~1u;
                    const UINT ch = (box.bottom - box.top) & ~1u;
                    box.right = box.left + cw;
                    box.bottom = box.top + ch;
                    winrt::com_ptr<ID3D11Texture2D> copy;
                    D3D11_TEXTURE2D_DESC cd = d;
                    cd.Width = cw;
                    cd.Height = ch;
                    cd.MiscFlags = 0;
                    cd.BindFlags = D3D11_BIND_SHADER_RESOURCE | D3D11_BIND_RENDER_TARGET;
                    cd.Usage = D3D11_USAGE_DEFAULT;
                    cd.CPUAccessFlags = 0;
                    {
                        std::scoped_lock lk{frame_mu_};
                        if (latest_) {
                            D3D11_TEXTURE2D_DESC ld{};
                            latest_->GetDesc(&ld);
                            if (ld.Width == cw && ld.Height == ch) copy = latest_;
                        }
                    }
                    if (!copy && FAILED(dev_->CreateTexture2D(&cd, nullptr, copy.put()))) return;
                    ctx_->CopySubresourceRegion(copy.get(), 0, 0, 0, 0, tex.get(), 0, &box);
                    {
                        std::scoped_lock lk{frame_mu_};
                        latest_ = copy;
                        have_frame_ = true;
                    }
                    // the window changed size (F11, resizing): the pool follows it
                    if (size.Width != cap_->size.Width || size.Height != cap_->size.Height) {
                        cap_->size = size;
                        pool.Recreate(cap_->device, wdx::DirectXPixelFormat::B8G8R8A8UIntNormalized, 2, size);
                    }
                } catch (...) {
                }
            });
            cap_->session.StartCapture();
        } catch (const winrt::hresult_error& e) {
            err = "The game window cannot be captured: " + HrText(e.code());
            std::string ignore;
            writer_.Finish(ignore);
            return false;
        }
    }
    stop_ = false;
    failed_ = false;
    video_ = std::thread([this] { VideoThread(); });
    if (s_.audio) audio_ = std::thread([this] { AudioThread(false); });
    if (mic_on_) mic_ = std::thread([this] { AudioThread(true); });
    return true;
}

void Recorder::VideoThread() {
    winrt::init_apartment(winrt::apartment_type::multi_threaded);
    const LONGLONG frame_100ns = 10'000'000LL / s_.fps;
    uint64_t index = 0;
    const uint64_t total = s_.synthetic ? uint64_t(s_.synthetic_seconds) * s_.fps : UINT64_MAX;
    winrt::com_ptr<ID3D11Texture2D> synth;
    if (s_.synthetic) {
        D3D11_TEXTURE2D_DESC td{};
        td.Width = 1280;
        td.Height = 720;
        td.MipLevels = 1;
        td.ArraySize = 1;
        td.Format = DXGI_FORMAT_B8G8R8A8_UNORM;
        td.SampleDesc.Count = 1;
        td.Usage = D3D11_USAGE_DEFAULT;
        td.BindFlags = D3D11_BIND_SHADER_RESOURCE | D3D11_BIND_RENDER_TARGET;
        dev_->CreateTexture2D(&td, nullptr, synth.put());
    }
    std::vector<uint32_t> pixels;
    while (!stop_ && index < total) {
        const LONGLONG due = LONGLONG(index) * frame_100ns;
        if (!s_.synthetic) {
            LONGLONG now = Now100ns();
            if (now < due) {
                std::this_thread::sleep_for(std::chrono::microseconds((due - now) / 10));
                continue;
            }
            // more than two frames late: skip ahead (counted as dropped) to keep real time
            if (now - due > 2 * frame_100ns) {
                const uint64_t skip = uint64_t((now - due) / frame_100ns);
                dropped_ += skip;
                index += skip;
                continue;
            }
        }
        std::string err;
        ID3D11Texture2D* nv12 = nullptr;
        if (s_.synthetic) {
            // moving colour bars, so every frame differs
            pixels.resize(1280 * 720);
            for (int y = 0; y < 720; ++y)
                for (int x = 0; x < 1280; ++x)
                    pixels[size_t(y) * 1280 + x] = 0xFF000000u | (uint32_t((x + index * 8) & 255) << 16) | (uint32_t(y & 255) << 8) | uint32_t((index * 4) & 255);
            ctx_->UpdateSubresource(synth.get(), 0, nullptr, pixels.data(), 1280 * 4, 0);
            nv12 = conv_.Convert(synth.get(), err);
        } else {
            winrt::com_ptr<ID3D11Texture2D> src;
            {
                std::scoped_lock lk{frame_mu_};
                if (have_frame_) src = latest_;
            }
            nv12 = src ? conv_.Convert(src.get(), err) : conv_.Black();
        }
        if (!nv12 || !writer_.WriteVideo(nv12, due, frame_100ns, err)) {
            Fail(err.empty() ? "video encoding failed" : err);
            break;
        }
        ++frames_;
        ++index;
        written_100ns_ = due + frame_100ns;
    }
    winrt::uninit_apartment();
}

// IActivateAudioInterfaceCompletionHandler for process loopback activation
struct ActivateHandler : winrt::implements<ActivateHandler, IActivateAudioInterfaceCompletionHandler, ::IAgileObject> {
    HANDLE done = CreateEventW(nullptr, TRUE, FALSE, nullptr);
    HRESULT result = E_FAIL;
    winrt::com_ptr<IAudioClient> client;
    ~ActivateHandler() { CloseHandle(done); }
    HRESULT STDMETHODCALLTYPE ActivateCompleted(IActivateAudioInterfaceAsyncOperation* op) override {
        HRESULT hr = E_FAIL;
        winrt::com_ptr<::IUnknown> unk;
        if (SUCCEEDED(op->GetActivateResult(&hr, unk.put())) && SUCCEEDED(hr)) {
            client = unk.try_as<IAudioClient>();
            result = client ? S_OK : E_NOINTERFACE;
        } else {
            result = FAILED(hr) ? hr : E_FAIL;
        }
        SetEvent(done);
        return S_OK;
    }
};

void Recorder::AudioThread(bool microphone) {
    winrt::init_apartment(winrt::apartment_type::multi_threaded);
    const int track = microphone ? 1 : 0;
    std::string err;
    if (s_.synthetic) {
        // a 440 Hz tone, 10 ms packets, with a gap in the middle (filled with silence)
        const int rate = 48000, ch = microphone ? 1 : 2;
        std::vector<int16_t> pcm(480 * ch);
        LONGLONG t = 0;
        const LONGLONG end = LONGLONG(s_.synthetic_seconds) * 10'000'000;
        size_t n = 0;
        while (!stop_ && t < end) {
            for (int i = 0; i < 480; ++i) {
                const int16_t v = int16_t(8000 * std::sin(2 * 3.14159265 * 440 * double(n + i) / rate));
                for (int c = 0; c < ch; ++c) pcm[size_t(i) * ch + c] = v;
            }
            if (!writer_.WriteAudio(track, pcm.data(), 480, ch, rate, t, err)) {
                Fail(err);
                break;
            }
            n += 480;
            t += 100'000;
        }
        {
            std::scoped_lock lk{st_mu_};
            if (!microphone) audio_mode_ = "process";
        }
        winrt::uninit_apartment();
        return;
    }

    winrt::com_ptr<IAudioClient> client;
    WAVEFORMATEX fmt{};
    fmt.wFormatTag = WAVE_FORMAT_PCM;
    fmt.nChannels = 2;
    fmt.nSamplesPerSec = 48000;
    fmt.wBitsPerSample = 16;
    fmt.nBlockAlign = 4;
    fmt.nAvgBytesPerSec = 48000 * 4;
    bool is_float = false;
    int rate = 48000, channels = 2;
    std::string mode = microphone ? "mic" : "process";
    DWORD stream_flags = AUDCLNT_STREAMFLAGS_EVENTCALLBACK;
    if (!microphone) {
        // only NEXUS's own sound (the game), not the rest of the PC
        AUDIOCLIENT_ACTIVATION_PARAMS params{};
        params.ActivationType = AUDIOCLIENT_ACTIVATION_TYPE_PROCESS_LOOPBACK;
        params.ProcessLoopbackParams.TargetProcessId = s_.pid ? s_.pid : GetCurrentProcessId();
        params.ProcessLoopbackParams.ProcessLoopbackMode = PROCESS_LOOPBACK_MODE_INCLUDE_TARGET_PROCESS_TREE;
        PROPVARIANT pv{};
        pv.vt = VT_BLOB;
        pv.blob.cbSize = sizeof(params);
        pv.blob.pBlobData = reinterpret_cast<BYTE*>(&params);
        auto handler = winrt::make_self<ActivateHandler>();
        winrt::com_ptr<IActivateAudioInterfaceAsyncOperation> op;
        auto completion = handler.as<IActivateAudioInterfaceCompletionHandler>();
        if (SUCCEEDED(ActivateAudioInterfaceAsync(VIRTUAL_AUDIO_DEVICE_PROCESS_LOOPBACK, __uuidof(IAudioClient), &pv, completion.get(), op.put())) &&
            WaitForSingleObject(handler->done, 5000) == WAIT_OBJECT_0 && SUCCEEDED(handler->result)) {
            client = handler->client;
            stream_flags |= AUDCLNT_STREAMFLAGS_LOOPBACK;
            if (FAILED(client->Initialize(AUDCLNT_SHAREMODE_SHARED, stream_flags, 2'000'000, 0, &fmt, nullptr))) client = nullptr;
        }
        if (!client) mode = "system"; // older Windows: the whole PC's sound
    }
    if (!client) {
        winrt::com_ptr<IMMDeviceEnumerator> en;
        winrt::com_ptr<IMMDevice> devc;
        if (FAILED(CoCreateInstance(__uuidof(MMDeviceEnumerator), nullptr, CLSCTX_ALL, __uuidof(IMMDeviceEnumerator), en.put_void())) ||
            FAILED(en->GetDefaultAudioEndpoint(microphone ? eCapture : eRender, eConsole, devc.put())) ||
            FAILED(devc->Activate(__uuidof(IAudioClient), CLSCTX_ALL, nullptr, client.put_void()))) {
            std::scoped_lock lk{st_mu_};
            if (!microphone) audio_mode_ = "off";
            winrt::uninit_apartment();
            return;
        }
        WAVEFORMATEX* mix = nullptr;
        client->GetMixFormat(&mix);
        rate = mix->nSamplesPerSec;
        channels = mix->nChannels;
        is_float = mix->wFormatTag == WAVE_FORMAT_IEEE_FLOAT ||
                   (mix->wFormatTag == WAVE_FORMAT_EXTENSIBLE && reinterpret_cast<WAVEFORMATEXTENSIBLE*>(mix)->SubFormat.Data1 == WAVE_FORMAT_IEEE_FLOAT);
        const int bits = mix->wBitsPerSample;
        DWORD flags = AUDCLNT_STREAMFLAGS_EVENTCALLBACK | (microphone ? 0 : AUDCLNT_STREAMFLAGS_LOOPBACK);
        HRESULT hr = client->Initialize(AUDCLNT_SHAREMODE_SHARED, flags, 2'000'000, 0, mix, nullptr);
        CoTaskMemFree(mix);
        if (FAILED(hr) || !(is_float || bits == 16)) {
            std::scoped_lock lk{st_mu_};
            if (!microphone) audio_mode_ = "off";
            winrt::uninit_apartment();
            return;
        }
    }
    const int out_ch = microphone ? (channels >= 2 ? 2 : 1) : 2;
    if (!microphone && rate != 48000) {
        // the system mix runs at another rate: the game-audio track needs 48 kHz
        std::scoped_lock lk{st_mu_};
        audio_mode_ = "off";
        winrt::uninit_apartment();
        return;
    }
    HANDLE ev = CreateEventW(nullptr, FALSE, FALSE, nullptr);
    client->SetEventHandle(ev);
    winrt::com_ptr<IAudioCaptureClient> capture;
    if (FAILED(client->GetService(__uuidof(IAudioCaptureClient), capture.put_void())) || FAILED(client->Start())) {
        CloseHandle(ev);
        winrt::uninit_apartment();
        return;
    }
    {
        std::scoped_lock lk{st_mu_};
        if (!microphone) audio_mode_ = mode;
    }
    std::vector<int16_t> out;
    LONGLONG next = -1; // where the next audio sample belongs on the timeline (100 ns)
    while (!stop_) {
        if (WaitForSingleObject(ev, 100) != WAIT_OBJECT_0) {
            // nothing to capture (the game is silent): keep the track continuous with silence
            const LONGLONG now = Now100ns();
            if (next >= 0 && now - next > 2'000'000) {
                const size_t frames = size_t((now - next) * rate / 10'000'000);
                out.assign(frames * out_ch, 0);
                if (!writer_.WriteAudio(track, out.data(), frames, out_ch, rate, next, err)) Fail(err);
                next += LONGLONG(frames) * 10'000'000 / rate;
            }
            continue;
        }
        UINT32 packet = 0;
        while (SUCCEEDED(capture->GetNextPacketSize(&packet)) && packet > 0) {
            BYTE* data = nullptr;
            UINT32 frames = 0;
            DWORD flags = 0;
            UINT64 qpc_pos = 0;
            if (FAILED(capture->GetBuffer(&data, &frames, &flags, nullptr, &qpc_pos))) break;
            LONGLONG t = LONGLONG(qpc_pos) - LONGLONG(qpc0_.QuadPart * 10'000'000.0 / qpf_.QuadPart);
            if (next < 0) next = std::max<LONGLONG>(t, 0);
            if (t > next + 400'000) { // a gap: silence up to this packet
                const size_t gap = size_t((t - next) * rate / 10'000'000);
                out.assign(gap * out_ch, 0);
                writer_.WriteAudio(track, out.data(), gap, out_ch, rate, next, err);
                next += LONGLONG(gap) * 10'000'000 / rate;
            }
            out.resize(size_t(frames) * out_ch);
            if ((flags & AUDCLNT_BUFFERFLAGS_SILENT) || !data) {
                std::fill(out.begin(), out.end(), int16_t(0));
            } else {
                for (UINT32 i = 0; i < frames; ++i) {
                    for (int c = 0; c < out_ch; ++c) {
                        const int sc = std::min(c, channels - 1);
                        int16_t v;
                        if (is_float) {
                            const float f = reinterpret_cast<const float*>(data)[size_t(i) * channels + sc];
                            v = int16_t(std::clamp(f, -1.0f, 1.0f) * 32767.0f);
                        } else {
                            v = reinterpret_cast<const int16_t*>(data)[size_t(i) * channels + sc];
                        }
                        out[size_t(i) * out_ch + c] = v;
                    }
                }
            }
            capture->ReleaseBuffer(frames);
            if (next >= 0 && t >= 0) {
                if (!writer_.WriteAudio(track, out.data(), frames, out_ch, rate, next, err)) {
                    Fail(err);
                    break;
                }
                next += LONGLONG(frames) * 10'000'000 / rate;
            }
        }
    }
    client->Stop();
    CloseHandle(ev);
    winrt::uninit_apartment();
}

bool Recorder::Stop(std::string& err) {
    {
        std::scoped_lock lk{st_mu_};
        if (st_.state == "idle") return true;
        if (st_.state == "recording") st_.state = "stopping";
    }
    stop_ = true;
    if (cap_) {
        try {
            if (cap_->pool) cap_->pool.FrameArrived(cap_->token);
            if (cap_->session) cap_->session.Close();
            if (cap_->pool) cap_->pool.Close();
        } catch (...) {
        }
    }
    if (video_.joinable()) video_.join();
    if (audio_.joinable()) audio_.join();
    if (mic_.joinable()) mic_.join();
    delete cap_;
    cap_ = nullptr;
    std::string ferr;
    const bool ok = writer_.Finish(ferr);
    const std::wstring part = s_.path + L".part";
    bool renamed = false;
    if (GetFileAttributesW(part.c_str()) != INVALID_FILE_ATTRIBUTES) {
        renamed = MoveFileExW(part.c_str(), s_.path.c_str(), MOVEFILE_WRITE_THROUGH) != FALSE;
    }
    std::scoped_lock lk{st_mu_};
    if (failed_) {
        err = st_.error;
        st_.state = "error";
        return false;
    }
    if (!ok || !renamed) {
        err = !ok ? ferr : "the recording could not be renamed from .part";
        st_.state = "error";
        st_.error = err;
        return false;
    }
    st_.state = "idle";
    st_.seconds = double(written_100ns_.load()) / 1e7;
    return true;
}

Status Recorder::GetStatus() {
    std::scoped_lock lk{st_mu_};
    Status s = st_;
    s.frames = frames_;
    s.dropped = dropped_;
    if (s.state == "recording" || s.state == "stopping") s.seconds = double(written_100ns_.load()) / 1e7;
    else if (s.seconds == 0) s.seconds = double(written_100ns_.load()) / 1e7;
    s.audio = s_.audio ? audio_mode_ : "off";
    s.mic = mic_on_;
    return s;
}

// ---------------------------------------------------------------- capabilities and self-test

static std::string Esc(const std::string& s) {
    std::string o;
    for (char c : s) {
        if (c == '"' || c == '\\') o += '\\';
        if ((unsigned char)c < 0x20) continue;
        o += c;
    }
    return o;
}

std::string CapabilitiesJson() {
    std::ostringstream j;
    j << "{\"encoders\":[";
    bool first = true;
    for (int codec = 0; codec < 2; ++codec) {
        for (int hwpass = 0; hwpass < 2; ++hwpass) {
            MFT_REGISTER_TYPE_INFO out{MFMediaType_Video, codec == 0 ? MFVideoFormat_H264 : MFVideoFormat_HEVC};
            IMFActivate** acts = nullptr;
            UINT32 n = 0;
            const UINT32 flags = (hwpass ? MFT_ENUM_FLAG_HARDWARE : (MFT_ENUM_FLAG_SYNCMFT | MFT_ENUM_FLAG_ASYNCMFT)) | MFT_ENUM_FLAG_SORTANDFILTER;
            if (FAILED(MFTEnumEx(MFT_CATEGORY_VIDEO_ENCODER, flags, nullptr, &out, &acts, &n))) continue;
            for (UINT32 i = 0; i < n; ++i) {
                wchar_t name[256]{};
                acts[i]->GetString(MFT_FRIENDLY_NAME_Attribute, name, 256, nullptr);
                wchar_t vend[64]{};
                acts[i]->GetString(MFT_ENUM_HARDWARE_VENDOR_ID_Attribute, vend, 64, nullptr);
                std::wstring v = vend;
                std::string vendor = "other";
                if (v.find(L"10DE") != std::wstring::npos) vendor = "nvidia";
                else if (v.find(L"1002") != std::wstring::npos || v.find(L"1022") != std::wstring::npos) vendor = "amd";
                else if (v.find(L"8086") != std::wstring::npos) vendor = "intel";
                else if (!hwpass) vendor = "microsoft";
                j << (first ? "" : ",") << "{\"name\":\"" << Esc(Narrow(name)) << "\",\"vendor\":\"" << vendor << "\",\"hardware\":" << (hwpass ? "true" : "false")
                  << ",\"codec\":\"" << (codec == 0 ? "h264" : "hevc") << "\"}";
                first = false;
                acts[i]->Release();
            }
            CoTaskMemFree(acts);
        }
    }
    bool wgc_ok = false;
    try {
        wgc_ok = wgc::GraphicsCaptureSession::IsSupported();
    } catch (...) {
    }
    // process loopback: Windows 10 build 20348 / Windows 11
    bool proc_audio = false;
    {
        using RtlGetVersionFn = LONG(WINAPI*)(OSVERSIONINFOW*);
        if (auto fn = reinterpret_cast<RtlGetVersionFn>(GetProcAddress(GetModuleHandleW(L"ntdll.dll"), "RtlGetVersion"))) {
            OSVERSIONINFOW vi{sizeof(vi)};
            if (fn(&vi) == 0) proc_audio = vi.dwMajorVersion > 10 || (vi.dwMajorVersion == 10 && vi.dwBuildNumber >= 20348);
        }
    }
    j << "],\"windowCapture\":" << (wgc_ok ? "true" : "false") << ",\"processAudio\":" << (proc_audio ? "true" : "false") << "}";
    return j.str();
}

std::string SelfTest(const std::wstring& path, int seconds) {
    std::ostringstream j;
    Settings s;
    s.path = path;
    s.width = 1280;
    s.height = 720;
    s.fps = 30;
    s.bitrate_kbps = 6000;
    s.audio = true;
    s.mic = true;
    s.synthetic = true;
    s.synthetic_seconds = std::clamp(seconds, 1, 30);
    Recorder r;
    std::string err;
    bool ok = r.Start(s, err);
    Status st;
    if (ok) {
        // the synthetic threads finish by themselves
        for (int i = 0; i < 600; ++i) {
            st = r.GetStatus();
            if (st.frames >= uint64_t(s.synthetic_seconds) * s.fps || st.state == "error") break;
            std::this_thread::sleep_for(std::chrono::milliseconds(50));
        }
        ok = r.Stop(err);
        st = r.GetStatus();
    }
    // read the file back: it must open and hold video and audio of the right length
    uint64_t vs = 0, as = 0;
    LONGLONG last = 0;
    std::string rerr;
    if (ok) {
        winrt::com_ptr<IMFSourceReader> reader;
        HRESULT hr = MFCreateSourceReaderFromURL(path.c_str(), nullptr, reader.put());
        if (FAILED(hr)) {
            rerr = "the file does not open: " + HrText(hr);
            ok = false;
        } else {
            for (;;) {
                DWORD stream = 0, flags = 0;
                LONGLONG ts = 0;
                winrt::com_ptr<IMFSample> sample;
                hr = reader->ReadSample(MF_SOURCE_READER_ANY_STREAM, 0, &stream, &flags, &ts, sample.put());
                if (FAILED(hr) || (flags & MF_SOURCE_READERF_ENDOFSTREAM)) break;
                if (!sample) continue;
                winrt::com_ptr<IMFMediaType> mt;
                reader->GetCurrentMediaType(stream, mt.put());
                GUID major{};
                if (mt) mt->GetGUID(MF_MT_MAJOR_TYPE, &major);
                if (major == MFMediaType_Video) ++vs;
                else ++as;
                last = std::max(last, ts);
            }
        }
    }
    const double dur = double(last) / 1e7;
    if (ok && (vs < uint64_t(s.synthetic_seconds) * s.fps * 9 / 10 || as == 0 || dur < s.synthetic_seconds - 1.0)) {
        ok = false;
        rerr = "the file is incomplete";
    }
    j << "{\"ok\":" << (ok ? "true" : "false") << ",\"encoder\":\"" << Esc(st.encoder) << "\",\"gpu\":" << (st.gpu ? "true" : "false")
      << ",\"mic\":" << (st.mic ? "true" : "false") << ",\"videoSamples\":" << vs << ",\"audioSamples\":" << as << ",\"duration\":" << dur
      << ",\"width\":" << st.width << ",\"height\":" << st.height << ",\"error\":\"" << Esc(!err.empty() ? err : rerr) << "\"}";
    return j.str();
}

} // namespace nexus::capture
