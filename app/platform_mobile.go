package main

// Shared mobile platform logic (Android today). Everything here is plain Go with no platform
// build tags, so it is compiled and tested on every platform; the Android app only reports what
// it observes (device facts, lifecycle, thermal state, memory pressure) through
// platform_android.go and shows the same NEXUS interface.

import (
	"fmt"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
)

// TouchState is the on-screen controller's state, sent to the engine (nexus_engine_touch).
// Buttons use the Switch names; sticks are -1…1.
type TouchState struct {
	Buttons []string   `json:"buttons"`
	LStick  [2]float32 `json:"lstick"`
	RStick  [2]float32 `json:"rstick"`
}

// LifecycleEvent mirrors the Android activity lifecycle that matters to a game session.
type LifecycleEvent int32

const (
	LifecycleResume LifecycleEvent = 1 // the app is in front again
	LifecyclePause  LifecycleEvent = 2 // another app or the notification shade is in front
	LifecycleStop   LifecycleEvent = 3 // the app is no longer visible (its surface is gone)
)

// MobileEngine is implemented by engine hosts that support the mobile additions to the engine
// interface (all optional exports; see docs/ANDROID.md).
type MobileEngine interface {
	Touch(TouchState) bool
	Lifecycle(LifecycleEvent) bool
	TrimMemory(level int) bool
}

// ---------- device profile and readiness

// DeviceProfile is what the Android app reports about the device (DeviceInfo.kt).
type DeviceProfile struct {
	OS              string   `json:"os"`
	APILevel        int      `json:"apiLevel"`
	Release         string   `json:"release"`
	Manufacturer    string   `json:"manufacturer"`
	Model           string   `json:"model"`
	SoCManufacturer string   `json:"socManufacturer"`
	SoCModel        string   `json:"socModel"`
	GPURenderer     string   `json:"gpuRenderer"`   // GL_RENDERER, e.g. "Adreno (TM) 750", "Mali-G715"
	VulkanVersion   string   `json:"vulkanVersion"` // "1.3", "" if the device has no Vulkan
	ABIs            []string `json:"abis"`
	TotalRAMMB      int      `json:"totalRamMb"`
	FreeStorageMB   int      `json:"freeStorageMb"`
	AllFilesAccess  bool     `json:"allFilesAccess"`
	ThermalStatus   int      `json:"thermalStatus"` // PowerManager.THERMAL_STATUS_* (0 none … 6 shutdown)
	EngineLibrary   bool     `json:"engineLibrary"` // libnexus_engine.so is packaged
}

// GPUClass is NEXUS's assessment of a mobile GPU family.
type GPUClass struct {
	Family  string `json:"family"` // Adreno, Mali, Immortalis, Xclipse, PowerVR, Other
	Number  int    `json:"number"` // model number where there is one (750, 715 …)
	Level   string `json:"level"`  // recommended | supported | limited | unsupported | unknown
	Comment string `json:"comment"`
}

var (
	reAdreno = regexp.MustCompile(`(?i)adreno[^0-9]*([0-9]{3})`)
	reMali   = regexp.MustCompile(`(?i)mali-?g([0-9]{2,3})`)
	reImmort = regexp.MustCompile(`(?i)immortalis-?g([0-9]{3})`)
	reXclip  = regexp.MustCompile(`(?i)xclipse\s*([0-9]{3})`)
)

