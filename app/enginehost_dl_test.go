//go:build linux && cgo

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

// A minimal engine library implementing the NEXUS C interface (ABI v2 plus the mobile
// additions), built at test time. It checks that the dlopen host (used by NEXUS on Android)
// calls every export correctly: the same contract the real engine implements.
const testEngineC = `
#include <stdint.h>
#include <stdio.h>
#include <string.h>
static char last_request[4096];
static char last_touch[1024];
static int32_t volume = 100, suspended = 0, background = 0, lifecycle = 0, trim = 0, running = 0;
static void* surface = 0;
int32_t nexus_engine_api_version(void) { return 2; }
const char* nexus_engine_info(void) { return "{\"version\":\"9.9.9-test\",\"abi\":2}"; }
int32_t nexus_engine_start(const char* req) {
	if (running) return -1;
	strncpy(last_request, req, sizeof last_request - 1); running = 1; return 0;
}
int32_t nexus_engine_status(char* buf, int32_t n) {
	return snprintf(buf, n, "{\"state\":\"%s\",\"frames\":42,\"suspended\":%s,\"volume\":%d,\"background\":%s}",
		running ? "running" : "idle", suspended ? "true" : "false", volume, background ? "true" : "false");
}
int32_t nexus_engine_stop(int32_t force) { running = 0; return 0; }
void nexus_engine_set_volume(int32_t p) { volume = p; }
void nexus_engine_focus(void) {}
void nexus_engine_suspend(int32_t s) { suspended = s; }
void nexus_engine_background(int32_t on) { background = on; }
int32_t nexus_engine_controllers(char* buf, int32_t n) { return snprintf(buf, n, "[{\"name\":\"Test Pad\"}]"); }
int32_t nexus_engine_set_surface(void* w) { surface = w; return 0; }
int32_t nexus_engine_touch(const char* json) { strncpy(last_touch, json, sizeof last_touch - 1); return 0; }
void nexus_engine_lifecycle(int32_t ev) { lifecycle = ev; }
void nexus_engine_trim_memory(int32_t level) { trim = level; }
const char* test_last_request(void) { return last_request; }
const char* test_last_touch(void) { return last_touch; }
int32_t test_lifecycle(void) { return lifecycle; }
int32_t test_trim(void) { return trim; }
void* test_surface(void) { return surface; }
`

func buildTestEngine(t *testing.T) string {
	t.Helper()
	cc, err := exec.LookPath("cc")
	if err != nil {
		if cc, err = exec.LookPath("clang"); err != nil {
			t.Skip("no C compiler")
		}
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "engine.c")
	os.WriteFile(src, []byte(testEngineC), 0o644)
	lib := filepath.Join(dir, "libnexus_engine.so")
	if out, err := exec.Command(cc, "-shared", "-fPIC", "-o", lib, src).CombinedOutput(); err != nil {
		t.Fatalf("building the test engine: %v\n%s", err, out)
	}
	return lib
}

func TestDLEngineHost(t *testing.T) {
	e, err := openDLEngine(buildTestEngine(t))
	if err != nil {
		t.Fatal(err)
	}
	var _ EngineHost = e   // the same interface the Windows host implements
	var _ MobileEngine = e // and the mobile additions
	info, err := e.Info()
	if err != nil || info.Version != "9.9.9-test" {
		t.Fatalf("info %+v %v", info, err)
	}
	if err := e.Start(EngineRequest{Game: "/games/a.nsp", Title: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := e.Start(EngineRequest{Game: "/games/b.nsp"}); err == nil {
		t.Fatal("second start accepted")
	}
	get := func(name string) unsafe.Pointer { return dlSymForTest(e, name) }
	if req := callStrForTest(get("test_last_request")); !strings.Contains(req, `"/games/a.nsp"`) {
		t.Fatalf("request %q", req)
	}
	e.SetVolume(150)
	e.Suspend(true)
	if !e.Background(true) {
		t.Fatal("background export not found")
	}
	st := e.Status()
	if st.State != "running" || st.Frames != 42 || !st.Suspended || st.Volume != 150 || !st.Background {
		t.Fatalf("status %+v", st)
	}
	pads, err := e.Controllers()
	if err != nil || len(pads) != 1 || pads[0].Name != "Test Pad" {
		t.Fatalf("controllers %+v %v", pads, err)
	}
	if !e.Touch(TouchState{Buttons: []string{"A", "ZR"}, LStick: [2]float32{0.5, -1}}) ||
		!strings.Contains(callStrForTest(get("test_last_touch")), `"buttons":["A","ZR"]`) {
		t.Fatal("touch")
	}
	if !e.Lifecycle(LifecycleStop) || callI32ForTest(get("test_lifecycle")) != int32(LifecycleStop) {
		t.Fatal("lifecycle")
	}
	if !e.TrimMemory(80) || callI32ForTest(get("test_trim")) != 80 {
		t.Fatal("trim")
	}
	marker := new(byte)
	if !e.SetSurface(unsafe.Pointer(marker)) || callPtrForTest(get("test_surface")) != unsafe.Pointer(marker) {
		t.Fatal("surface")
	}
	e.SetSurface(nil)
	e.Stop(false)
	if e.Status().State != "idle" {
		t.Fatal("stop")
	}
}

func TestDLEngineHostRejectsWrongLibraries(t *testing.T) {
	if _, err := openDLEngine("/nonexistent/libnexus_engine.so"); err == nil {
		t.Fatal("missing library accepted")
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no C compiler")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "old.c")
	os.WriteFile(src, []byte(`#include <stdint.h>
int32_t nexus_engine_api_version(void) { return 1; }`), 0o644)
	lib := filepath.Join(dir, "libold.so")
	if out, err := exec.Command(cc, "-shared", "-fPIC", "-o", lib, src).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if _, err := openDLEngine(lib); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("incomplete library accepted: %v", err)
	}
}
