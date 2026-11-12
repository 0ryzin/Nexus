//go:build (linux || android) && cgo

package main

// The NEXUS engine host for platforms that load shared libraries with dlopen (Android; also
// used by the test suite on Linux). It speaks the same C interface (nexus_engine.h, ABI v2) as
// the Windows host in enginehost_windows.go, so sessions, settings, play time and errors are
// handled by the same shared Go code on every platform.

/*
#cgo LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdint.h>
#include <stdlib.h>

typedef int32_t (*nx_i32_void)(void);
typedef const char* (*nx_str_void)(void);
typedef int32_t (*nx_i32_str)(const char*);
typedef int32_t (*nx_i32_buf)(char*, int32_t);
typedef int32_t (*nx_i32_i32)(int32_t);
typedef void (*nx_void_i32)(int32_t);
typedef void (*nx_void_void)(void);
typedef void* (*nx_ptr_void)(void);
typedef int32_t (*nx_i32_ptr)(void*);

static void* nx_open(const char* path, char** err) {
	void* h = dlopen(path, RTLD_NOW | RTLD_LOCAL);
	if (!h) *err = (char*)dlerror();
	return h;
}
static void* nx_sym(void* h, const char* name) { return dlsym(h, name); }
static int32_t nx_call_i32_void(void* f) { return ((nx_i32_void)f)(); }
static const char* nx_call_str_void(void* f) { return ((nx_str_void)f)(); }
static int32_t nx_call_i32_str(void* f, const char* s) { return ((nx_i32_str)f)(s); }
static int32_t nx_call_i32_buf(void* f, char* b, int32_t n) { return ((nx_i32_buf)f)(b, n); }
static int32_t nx_call_i32_i32(void* f, int32_t v) { return ((nx_i32_i32)f)(v); }
static void nx_call_void_i32(void* f, int32_t v) { ((nx_void_i32)f)(v); }
static void nx_call_void_void(void* f) { ((nx_void_void)f)(); }
static void* nx_call_ptr_void(void* f) { return ((nx_ptr_void)f)(); }
static int32_t nx_call_i32_ptr(void* f, void* p) { return ((nx_i32_ptr)f)(p); }
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"unsafe"
)

// dlEngine calls a NEXUS engine shared library (libnexus_engine.so).
type dlEngine struct {
	handle                                                           unsafe.Pointer
	apiVersion, info, start, status, stop, setVolume, focus, suspend unsafe.Pointer
	reveal, controllers                                              unsafe.Pointer // ABI 0.4 (optional)
	background, screenshot, window                                   unsafe.Pointer // ABI 0.5 (optional)
	// mobile additions (optional, see docs/ANDROID.md): the surface to draw into, touch
	// controls, app lifecycle and memory pressure
	setSurface, touch, lifecycle, trimMemory unsafe.Pointer
}

// openDLEngine loads the engine library at path and checks its interface version.
func openDLEngine(path string) (*dlEngine, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var cerr *C.char
	h := C.nx_open(cpath, &cerr)
	if h == nil {
		msg := "unknown error"
		if cerr != nil {
			msg = C.GoString(cerr)
		}
		return nil, fmt.Errorf("the NEXUS engine could not be loaded from %s: %s", path, msg)
	}
	e := &dlEngine{handle: h}
	sym := func(name string) unsafe.Pointer {
		cn := C.CString(name)
		defer C.free(unsafe.Pointer(cn))
		return C.nx_sym(h, cn)
	}
	for name, dst := range map[string]*unsafe.Pointer{
		"nexus_engine_api_version": &e.apiVersion, "nexus_engine_info": &e.info,
		"nexus_engine_start": &e.start, "nexus_engine_status": &e.status, "nexus_engine_stop": &e.stop,
		"nexus_engine_set_volume": &e.setVolume, "nexus_engine_focus": &e.focus, "nexus_engine_suspend": &e.suspend,
	} {
		if *dst = sym(name); *dst == nil {
			return nil, fmt.Errorf("the engine build is incomplete (%s missing)", name)
		}
	}
	e.reveal, e.controllers = sym("nexus_engine_reveal"), sym("nexus_engine_controllers")
	e.background, e.screenshot, e.window = sym("nexus_engine_background"), sym("nexus_engine_screenshot"), sym("nexus_engine_window")
	e.setSurface, e.touch = sym("nexus_engine_set_surface"), sym("nexus_engine_touch")
	e.lifecycle, e.trimMemory = sym("nexus_engine_lifecycle"), sym("nexus_engine_trim_memory")
	if v := int32(C.nx_call_i32_void(e.apiVersion)); v != engineAPIVersion {
		return nil, fmt.Errorf("this engine build uses interface version %d; NEXUS needs %d", v, engineAPIVersion)
	}
	return e, nil
}

func (e *dlEngine) Info() (EngineBuildInfo, error) {
	return parseEngineInfo(C.GoString(C.nx_call_str_void(e.info)))
}

func (e *dlEngine) Start(req EngineRequest) error {
	b, _ := json.Marshal(req)
	cs := C.CString(string(b))
	defer C.free(unsafe.Pointer(cs))
	switch int32(C.nx_call_i32_str(e.start, cs)) {
	case 0:
		return nil
	case -1:
		return errors.New("a game is already running")
	default:
		return errors.New("the engine refused the launch request")
	}
}

// callJSON calls an export that fills a buffer with JSON.
func (e *dlEngine) callJSON(f unsafe.Pointer, size int) (string, int32) {
	buf := C.malloc(C.size_t(size))
	defer C.free(buf)
	*(*byte)(buf) = 0
	r := int32(C.nx_call_i32_buf(f, (*C.char)(buf), C.int32_t(size)))
	return C.GoString((*C.char)(buf)), r
}

func (e *dlEngine) Status() EngineStatus {
	s, _ := e.callJSON(e.status, 4096)
	var st EngineStatus
	_ = json.Unmarshal([]byte(s), &st)
	return st
}

func cbool(b bool) C.int32_t {
	if b {
		return 1
	}
	return 0
}

func (e *dlEngine) Stop(force bool) { C.nx_call_i32_i32(e.stop, cbool(force)) }
func (e *dlEngine) SetVolume(p int) { C.nx_call_void_i32(e.setVolume, C.int32_t(p)) }
func (e *dlEngine) Focus()          { C.nx_call_void_void(e.focus) }
func (e *dlEngine) Suspend(on bool) { C.nx_call_void_i32(e.suspend, cbool(on)) }

func (e *dlEngine) Reveal() {
	if e.reveal != nil {
		C.nx_call_void_void(e.reveal)
	}
}

func (e *dlEngine) Background(on bool) bool {
	if e.background == nil {
		return false
	}
	C.nx_call_void_i32(e.background, cbool(on))
	return true
}

func (e *dlEngine) Screenshot() bool {
	if e.screenshot == nil {
		return false
	}
	C.nx_call_void_void(e.screenshot)
	return true
}

func (e *dlEngine) Window() uintptr {
	if e.window == nil {
		return 0
	}
	return uintptr(C.nx_call_ptr_void(e.window))
}

func (e *dlEngine) Controllers() ([]PadInfo, error) {
	if e.controllers == nil {
		return nil, errors.New("this engine build cannot list controllers")
	}
	s, r := e.callJSON(e.controllers, 64<<10)
	if r < 0 {
		return nil, errors.New("the engine could not list controllers")
	}
	var out []PadInfo
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ---- mobile additions (MobileEngine, platform_mobile.go)

func (e *dlEngine) SetSurface(window unsafe.Pointer) bool {
	if e.setSurface == nil {
		return false
	}
	return int32(C.nx_call_i32_ptr(e.setSurface, window)) == 0
}

func (e *dlEngine) Touch(state TouchState) bool {
	if e.touch == nil {
		return false
	}
	b, _ := json.Marshal(state)
	cs := C.CString(string(b))
	defer C.free(unsafe.Pointer(cs))
	return int32(C.nx_call_i32_str(e.touch, cs)) == 0
}

func (e *dlEngine) Lifecycle(ev LifecycleEvent) bool {
	if e.lifecycle == nil {
		return false
	}
	C.nx_call_void_i32(e.lifecycle, C.int32_t(ev))
	return true
}

func (e *dlEngine) TrimMemory(level int) bool {
	if e.trimMemory == nil {
		return false
	}
	C.nx_call_void_i32(e.trimMemory, C.int32_t(level))
	return true
}
