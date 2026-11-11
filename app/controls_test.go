package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestControllerLayouts(t *testing.T) {
	app, root := newTestApp(t, "")
	t.Setenv("NEXUS_FAKE_PLAY", "30s")
	os.WriteFile(filepath.Join(root, "Games", "Super Mario Party [0100BBBB00010000].nsp"), []byte("x"), 0o644)
	app.Library.Scan()
	p, _ := app.Profiles.Create("Lu", Avatar{Kind: "glyph"})
	g, _ := app.Library.Get("t-0100aaaa00010000")
	var smp Game
	for _, x := range app.Library.Games() {
		if x.Title == "Super Mario Party" {
			smp = x
		}
	}
	// defaults: Nintendo positions, automatic controller
	l, src, _ := app.Emu.EffectiveLayout(&g, &p)
	if src != "default" || l.Resolve()["a"] != "east" || l.ControllerType != "" {
		t.Fatalf("default layout %+v %s", l, src)
	}
	// a game that needs Joy-Cons gets its preset
	if l, src, pre := app.Emu.EffectiveLayout(&smp, &p); pre == nil || l.ControllerType != "dual_joycon" || src != "preset" {
		t.Fatalf("preset %+v %s", l, src)
	}
	// user layout: Xbox labels with one custom button; invalid input refused
	if _, err := normalizeLayout(GameLayout{Base: "xbox", Buttons: map[string]string{"a": "nowhere"}}); err == nil {
		t.Fatal("invalid position accepted")
	}
	ul, _ := normalizeLayout(GameLayout{Base: "xbox", Buttons: map[string]string{"zr": "rshoulder", "a": "south"}})
	if len(ul.Buttons) != 1 { // a=south is already the xbox default
		t.Fatalf("defaults must not be stored as custom: %v", ul.Buttons)
	}
	pr, _ := app.Profiles.Get(p.ID)
	pr.Controls.GameLayout = ul
	if l, src, _ := app.Emu.EffectiveLayout(&g, &pr); src != "user" || l.Resolve()["a"] != "south" || l.Resolve()["zr"] != "rshoulder" {
		t.Fatalf("user layout %+v", l.Resolve())
	}
	// a game's own layout wins, only for that game
	own := GameLayout{Base: "nintendo", Buttons: map[string]string{"b": "west"}, ControllerType: "handheld"}
	app.Library.GP.Update(g.Key, func(gp *GameProfile) { gp.Controls = &own })
	if l, src, _ := app.Emu.EffectiveLayout(&g, &pr); src != "game" || l.Resolve()["b"] != "west" || l.ControllerType != "handheld" {
		t.Fatalf("game layout %+v %s", l, src)
	}
	if l, _, _ := app.Emu.EffectiveLayout(&smp, &pr); l.Resolve()["b"] != "east" {
		t.Fatal("another game's layout leaked")
	}
	// the engine receives the layout at launch
	if err := app.Emu.Launch(&g, p.ID); err != nil {
		t.Fatal(err)
	}
	h, _ := app.Emu.Host()
	req := h.(*fakeEngine).req
	if req.ButtonMap["b"] != "west" || req.ButtonMap["a"] != "east" || req.ControllerType != "handheld" || len(req.ButtonMap) != 12 {
		t.Fatalf("request %+v %s", req.ButtonMap, req.ControllerType)
	}
	app.Emu.Stop(true)
	waitIdle(t, app)
}

func TestProfileLimitEight(t *testing.T) {
	app, err := NewApp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxProfiles; i++ {
		if _, err := app.Profiles.Create(string(rune('A'+i))+"user", Avatar{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := app.Profiles.Create("Ninth", Avatar{}); err == nil {
		t.Fatal("a ninth user was created")
	}
	if len(app.Profiles.List()) != 8 {
		t.Fatal("count")
	}
}
