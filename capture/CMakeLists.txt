# SPDX-FileCopyrightText: 2026 NEXUS project
# SPDX-License-Identifier: GPL-3.0-or-later
cmake_minimum_required(VERSION 3.22)
project(nexus_capture CXX)
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>") # static CRT: no extra DLLs

add_library(nexus_capture SHARED src/nexus_capture.cpp src/recorder.cpp)
target_include_directories(nexus_capture PUBLIC include PRIVATE src)
target_compile_definitions(nexus_capture PRIVATE NOMINMAX UNICODE _UNICODE)
target_compile_options(nexus_capture PRIVATE /W4 /permissive- /EHsc /utf-8 /bigobj)
target_link_libraries(nexus_capture PRIVATE d3d11 dxgi dwmapi mfplat mfreadwrite mfuuid mf windowsapp mmdevapi ole32 ksuser)

add_executable(nexus_capture_test test/capture_test.cpp)
target_compile_definitions(nexus_capture_test PRIVATE UNICODE _UNICODE)