// ClassifyGPU places a GPU renderer string in one of NEXUS's support levels. The levels follow
// what is known about the Vulkan drivers of each family for this kind of workload: recent Adreno
// drivers (and the open-source Turnip driver) are the most complete; Mali and Immortalis
// (Valhall and newer) work but have more driver-specific rendering problems; older
// architectures lack features the engine's Vulkan renderer needs.
func ClassifyGPU(renderer string) GPUClass {
	switch {
	case reAdreno.MatchString(renderer):
		n, _ := strconv.Atoi(reAdreno.FindStringSubmatch(renderer)[1])
		g := GPUClass{Family: "Adreno", Number: n}
		switch {
		case n >= 730:
			g.Level, g.Comment = "recommended", "Snapdragon 8 Gen 2 class or newer."
		case n >= 640:
			g.Level, g.Comment = "supported", "Older high-end Adreno: lighter games only."
		case n >= 610:
			g.Level, g.Comment = "limited", "Mid-range Adreno: expect low frame rates."
		default:
			g.Level, g.Comment = "unsupported", "This Adreno generation lacks Vulkan features the engine needs."
		}
		return g
	case reImmort.MatchString(renderer):
		n, _ := strconv.Atoi(reImmort.FindStringSubmatch(renderer)[1])
		return GPUClass{Family: "Immortalis", Number: n, Level: "supported", Comment: "Arm Immortalis: works, with more driver-specific graphics problems than Adreno."}
	case reMali.MatchString(renderer):
		n, _ := strconv.Atoi(reMali.FindStringSubmatch(renderer)[1])
		g := GPUClass{Family: "Mali", Number: n}
		switch {
		case n >= 710: // Valhall 3rd gen and newer (G710, G715, G720 …)
			g.Level, g.Comment = "supported", "Recent Mali: works, with more driver-specific graphics problems than Adreno."
		case n >= 77 && n < 100: // G77, G78 (Valhall)
			g.Level, g.Comment = "limited", "Older Valhall Mali: expect graphics problems and low frame rates."
		default:
			g.Level, g.Comment = "unsupported", "This Mali generation lacks Vulkan features the engine needs."
		}
		return g
	case reXclip.MatchString(renderer):
		n, _ := strconv.Atoi(reXclip.FindStringSubmatch(renderer)[1])
		return GPUClass{Family: "Xclipse", Number: n, Level: "unknown", Comment: "Samsung Xclipse (AMD RDNA): not tested with NEXUS yet."}
	case strings.Contains(strings.ToLower(renderer), "powervr"):
		return GPUClass{Family: "PowerVR", Level: "unknown", Comment: "PowerVR: not tested with NEXUS yet."}
	case renderer == "":
		return GPUClass{Family: "Other", Level: "unknown", Comment: "The graphics processor could not be identified."}
	}
	return GPUClass{Family: "Other", Level: "unknown", Comment: "Not tested with NEXUS yet."}
}

// ReadinessItem is one line of the readiness check.
type ReadinessItem struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	State  string `json:"state"` // ok | warn | fail
	Value  string `json:"value"`
	Detail string `json:"detail,omitempty"`
}

// Readiness is the result of the device readiness check.
type Readiness struct {
	Ready bool            `json:"ready"` // no item failed
	GPU   GPUClass        `json:"gpu"`
	Items []ReadinessItem `json:"items"`
}

// Android requirements. Minimum: below it NEXUS does not run games. Recommended: what the
// targeted Switch games need to run well.
const (
	androidMinAPI         = 33 // Android 13
	androidMinRAMMB       = 8 * 1024
	androidRecRAMMB       = 12 * 1024
	androidDemandingRAMMB = 16 * 1024
	androidMinStorageMB   = 4 * 1024
)

func vulkanAtLeast(v string, major, minor int) bool {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return false
	}
	a, err1 := strconv.Atoi(parts[0])
	b, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	return a > major || a == major && b >= minor
}

