package main

import "testing"

// TestSwitch2LowRAMWarning: a Switch 2 game on a PC with less than 16 GB of RAM starts with a
// warning (the engine forces 12 GB of emulated memory); Switch 1 games get none.
func TestSwitch2LowRAMWarning(t *testing.T) {
	app, _ := newTestApp(t, "")
	hwCache = &Hardware{Memory: MemoryInfo{TotalMB: 12000}}
	defer func() { hwCache = nil }()
	ev := app.Events.Subscribe()
	defer app.Events.Unsubscribe(ev)
	s := app.Settings.Get()
	if err := app.Emu.checkMemoryLayout(&Game{Title: "Alpha", TitleID: "0100AAAA00010000"}, s); err != nil {
		t.Fatal(err)
	}
	if err := app.Emu.checkMemoryLayout(&Game{Title: "Ocarina", TitleID: "0400486024DEC000"}, s); err != nil {
		t.Fatal(err)
	}
	if e := waitEvent(t, ev, "launch.warning"); e == nil {
		t.Fatal("no warning for a Switch 2 game on 12 GB")
	}
}
