package main

import (
	"encoding/json"
	"testing"
)

// TestHomePrefsPerUser: each user's home screen (layout, folders, favourites, shortcuts) is their
// own; changing one user's never changes another's, and bad values fall back to the defaults.
func TestHomePrefsPerUser(t *testing.T) {
	app, _ := newTestApp(t, "")
	a, _ := app.Profiles.Create("Ann", Avatar{Kind: "glyph"})
	b, _ := app.Profiles.Create("Ben", Avatar{Kind: "glyph"})
	if a.Home.Layout != "classic" || a.Home.Effects != "glass" || a.Home.Count != 12 || !a.Home.Widgets["recent"] {
		t.Fatalf("defaults: %+v", a.Home)
	}
	patch, _ := json.Marshal(map[string]any{"home": map[string]any{
		"layout": "cards", "favorites": []string{"t-1"}, "bgColor": "#1b2a41",
		"folders": []map[string]any{{"id": "f1", "name": " Racing ", "games": []string{"t-2"}}, {"id": "f1", "name": "dup"}, {"id": "", "name": "no id"}},
		"dock":    []string{"power", "library"}, "dockHidden": []string{"media"}, "widgets": map[string]bool{"storage": true},
	}})
	pa, err := app.Profiles.Update(a.ID, patch)
	if err != nil {
		t.Fatal(err)
	}
	if pa.Home.Layout != "cards" || len(pa.Home.Folders) != 1 || pa.Home.Folders[0].Name != "Racing" || pa.Home.BgColor != "#1b2a41" ||
		!pa.Home.Widgets["storage"] || !pa.Home.Widgets["recent"] {
		t.Fatalf("Ann after the change: %+v", pa.Home)
	}
	pb, _ := app.Profiles.Get(b.ID)
	if pb.Home.Layout != "classic" || len(pb.Home.Favorites) != 0 || len(pb.Home.Folders) != 0 || len(pb.Home.Dock) != 0 {
		t.Fatalf("Ben's home changed with Ann's: %+v", pb.Home)
	}
	bad, _ := json.Marshal(map[string]any{"home": map[string]any{"layout": "wiiu", "bgColor": "red; background:url(x)", "count": 999, "effects": "x"}})
	pa, _ = app.Profiles.Update(a.ID, bad)
	if pa.Home.Layout != "classic" || pa.Home.BgColor != "" || pa.Home.Count != 12 || pa.Home.Effects != "glass" {
		t.Fatalf("invalid values kept: %+v", pa.Home)
	}
	// kept across a restart (the store reloads from disk)
	st2, err := LoadProfiles(app.Paths)
	if err == nil {
		if p, ok := st2.Get(a.ID); !ok || len(p.Home.Folders) != 1 || p.Home.Favorites[0] != "t-1" {
			t.Fatalf("not kept: %+v", p.Home)
		}
	}
}