// CheckReadiness evaluates a device against NEXUS's Android requirements.
func CheckReadiness(d DeviceProfile) Readiness {
	r := Readiness{GPU: ClassifyGPU(d.GPURenderer)}
	add := func(id, label, state, value, detail string) {
		r.Items = append(r.Items, ReadinessItem{id, label, state, value, detail})
	}
	// processor architecture
	arm64 := false
	for _, a := range d.ABIs {
		if a == "arm64-v8a" {
			arm64 = true
		}
	}
	if arm64 {
		add("abi", "Processor", "ok", "64-bit ARM (arm64-v8a)", "")
	} else {
		add("abi", "Processor", "fail", strings.Join(d.ABIs, ", "), "NEXUS needs a 64-bit ARM processor.")
	}
	// Android version
	ver := fmt.Sprintf("Android %s (API %d)", firstNonEmpty(d.Release, "?"), d.APILevel)
	if d.APILevel >= androidMinAPI {
		add("os", "Android version", "ok", ver, "")
	} else {
		add("os", "Android version", "fail", ver, "NEXUS needs Android 13 or newer.")
	}
	// Vulkan
	switch {
	case d.VulkanVersion == "":
		add("vulkan", "Vulkan", "fail", "Not available", "NEXUS draws games with Vulkan; this device's graphics driver does not offer it.")
	case !vulkanAtLeast(d.VulkanVersion, 1, 1):
		add("vulkan", "Vulkan", "fail", d.VulkanVersion, "NEXUS needs Vulkan 1.1 or newer.")
	case !vulkanAtLeast(d.VulkanVersion, 1, 3):
		add("vulkan", "Vulkan", "warn", d.VulkanVersion, "Vulkan 1.3 is recommended; some games may not display correctly.")
	default:
		add("vulkan", "Vulkan", "ok", d.VulkanVersion, "")
	}
	// GPU family
	gpu := strings.TrimSpace(firstNonEmpty(d.GPURenderer, "Unknown"))
	switch r.GPU.Level {
	case "recommended", "supported":
		st := "ok"
		if r.GPU.Level == "supported" && r.GPU.Family != "Adreno" {
			st = "warn"
		}
		add("gpu", "Graphics", st, gpu, r.GPU.Comment)
	case "limited", "unknown":
		add("gpu", "Graphics", "warn", gpu, r.GPU.Comment)
	default:
		add("gpu", "Graphics", "fail", gpu, r.GPU.Comment)
	}
	// memory
	ram := fmt.Sprintf("%.1f GB", float64(d.TotalRAMMB)/1024)
	switch {
	case d.TotalRAMMB < androidMinRAMMB:
		add("ram", "Memory", "fail", ram, "NEXUS needs at least 8 GB of RAM to run games.")
	case d.TotalRAMMB < androidRecRAMMB:
		add("ram", "Memory", "warn", ram, "12 GB or more is recommended; larger games may close when memory runs out.")
	case d.TotalRAMMB < androidDemandingRAMMB:
		add("ram", "Memory", "ok", ram, "16 GB is recommended for the most demanding games.")
	default:
		add("ram", "Memory", "ok", ram, "")
	}
	// storage
	free := fmt.Sprintf("%.1f GB free", float64(d.FreeStorageMB)/1024)
	if d.FreeStorageMB < androidMinStorageMB {
		add("storage", "Storage", "warn", free, "Shader caches and saves need space; free up at least 4 GB.")
	} else {
		add("storage", "Storage", "ok", free, "")
	}
	// file access for game folders
	if d.AllFilesAccess {
		add("files", "Access to game folders", "ok", "Allowed", "")
	} else {
		add("files", "Access to game folders", "warn", "Not allowed", "Allow NEXUS to access your files so it can read games from your folders.")
	}
	// temperature
	if d.ThermalStatus >= 3 { // THERMAL_STATUS_SEVERE
		add("thermal", "Temperature", "warn", thermalName(d.ThermalStatus), "The device is too hot; it will slow games down until it cools.")
	} else {
		add("thermal", "Temperature", "ok", thermalName(d.ThermalStatus), "")
	}
	// the engine itself
	if d.EngineLibrary {
		add("engine", "NEXUS Engine", "ok", "Installed", "")
	} else {
		add("engine", "NEXUS Engine", "fail", "Not included", "This development build of NEXUS for Android does not include the NEXUS Engine yet, so games cannot be started.")
	}
	r.Ready = true
	for _, it := range r.Items {
		if it.State == "fail" {
			r.Ready = false
		}
	}
	return r
}

func thermalName(s int) string {
	return map[int]string{0: "Normal", 1: "Light", 2: "Moderate", 3: "Severe", 4: "Critical", 5: "Emergency", 6: "Shutdown"}[s]
}

// ---------- platform state (filled by the Android app; empty on Windows)

type mobileState struct {
	mu      sync.Mutex
	device  *DeviceProfile
	visible bool
	thermal int
	// a game NEXUS paused because the app left the screen (resumed when it returns)
	pausedByLifecycle bool
}

