package main

import (
	"testing"
	"time"
)

func TestClassifyGPU(t *testing.T) {
	cases := map[string]string{
		"Adreno (TM) 750":            "recommended",
		"Adreno (TM) 830":            "recommended",
		"Adreno (TM) 740":            "recommended",
		"Adreno (TM) 660":            "supported",
		"Adreno (TM) 619":            "limited",
		"Adreno (TM) 540":            "unsupported",
		"Mali-G715 Immortalis MC11":  "supported", // Dimensity 9200 reports this form
		"Immortalis-G925":            "supported",
		"Mali-G720":                  "supported",
		"Mali-G78 MP20":              "limited",
		"Mali-G57 MC2":               "unsupported",
		"Mali-G52":                   "unsupported",
		"Samsung Xclipse 940":        "unknown",
		"PowerVR B-Series BXM-8-256": "unknown",
		"":                           "unknown",
	}
	for in, want := range cases {
		if got := ClassifyGPU(in); got.Level != want {
			t.Errorf("%q: %s (%+v), want %s", in, got.Level, got, want)
		}
	}
}

func goodDevice() DeviceProfile {
	return DeviceProfile{APILevel: 34, Release: "14", GPURenderer: "Adreno (TM) 750", VulkanVersion: "1.3", ABIs: []string{"arm64-v8a", "armeabi-v7a"},
		TotalRAMMB: 16 * 1024, FreeStorageMB: 64 * 1024, AllFilesAccess: true, EngineLibrary: true}
}

func stateOf(r Readiness, id string) string {
	for _, it := range r.Items {
		if it.ID == id {
			return it.State
		}
	}
	return "missing"
}

func TestReadinessMinimumAndRecommended(t *testing.T) {
	if r := CheckReadiness(goodDevice()); !r.Ready {
		t.Fatalf("recommended device not ready: %+v", r)
	}
	for name, c := range map[string]struct {
		mod       func(*DeviceProfile)
		id, state string
		ready     bool
	}{
		"android 12":      {func(d *DeviceProfile) { d.APILevel, d.Release = 32, "12L" }, "os", "fail", false},
		"32-bit only":     {func(d *DeviceProfile) { d.ABIs = []string{"armeabi-v7a"} }, "abi", "fail", false},
		"x86_64 emulator": {func(d *DeviceProfile) { d.ABIs = []string{"x86_64"} }, "abi", "fail", false},
		"no vulkan":       {func(d *DeviceProfile) { d.VulkanVersion = "" }, "vulkan", "fail", false},
		"vulkan 1.0":      {func(d *DeviceProfile) { d.VulkanVersion = "1.0.61" }, "vulkan", "fail", false},
		"vulkan 1.1":      {func(d *DeviceProfile) { d.VulkanVersion = "1.1.128" }, "vulkan", "warn", true},
		"6 GB":            {func(d *DeviceProfile) { d.TotalRAMMB = 6 * 1024 }, "ram", "fail", false},
		"8 GB":            {func(d *DeviceProfile) { d.TotalRAMMB = 8 * 1024 }, "ram", "warn", true},
		"12 GB":           {func(d *DeviceProfile) { d.TotalRAMMB = 12 * 1024 }, "ram", "ok", true},
		"mali g715":       {func(d *DeviceProfile) { d.GPURenderer = "Mali-G715" }, "gpu", "warn", true},
		"old adreno":      {func(d *DeviceProfile) { d.GPURenderer = "Adreno (TM) 530" }, "gpu", "fail", false},
		"no file access":  {func(d *DeviceProfile) { d.AllFilesAccess = false }, "files", "warn", true},
		"hot":             {func(d *DeviceProfile) { d.ThermalStatus = 4 }, "thermal", "warn", true},
		"low storage":     {func(d *DeviceProfile) { d.FreeStorageMB = 1000 }, "storage", "warn", true},
		"no engine":       {func(d *DeviceProfile) { d.EngineLibrary = false }, "engine", "fail", false},
	} {
		d := goodDevice()
		c.mod(&d)
		r := CheckReadiness(d)
		if got := stateOf(r, c.id); got != c.state || r.Ready != c.ready {
			t.Errorf("%s: %s=%s ready=%v, want %s ready=%v", name, c.id, got, r.Ready, c.state, c.ready)
		}
	}
}

// The app leaving the screen pauses a running game; coming back resumes it. A game the user
// paused (NEXUS Home) stays paused.
func TestLifecyclePausesAndResumes(t *testing.T) {
	app, _ := newTestApp(t, "")
	t.Setenv("NEXUS_FAKE_PLAY", "30s")
	app.Library.Scan()
	p, _ := app.Profiles.Create("Tom", Avatar{Kind: "glyph"})
	app.Settings.Update(func(g *GlobalSettings) { g.Emulator.HomeAction = "pause" })
	g, _ := app.Library.Get("t-0100aaaa00010000")
	if err := app.Emu.Launch(&g, p.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { st := app.Emu.State(); return st.Running && st.Phase == "playing" })
	app.OnLifecycle(LifecyclePause) // notification shade: keep playing
	if app.Emu.State().Suspended {
		t.Fatal("paused on a partial pause")
	}
	app.OnLifecycle(LifecycleStop)
	waitFor(t, func() bool { return app.Emu.State().Suspended })
	app.OnLifecycle(LifecycleResume)
	waitFor(t, func() bool { return !app.Emu.State().Suspended })
	// paused by the user before leaving: stays paused when the app returns
	_ = app.Emu.Suspend(true)
	waitFor(t, func() bool { return app.Emu.State().Suspended })
	app.OnLifecycle(LifecycleStop)
	app.OnLifecycle(LifecycleResume)
	time.Sleep(300 * time.Millisecond)
	if !app.Emu.State().Suspended {
		t.Fatal("a game the user paused was resumed")
	}
	_ = app.Emu.Stop(true)
}

func TestTrimMemoryDropsPendingImports(t *testing.T) {
	app, _ := newTestApp(t, "")
	if _, err := app.Saves.Inspect("", makeZip(t, ootEntries("0100AAAA00010000", "x")), "a.zip"); err != nil {
		t.Fatal(err)
	}
	app.OnTrimMemory(80)
	inspMu.Lock()
	n := len(inspList)
	inspMu.Unlock()
	if n != 0 {
		t.Fatalf("%d inspections kept", n)
	}
	info := app.PlatformInfo()
	if info["mobile"] != false {
		t.Fatal("desktop reported as mobile")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 250; i++ {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out")
}
