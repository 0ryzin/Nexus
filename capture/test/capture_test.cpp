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
#include <cstdint>

using ApiVersion = int (*)();
using Caps = int (*)(char*, int);
using SelfTest = int (*)(const char*, int, char*, int);
using Start = int (*)(const char*, char*, int);
using Stop = int (*)(char*, int);
using StatusFn = int (*)(char*, int);

static int g_tick = 0;
static LRESULT CALLBACK WndProc(HWND h, UINT m, WPARAM w, LPARAM l) {
    if (m == WM_PAINT) {
        PAINTSTRUCT ps;
        HDC dc = BeginPaint(h, &ps);
        RECT rc;
        GetClientRect(h, &rc);
        HBRUSH b = CreateSolidBrush(RGB((g_tick * 7) & 255, 80, 200 - (g_tick & 127)));
        FillRect(dc, &rc, b);
        DeleteObject(b);
        EndPaint(h, &ps);
        return 0;
    }
    return DefWindowProcW(h, m, w, l);
}

// Records a real, animated window for ~3 s through Windows Graphics Capture.
static bool WindowTest(Start start, Stop stop, StatusFn status, const std::string& out) {
    WNDCLASSW wc{};
    wc.lpfnWndProc = WndProc;
    wc.hInstance = GetModuleHandleW(nullptr);
    wc.lpszClassName = L"NexusCaptureTest";
    RegisterClassW(&wc);
    HWND hwnd = CreateWindowExW(0, wc.lpszClassName, L"NEXUS capture test", WS_OVERLAPPEDWINDOW, 100, 100, 960, 540, nullptr, nullptr, wc.hInstance, nullptr);
    ShowWindow(hwnd, SW_SHOW);
    UpdateWindow(hwnd);
    char err[1024] = {};
    std::string settings = "hwnd=" + std::to_string(reinterpret_cast<intptr_t>(hwnd)) + "\npath=" + out + "\nfps=30\nbitrate=4000\naudio=1\nmic=0\n";
    bool started = false;
    const DWORD t0 = GetTickCount();
    MSG msg;
    while (GetTickCount() - t0 < 4000) {
        while (PeekMessageW(&msg, nullptr, 0, 0, PM_REMOVE)) {
            TranslateMessage(&msg);
            DispatchMessageW(&msg);
        }
        ++g_tick;
        InvalidateRect(hwnd, nullptr, FALSE);
        if (!started && GetTickCount() - t0 > 500) {
            if (start(settings.c_str(), err, sizeof(err)) != 0) {
                std::printf("window test: start failed: %s\n", err);
                DestroyWindow(hwnd);
                return false;
            }
            started = true;
        }
        Sleep(15);
    }
    static char st[4096];
    status(st, sizeof(st));
    std::printf("window test status: %s\n", st);
    const int r = stop(err, sizeof(err));
    DestroyWindow(hwnd);
    if (r != 0) {
        std::printf("window test: stop failed: %s\n", err);
        return false;
    }
    WIN32_FILE_ATTRIBUTE_DATA fa{};
    if (!GetFileAttributesExA(out.c_str(), GetFileExInfoStandard, &fa)) {
        std::printf("window test: no file\n");
        return false;
    }
    std::printf("window test: %s, %lu bytes\n", out.c_str(), fa.nFileSizeLow);
    return fa.nFileSizeLow > 10000 && std::strstr(st, "\"frames\":0") == nullptr;
}

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
    if (!std::strstr(buf, "\"ok\":true")) return 1;
    if (argc > 2) {
        auto start = reinterpret_cast<Start>(GetProcAddress(m, "nxcap_start"));
        auto stop = reinterpret_cast<Stop>(GetProcAddress(m, "nxcap_stop"));
        auto status = reinterpret_cast<StatusFn>(GetProcAddress(m, "nxcap_status"));
        if (!WindowTest(start, stop, status, argv[2])) return 3;
    }
    return 0;
}
