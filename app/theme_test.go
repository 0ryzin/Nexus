package main

import "testing"

// TestThemePersists: light/dark mode, accent colour and animation choices are saved with the
// user and read back after a restart.
func TestThemePersists(t *testing.T) {
	app, _ := newTestApp(t, "")
	p, _ := app.Profiles.Create("Ana", Avatar{Kind: "glyph"})
	if _, err := app.Profiles.Update(p.ID, []byte(`{"theme":{"mode":"light","accent":"#30d158","animations":"reduced"}}`)); err != nil {
		t.Fatal(err)
	}
	again, err := LoadProfiles(app.Paths)
	if err != nil {
		t.Fatal(err)
	}
	q, ok := again.Get(p.ID)
	if !ok || q.Theme.Mode != "light" || q.Theme.Accent != "#30d158" || q.Theme.Animations != "reduced" {
		t.Fatalf("after restart: %+v", q.Theme)
	}
	app.Profiles.Update(p.ID, []byte(`{"theme":{"mode":"dark"}}`))
	again, _ = LoadProfiles(app.Paths)
	if q, _ := again.Get(p.ID); q.Theme.Mode != "dark" || q.Theme.Accent != "#30d158" {
		t.Fatalf("partial change lost other choices: %+v", q.Theme)
	}
}