func (a *App) mobile() *mobileState {
	a.mobileOnce.Do(func() { a.mobileSt = &mobileState{visible: true} })
	return a.mobileSt
}

// SetDevice records the device profile the app reported.
func (a *App) SetDevice(d DeviceProfile) {
	m := a.mobile()
	m.mu.Lock()
	m.device, m.thermal = &d, d.ThermalStatus
	m.mu.Unlock()
	a.Events.Emit("platform.device", CheckReadiness(d))
}

// PlatformInfo is GET /api/platform.
func (a *App) PlatformInfo() map[string]any {
	m := a.mobile()
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]any{"platform": runtime.GOOS, "mobile": m.device != nil}
	if m.device != nil {
		d := *m.device
		d.ThermalStatus = m.thermal
		out["device"] = d
		out["readiness"] = CheckReadiness(d)
	}
	return out
}

// OnLifecycle: the app left or returned to the screen. A running game is paused while the app
// is not visible (Android takes its drawing surface away) and resumed when it returns, unless
// the user had paused it themselves.
func (a *App) OnLifecycle(ev LifecycleEvent) {
	m := a.mobile()
	m.mu.Lock()
	wasVisible := m.visible
	m.visible = ev == LifecycleResume
	m.mu.Unlock()
	if me, ok := a.Emu.mobileHost(); ok {
		me.Lifecycle(ev)
	}
	st := a.Emu.State()
	switch {
	case ev == LifecycleStop && st.Running && !st.Suspended:
		a.Emu.Suspend(true)
		m.mu.Lock()
		m.pausedByLifecycle = true
		m.mu.Unlock()
		a.Log.Printf("platform: app in the background, game paused")
	case ev == LifecycleResume && !wasVisible:
		m.mu.Lock()
		resume := m.pausedByLifecycle
		m.pausedByLifecycle = false
		m.mu.Unlock()
		if resume && a.Emu.State().Running {
			a.Emu.Suspend(false)
			a.Log.Printf("platform: app in front again, game resumed")
		}
	}
	a.Events.Emit("platform.lifecycle", map[string]any{"event": int(ev)})
}

// OnThermal: the device's thermal status changed (PowerManager). From "severe" on, NEXUS tells
// the user; the system itself throttles the processor.
func (a *App) OnThermal(status int) {
	m := a.mobile()
	m.mu.Lock()
	old := m.thermal
	m.thermal = status
	m.mu.Unlock()
	if status != old {
		a.Log.Printf("platform: thermal status %s", thermalName(status))
		a.Events.Emit("platform.thermal", map[string]any{"status": status, "name": thermalName(status), "hot": status >= 3})
	}
}

// OnTrimMemory: Android asks the app to release memory (ComponentCallbacks2 levels). NEXUS frees
// its own caches and passes the request to the engine.
func (a *App) OnTrimMemory(level int) {
	dropped := 0
	if level >= 60 { // TRIM_MEMORY_MODERATE and above (or RUNNING_CRITICAL = 15 while in front)
		dropped = dropSaveInspections() // archives held in memory between "inspect" and "import"
	} else if level == 15 {
		dropped = dropSaveInspections()
	}
	if me, ok := a.Emu.mobileHost(); ok {
		me.TrimMemory(level)
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	debug.FreeOSMemory()
	runtime.ReadMemStats(&after)
	a.Log.Printf("platform: memory trim level %d: %d pending save imports dropped, %d MB returned to the system",
		level, dropped, (int64(after.HeapReleased)-int64(before.HeapReleased))>>20)
}

// dropSaveInspections forgets inspected archives the user has not imported yet (they hold the
// ZIP in memory); the user selects the archive again if needed.
func dropSaveInspections() int {
	inspMu.Lock()
	defer inspMu.Unlock()
	n := len(inspList)
	inspList = map[string]*saveSource{}
	return n
}

// mobileHost returns the engine host's mobile additions, if the loaded engine has them.
func (e *Emulator) mobileHost() (MobileEngine, bool) {
	e.mu.Lock()
	h := e.host
	e.mu.Unlock()
	if h == nil {
		return nil, false
	}
	me, ok := h.(MobileEngine)
	return me, ok
}
