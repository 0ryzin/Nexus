package main

import (
	"os"
	"testing"
)

// Joy-Con Compatibility: Disabled turns off only NEXUS's own Joy-Con handling (the engine's
// driver and SDL's direct Joy-Con support) and puts the user's own environment back when Enabled.
func TestJoyConCompat(t *testing.T) {
	t.Setenv(sdlJoyConHint, "user-value")
	joyConEnv.saved, joyConEnv.set = false, false
	a, _ := newTestApp(t, "")
	m := engineBinds["controls.joycon_compat"]
	if m.key != "Controls/enable_joycon_driver" || m.vals["enabled"] != "true" || m.vals["disabled"] != "false" {
		t.Fatalf("engine mapping %+v", m)
	}
	if got := engineProject(map[string]string{"controls.joycon_compat": "disabled"}, false); got["Controls/enable_joycon_driver"] != "false" {
		t.Fatalf("not written for the engine: %v", got)
	}
	if !validNexusValue("controls.joycon_compat", "disabled") || validNexusValue("controls.joycon_compat", "off") {
		t.Fatal("setting values")
	}
	a.applyJoyConCompat() // Enabled (not set): nothing changes
	if v := os.Getenv(sdlJoyConHint); v != "user-value" {
		t.Fatalf("enabled changed the environment: %q", v)
	}
	a.Settings.Update(func(s *GlobalSettings) { s.Engine["controls.joycon_compat"] = "disabled" })
	a.applyJoyConCompat()
	if v := os.Getenv(sdlJoyConHint); v != "0" {
		t.Fatalf("disabled: %q", v)
	}
	a.Settings.Update(func(s *GlobalSettings) { s.Engine["controls.joycon_compat"] = "enabled" })
	a.applyJoyConCompat()
	if v := os.Getenv(sdlJoyConHint); v != "user-value" {
		t.Fatalf("the user's own value was not put back: %q", v)
	}
}
