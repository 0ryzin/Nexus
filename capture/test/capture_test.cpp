// SPDX-FileCopyrightText: 2026 NEXUS project
// SPDX-License-Identifier: GPL-3.0-or-later
//
// Build check of the capture module: loads nexus_capture.dll like NEXUS does, prints the encoders
// it finds and runs the self-test (generated video + audio -> MP4 -> read back). Exit code 0 only
// if the self-test file is complete.

#include <windows.h>
#include <cstdio>
#include <cstring>
#include <string>

using ApiVersion = int (*)();
using Caps = int (*)(char*, int);
using SelfTest = int (*)(const char*, int, char*, int);

int main(int argc, char** argv) {
    const char* out = argc > 1 ? argv[1] : "capture-selftest.mp4";
    HMODULE m = LoadLibraryW(L"nexus_capture.dll");
    if (!m) {
        std::printf("{\"ok\":false,\"error\":\"nexus_capture.dll did not load (%lu)\"}\n", GetLastError());
        return 2;
    }
    auto ver = reinterpret_cast<ApiVersion>(GetProcAddress(m, "nxcap_api_version"));
    auto caps = reinterpret_cast<Caps>(GetProcAddress(m, "nxcap_capabilities"));
    auto test = reinterpret_cast<SelfTest>(GetProcAddress(m, "nxcap_selftest"));
    if (!ver || !caps || !test || !GetProcAddress(m, "nxcap_start") || !GetProcAddress(m, "nxcap_stop") || !GetProcAddress(m, "nxcap_status")) {
        std::printf("{\"ok\":false,\"error\":\"missing exports\"}\n");
        return 2;
    }
    static char buf[1 << 16];
    caps(buf, sizeof(buf));
    std::printf("api %d\ncapabilities %s\n", ver(), buf);
    test(out, 4, buf, sizeof(buf));
    std::printf("selftest %s\n", buf);
    return std::strstr(buf, "\"ok\":true") ? 0 : 1;
}
