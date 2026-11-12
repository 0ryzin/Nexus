// SPDX-FileCopyrightText: 2026 NEXUS project
// SPDX-License-Identifier: GPL-3.0-or-later

#define NEXUS_CAPTURE_BUILD
#include "nexus_capture.h"
#include "recorder.h"

#include <algorithm>
#include <cstring>
#include <memory>
#include <thread>
#include <mutex>
#include <sstream>

using namespace nexus::capture;

namespace {

std::mutex g_mu;
std::unique_ptr<Recorder> g_rec;
std::once_flag g_mf_once;

void EnsureMF() {
    std::call_once(g_mf_once, [] {
        CoInitializeEx(nullptr, COINIT_MULTITHREADED);
        MFStartup(MF_VERSION, MFSTARTUP_FULL);
    });
}

int32_t Out(const std::string& s, char* buf, int32_t len) {
    if (buf && len > 0) {
        const size_t n = std::min<size_t>(s.size(), size_t(len - 1));
        std::memcpy(buf, s.data(), n);
        buf[n] = '\0';
    }
    return int32_t(s.size());
}

// one MTA thread for everything that needs COM: callers (Go) may run on any thread
template <typename F>
auto OnMta(F&& f) {
    decltype(f()) result{};
    std::thread t([&] {
        winrt::init_apartment(winrt::apartment_type::multi_threaded);
        result = f();
        winrt::uninit_apartment();
    });
    t.join();
    return result;
}

std::string Esc(const std::string& s) {
    std::string o;
    for (char c : s) {
        if (c == '"' || c == '\\') o += '\\';
        if ((unsigned char)c < 0x20) continue;
        o += c;
    }
    return o;
}

} // namespace

extern "C" {

NXCAP_API int32_t nxcap_api_version(void) { return NXCAP_API_VERSION; }

NXCAP_API int32_t nxcap_capabilities(char* buf, int32_t buf_len) {
    EnsureMF();
    return Out(OnMta([] { return CapabilitiesJson(); }), buf, buf_len);
}

NXCAP_API int32_t nxcap_start(const char* settings, char* err, int32_t err_len) {
    EnsureMF();
    Settings s;
    std::istringstream in(settings ? settings : "");
    std::string line;
    while (std::getline(in, line)) {
        const auto eq = line.find('=');
        if (eq == std::string::npos) continue;
        const std::string k = line.substr(0, eq), v = line.substr(eq + 1);
        const long long n = std::atoll(v.c_str());
        if (k == "hwnd") s.hwnd = reinterpret_cast<HWND>(static_cast<intptr_t>(n));
        else if (k == "path") s.path = Widen(v);
        else if (k == "width") s.width = int(n);
        else if (k == "height") s.height = int(n);
        else if (k == "fps") s.fps = int(n);
        else if (k == "bitrate") s.bitrate_kbps = int(n);
        else if (k == "codec") s.hevc = v == "hevc";
        else if (k == "audio") s.audio = n != 0;
        else if (k == "mic") s.mic = n != 0;
        else if (k == "hardware") s.hardware = n != 0;
        else if (k == "pid") s.pid = DWORD(n);
    }
    if (s.path.empty()) return Out("no file name", err, err_len), -1;
    std::scoped_lock lk{g_mu};
    if (g_rec) {
        const auto st = g_rec->GetStatus();
        if (st.state == "recording" || st.state == "stopping") return Out("already recording", err, err_len), -1;
        std::string e;
        g_rec->Stop(e);
        g_rec.reset();
    }
    std::string e;
    auto rec = std::make_unique<Recorder>();
    const bool ok = OnMta([&] { return rec->Start(s, e); });
    if (!ok) {
        Out(e, err, err_len);
        return -1;
    }
    g_rec = std::move(rec);
    return 0;
}

NXCAP_API int32_t nxcap_status(char* buf, int32_t buf_len) {
    std::scoped_lock lk{g_mu};
    Status st;
    if (g_rec) st = g_rec->GetStatus();
    std::ostringstream j;
    j << "{\"state\":\"" << st.state << "\",\"seconds\":" << st.seconds << ",\"frames\":" << st.frames << ",\"dropped\":" << st.dropped
      << ",\"width\":" << st.width << ",\"height\":" << st.height << ",\"encoder\":\"" << Esc(st.encoder) << "\",\"gpu\":" << (st.gpu ? "true" : "false")
      << ",\"audio\":\"" << st.audio << "\",\"mic\":" << (st.mic ? "true" : "false") << ",\"path\":\"" << Esc(st.path) << "\",\"error\":\"" << Esc(st.error) << "\"}";
    return Out(j.str(), buf, buf_len);
}

NXCAP_API int32_t nxcap_stop(char* err, int32_t err_len) {
    std::scoped_lock lk{g_mu};
    if (!g_rec) return 0;
    std::string e;
    const bool ok = OnMta([&] { return g_rec->Stop(e); });
    if (!ok) {
        Out(e, err, err_len);
        return -1;
    }
    return 0;
}

NXCAP_API int32_t nxcap_selftest(const char* path, int32_t seconds, char* buf, int32_t buf_len) {
    EnsureMF();
    const std::wstring p = Widen(path ? path : "");
    return Out(OnMta([&] { return SelfTest(p, seconds); }), buf, buf_len);
}

} // extern "C"
