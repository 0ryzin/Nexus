package main

import (
	"path/filepath"
	"testing"
)

// TestStatsMergeOldFileKey: play time recorded while a game was only known by its file name is
// merged into the identified game after a scan; another game with the same title is not.
func TestStatsMergeOldFileKey(t *testing.T) {
	app, root := newTestApp(t, "")
	app.Library.Scan()
	p, _ := app.Profiles.Create("Tom", Avatar{Kind: "glyph"})
	old := gameKey(GameInfo{}, filepath.Join(root, "Games", "a.nsp")) // before the Title ID was readable
	app.Profiles.AddSession(p.ID, old, "Alpha Quest", SessionRec{Start: 100, End: 700, Seconds: 600})
	app.Profiles.AddSession(p.ID, "t-0100aaaa00010000", "Alpha Quest", SessionRec{Start: 1000, End: 1300, Seconds: 300})
	app.Profiles.AddSession(p.ID, "t-0100aaaa00099999", "Alpha Quest", SessionRec{Start: 2000, End: 2100, Seconds: 100}) // another edition
	app.Library.Scan()
	s := app.Profiles.LocalStats(p.ID)
	if _, ok := s.Games[old]; ok {
		t.Fatal("old entry kept")
	}
	g := s.Games["t-0100aaaa00010000"]
	if g == nil || g.Seconds != 900 || g.Sessions != 2 || len(g.History) != 2 || g.History[0].Start != 1000 {
		t.Fatalf("merged: %+v", g)
	}
	if e := s.Games["t-0100aaaa00099999"]; e == nil || e.Seconds != 100 {
		t.Fatal("a different edition was merged")
	}
	again, _ := LoadProfiles(app.Paths)
	if again.LocalStats(p.ID).Games["t-0100aaaa00010000"].Seconds != 900 {
		t.Fatal("not saved")
	}
}

// TestSetPlaytime: a corrected total is kept after a restart and later sessions add to it.
func TestSetPlaytime(t *testing.T) {
	app, _ := newTestApp(t, "")
	p, _ := app.Profiles.Create("Tom", Avatar{Kind: "glyph"})
	app.Profiles.AddSession(p.ID, "t-0100aaaa00010000", "Alpha Quest", SessionRec{Start: 1, End: 3601, Seconds: 3600})
	if _, err := app.Profiles.SetPlaytime(p.ID, "t-0100aaaa00010000", 15*3600+30*60); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Profiles.SetPlaytime(p.ID, "t-0100aaaa00010000", -5); err == nil {
		t.Fatal("negative accepted")
	}
	app.Profiles.AddSession(p.ID, "t-0100aaaa00010000", "Alpha Quest", SessionRec{Start: 9000, End: 9600, Seconds: 600})
	again, _ := LoadProfiles(app.Paths)
	g := again.Stats(p.ID).Games["t-0100aaaa00010000"]
	if g.Seconds != 15*3600+30*60+600 || g.Adjusted != 14*3600+30*60 || g.Sessions != 2 {
		t.Fatalf("%+v", g)
	}
}
