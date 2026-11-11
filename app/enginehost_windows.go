//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// dllEngine calls nexus_engine.dll through its C interface. The DLL (and the libraries it needs)
// is loaded once into the NEXUS process from the active engine build folder.
type dllEngine struct {
	dll                                                              *syscall.DLL
	apiVersion, info, start, status, stop, setVolume, focus, suspend *syscall.Proc
}

func loadEngineHost(dir string) (EngineHost, error) {
	if fakeEngineEnabled() {
		return newFakeEngine(), nil
	}
	path := filepath.Join(dir, "nexus_engine.dll")
	d, err := loadEngineDLL(dir, path)
	if err != nil {
		return nil, explainLoadError(dir, path, err)
	}
	e := &dllEngine{dll: d}
	for name, dst := range map[string]**syscall.Proc{
		"nexus_engine_api_version": &e.apiVersion, "nexus_engine_info": &e.info,
		"nexus_engine_start": &e.start, "nexus_engine_status": &e.status, "nexus_engine_stop": &e.stop,
		"nexus_engine_set_volume": &e.setVolume, "nexus_engine_focus": &e.focus, "nexus_engine_suspend": &e.suspend,
	} {
		p, err := d.FindProc(name)
		if err != nil {
			return nil, fmt.Errorf("the engine build is incomplete (%s missing)", name)
		}
		*dst = p
	}
	if v, _, _ := e.apiVersion.Call(); int32(v) != engineAPIVersion {
		return nil, fmt.Errorf("this engine build uses interface version %d; NEXUS needs %d", int32(v), engineAPIVersion)
	}
	return e, nil
}

func cstr(p uintptr) string {
	if p == 0 {
		return ""
	}
	var b []byte
	for i := uintptr(0); i < 1<<20; i++ {
		c := *(*byte)(unsafe.Pointer(p + i))
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return string(b)
}

func (e *dllEngine) Info() (EngineBuildInfo, error) {
	p, _, _ := e.info.Call()
	return parseEngineInfo(cstr(p))
}

func (e *dllEngine) Start(req EngineRequest) error {
	b, _ := json.Marshal(req)
	b = append(b, 0)
	r, _, _ := e.start.Call(uintptr(unsafe.Pointer(&b[0])))
	switch int32(r) {
	case 0:
		return nil
	case -1:
		return errors.New("a game is already running")
	default:
		return errors.New("the engine refused the launch request")
	}
}

func (e *dllEngine) Status() EngineStatus {
	buf := make([]byte, 4096)
	_, _, _ = e.status.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	var st EngineStatus
	_ = json.Unmarshal([]byte(cstr(uintptr(unsafe.Pointer(&buf[0])))), &st)
	return st
}

func (e *dllEngine) Stop(force bool) {
	f := uintptr(0)
	if force {
		f = 1
	}
	_, _, _ = e.stop.Call(f)
}
func (e *dllEngine) SetVolume(p int) { _, _, _ = e.setVolume.Call(uintptr(int32(p))) }
func (e *dllEngine) Focus()          { _, _, _ = e.focus.Call() }
func (e *dllEngine) Suspend(on bool) {
	v := uintptr(0)
	if on {
		v = 1
	}
	_, _, _ = e.suspend.Call(v)
}

// fakeEngineEnabled: the stand-in engine exists in Windows builds only for NEXUS's own test
// suite (allowFakeEngine is set by the tests); a normal NEXUS.exe always uses nexus_engine.dll.
func fakeEngineEnabled() bool { return allowFakeEngine && os.Getenv("NEXUS_FAKE_ENGINE") == "1" }

// loadEngineDLL loads nexus_engine.dll from the engine folder. Its dependencies are searched in
// the engine folder itself, in Engine\Libraries and in System32 only (never the current
// directory or PATH), so the NEXUS folder works wherever it was extracted.
func loadEngineDLL(dir, path string) (*syscall.DLL, error) {
	k32 := syscall.NewLazyDLL("kernel32.dll")
	addDir := k32.NewProc("AddDllDirectory")
	loadEx := k32.NewProc("LoadLibraryExW")
	for _, sub := range []string{dir, filepath.Join(dir, "Libraries"), filepath.Join(dir, "Dependencies")} {
		if p, err := syscall.UTF16PtrFromString(sub); err == nil {
			_, _, _ = addDir.Call(uintptr(unsafe.Pointer(p)))
		}
	}
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	const (
		searchDLLLoadDir = 0x00000100 // LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR
		searchSystem32   = 0x00000800 // LOAD_LIBRARY_SEARCH_SYSTEM32
		searchUserDirs   = 0x00000400 // LOAD_LIBRARY_SEARCH_USER_DIRS
	)
	h, _, callErr := loadEx.Call(uintptr(unsafe.Pointer(p)), 0, searchDLLLoadDir|searchSystem32|searchUserDirs)
	if h == 0 {
		return nil, callErr
	}
	return &syscall.DLL{Name: path, Handle: syscall.Handle(h)}, nil
}

// explainLoadError turns Windows loader errors into something a person can act on.
func explainLoadError(dir, path string, err error) error {
	var en syscall.Errno
	if errors.As(err, &en) {
		switch en {
		case 126: // ERROR_MOD_NOT_FOUND: a dependency is missing
			missing := []string{}
			if !vcRuntimeAvailable(dir) {
				missing = append(missing, "the Microsoft Visual C++ 2015-2022 x64 runtime (https://aka.ms/vs/17/release/vc_redist.x64.exe)")
			}
			if !vulkanLoaderPresent() {
				missing = append(missing, "a Vulkan-capable graphics driver (install the latest driver for your GPU)")
			}
			if len(missing) > 0 {
				return fmt.Errorf("the NEXUS engine needs %s", strings.Join(missing, " and "))
			}
			return fmt.Errorf("a file the NEXUS engine needs is missing from the Engine folder. Extract the complete NEXUS ZIP again (%s)", path)
		case 193: // ERROR_BAD_EXE_FORMAT
			return fmt.Errorf("%s is not a 64-bit Windows engine build", path)
		}
	}
	return fmt.Errorf("the NEXUS engine could not be loaded from %s: %w", path, err)
}
