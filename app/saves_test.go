package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testUID = "C36C52B3FCFA09313E76441554C5F2B5"

func writeSave(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("missing %s", p)
	}
	return string(b)
}

func TestSaveFolderNames(t *testing.T) {
	cases := map[string]string{
		"The Legend of Zelda: Ocarina of Time":    "0400486024DEC000-OOT",
		"The Legend of Zelda: Breath of the Wild": "0400486024DEC000-BOTW",
		"Cloud Garden":        "0400486024DEC000-CG",
		"Mario Kart 8 Deluxe": "0400486024DEC000-MK8DX",
		"Alpha Quest":         "0400486024DEC000-AQ",
		"":                    "0400486024DEC000",
		"<>|*":                "0400486024DEC000",
	}
	for in, want := range cases {
		if got := GameSaveFolderName("0400486024dec000", in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"slot_00": "Slot 00", "slot_5": "Slot 05", "account.sav": "Account Data", "option.sav": "Option Data", "data.bin": "data.bin"} {
		if got := itemLabel(in, strings.HasPrefix(in, "slot")); got != want {
			t.Errorf("label %q -> %q, want %q", in, got, want)
		}
	}
}

// writeOOTSave writes a save shaped like the Ocarina of Time remake's (six slots, account and
// option data) into dir.
func writeOOTSave(t *testing.T, dir, mark string) {
	for i := 0; i < 6; i++ {
		writeSave(t, filepath.Join(dir, "slot_0"+string(rune('0'+i)), "data.bin"), mark+" slot")
	}
	writeSave(t, filepath.Join(dir, "account.sav"), mark+" account")
	writeSave(t, filepath.Join(dir, "option.sav"), mark+" option")
}

// TestSaveMigrationFromV02: the 0.2 layout (readable folders + engine layout) becomes
// <PLAYER_ID>-<name>/<GAME_ID>-<SHORT>, after a backup, without losing or merging anything.
func TestSaveMigrationFromV02(t *testing.T) {
	app, root := newTestApp(t, "")
	app.Library.Scan()
	p, _ := app.Profiles.Create("Tom", Avatar{Kind: "glyph"})
	// a 0.2 user: no player ID, saves in System/Saves/"<id> - Tom"
	app.Profiles.mu.Lock()
	for _, pp := range app.Profiles.Profiles {
		pp.PlayerID = ""
	}
	app.Profiles.saveLocked()
	app.Profiles.mu.Unlock()
	os.RemoveAll(app.Paths.Saves)
	old := filepath.Join(app.Paths.System, "Saves", p.ID+" - Tom")
	writeSave(t, filepath.Join(old, "Alpha Quest", "slot_00", "data.bin"), "readable alpha")
	m := map[string]any{"version": 1, "folders": map[string]any{
		"Alpha Quest": map[string]any{"titleId": "0100AAAA00010000", "game": "Alpha Quest", "enginePath": "user/save/0000000000000000/" + testUID + "/0100AAAA00010000", "kind": "account"},
		"Empty one":   map[string]any{"titleId": "0100AAAA00020000", "kind": "pending"},
	}}
	data, _ := json.Marshal(m)
	writeSave(t, filepath.Join(old, ".nexus-saves.json"), string(data))
	os.MkdirAll(filepath.Join(old, "Empty one"), 0o755)
	eng := filepath.Join(old, "user", "save", "0000000000000000")
	// left in the engine layout by a crash: the same game again (conflict) and other saves
	writeSave(t, filepath.Join(eng, testUID, "0100AAAA00010000", "slot_00", "data.bin"), "engine alpha")
	writeSave(t, filepath.Join(eng, "00000000000000000000000000000000", "0100AAAA00010000", "device.dat"), "device")
	writeSave(t, filepath.Join(eng, testUID, "0100FFFF00010000", "x.sav"), "not in library")
	writeSave(t, filepath.Join(old, "user", "save", "cache", "0100AAAA00010000", "c.bin"), "cache")

	if err := app.Saves.Migrate(); err != nil {
		t.Fatal(err)
	}
	pr, _ := app.Profiles.Get(p.ID)
	if pr.PlayerID != testUID {
		t.Fatalf("player ID %q: the engine user the saves were written with must be kept", pr.PlayerID)
	}
	dir := app.Profiles.SaveDir(p.ID)
	if filepath.Base(dir) != testUID+"-Tom" || filepath.Dir(dir) != filepath.Join(root, "System", "UserData", "Saves") {
		t.Fatalf("player folder %s", dir)
	}
	if readFile(t, filepath.Join(dir, "0100AAAA00010000-AQ", "slot_00", "data.bin")) != "readable alpha" {
		t.Fatal("readable folder not converted")
	}
	if readFile(t, filepath.Join(dir, "0100AAAA00010000-AQ (other copy 1)", "slot_00", "data.bin")) != "engine alpha" {
		t.Fatal("the second copy of the same game must be kept beside it, not merged")
	}
	if readFile(t, filepath.Join(dir, "0100AAAA00010000-AQ (device)", "device.dat")) != "device" {
		t.Fatal("device save")
	}
	if readFile(t, filepath.Join(dir, "0100FFFF00010000", "x.sav")) != "not in library" {
		t.Fatal("a save of a game not in the library must be kept under its ID")
	}
	if !fileExists(filepath.Join(dir, "user", "save", "cache", "0100AAAA00010000", "c.bin")) || dirExists(filepath.Join(dir, "Empty one")) {
		t.Fatal("caches stay; empty 0.2 placeholders go")
	}
	bks, _ := filepath.Glob(filepath.Join(app.Paths.UserData, "Backups", "Saves", "*migration*.zip"))
	if len(bks) != 1 {
		t.Fatal("no backup before the migration")
	}
	zr, _ := zip.OpenReader(bks[0])
	n := 0
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "data.bin") {
			n++
		}
	}
	zr.Close()
	if n != 2 {
		t.Fatalf("backup holds %d of 2 slot files", n)
	}
	// runs once
	if err := app.Saves.Migrate(); err != nil || len(mustGlob(filepath.Join(app.Paths.UserData, "Backups", "Saves", "*.zip"))) != 1 {
		t.Fatal("migration ran twice")
	}
	sp, _ := app.Saves.Player(p.ID)
	if len(sp.Games) != 4 {
		t.Fatalf("games %+v", sp.Games)
	}
}

func mustGlob(p string) []string { m, _ := filepath.Glob(p); return m }

// TestSaveSessionUsesGameFolders: the engine is told the user's folder and each game's folder;
// users never share saves; saves an older engine writes in its own layout are adopted.
func TestSaveSessionUsesGameFolders(t *testing.T) {
	app, _ := newTestApp(t, "")
	app.Library.Scan()
	tom, _ := app.Profiles.Create("Tom", Avatar{Kind: "glyph"})
	ana, _ := app.Profiles.Create("Ana", Avatar{Kind: "glyph"})
	if tom.PlayerID == ana.PlayerID || len(tom.PlayerID) != 32 {
		t.Fatal("player IDs")
	}
	tdir, adir := app.Profiles.SaveDir(tom.ID), app.Profiles.SaveDir(ana.ID)
	writeOOTSave(t, filepath.Join(tdir, "0100AAAA00010000-AQ"), "tom")
	writeOOTSave(t, filepath.Join(adir, "0100AAAA00010000-AQ"), "ana")
	t.Setenv("NEXUS_FAKE_PLAY", "300ms")
	g, _ := app.Library.Get("t-0100aaaa00010000")
	for _, who := range []Profile{tom, ana} {
		if err := app.Emu.Launch(&g, who.ID); err != nil {
			t.Fatal(err)
		}
		req := fakeLast.req
		ini, _ := LoadIni(app.Emu.SessionPath())
		sd, _ := ini.Get("Data%20Storage", "save_directory")
		if !strings.Contains(sd, who.PlayerID+"-"+who.Name) {
			t.Fatalf("%s: engine save directory %q", who.Name, sd)
		}
		if f := req.SaveFolders["0100AAAA00010000"]; f["account"] != "0100AAAA00010000-AQ" || f["device"] != "0100AAAA00010000-AQ (device)" {
			t.Fatalf("save folders %v", req.SaveFolders)
		}
		waitIdle(t, app)
	}
	// an older engine (no save-folder support) wrote Ana's new save in its own layout
	writeSave(t, filepath.Join(adir, "user", "save", "0000000000000000", testUID, "0100AAAA00020000", "s.sav"), "old engine")
	app.Saves.Tidy(ana.ID)
	if readFile(t, filepath.Join(adir, gameDir(adir, "0100AAAA00020000", false), "s.sav")) != "old engine" || gameDir(adir, "0100AAAA00020000", false) != "0100AAAA00020000-B" {
		entries, _ := os.ReadDir(adir)
		t.Fatalf("not adopted: %v", entries)
	}
	if readFile(t, filepath.Join(tdir, "0100AAAA00010000-AQ", "account.sav")) != "tom account" || readFile(t, filepath.Join(adir, "0100AAAA00010000-AQ", "account.sav")) != "ana account" {
		t.Fatal("users' saves mixed")
	}
	sp, _ := app.Saves.Player(tom.ID)
	var labels []string
	for _, it := range sp.Games[0].Items {
		labels = append(labels, it.Label)
	}
	if strings.Join(labels, ",") != "Slot 00,Slot 01,Slot 02,Slot 03,Slot 04,Slot 05,Account Data,Option Data" {
		t.Fatalf("items %v", labels)
	}
	// renaming the user renames the folder; the player ID stays
	app.Profiles.Update(tom.ID, []byte(`{"name":"Thomas"}`))
	app.Saves.SyncProfileName(tom.ID)
	if filepath.Base(app.Profiles.SaveDir(tom.ID)) != tom.PlayerID+"-Thomas" {
		t.Fatal("rename")
	}
}

// TestSaveImportIdentifiesAndProtects: a Lichii-style folder (player/game IDs), a NEXUS export,
// a Ryujinx save and an unnamed folder; conflicts need confirmation and back up first.
func TestSaveImportIdentifiesAndProtects(t *testing.T) {
	app, _ := newTestApp(t, "")
	app.Library.Scan()
	tom, _ := app.Profiles.Create("Tom", Avatar{Kind: "glyph"})
	src := t.TempDir()
	lichii := filepath.Join(src, testUID)
	writeOOTSave(t, filepath.Join(lichii, "0100AAAA00010000"), "lichii")

	insp, err := app.Saves.Inspect(lichii, nil, "")
	if err != nil || len(insp.Candidates) != 1 {
		t.Fatalf("%+v %v", insp, err)
	}
	c := insp.Candidates[0]
	if c.PlayerID != testUID || c.TitleID != "0100AAAA00010000" || !c.Confident || c.Title != "Alpha Quest" || len(c.Items) != 8 {
		t.Fatalf("candidate %+v", c)
	}
	// the wrong game is refused for an identified save
	if _, err := app.Saves.Import(insp.Token, tom.ID, []SaveImportPick{{ID: c.ID, TitleID: "0100AAAA00020000"}}); err == nil {
		t.Fatal("identified save imported into another game")
	}
	if _, err := app.Saves.Import(insp.Token, tom.ID, []SaveImportPick{{ID: c.ID, TitleID: c.TitleID}}); err != nil {
		t.Fatal(err)
	}
	dir := app.Profiles.SaveDir(tom.ID)
	if readFile(t, filepath.Join(dir, "0100AAAA00010000-AQ", "slot_03", "data.bin")) != "lichii slot" || readFile(t, filepath.Join(dir, "0100AAAA00010000-AQ", "option.sav")) != "lichii option" {
		t.Fatal("import")
	}
	// the game folder itself selected, again: conflict, nothing changed
	writeOOTSave(t, filepath.Join(src, "again", "0100AAAA00010000"), "second")
	insp2, _ := app.Saves.Inspect(filepath.Join(src, "again", "0100AAAA00010000"), nil, "")
	conf, err := app.Saves.Import(insp2.Token, tom.ID, []SaveImportPick{{ID: "1", TitleID: "0100AAAA00010000"}})
	if !errors.Is(err, errSaveConflict) || len(conf) != 1 || readFile(t, filepath.Join(dir, "0100AAAA00010000-AQ", "account.sav")) != "lichii account" {
		t.Fatalf("conflict: %v %v", conf, err)
	}
	if _, err := app.Saves.Import(insp2.Token, tom.ID, []SaveImportPick{{ID: "1", TitleID: "0100AAAA00010000", Replace: true}}); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dir, "0100AAAA00010000-AQ", "account.sav")) != "second account" {
		t.Fatal("replace")
	}
	bks := app.Saves.Backups(tom.ID, "0100AAAA00010000")
	if len(bks) != 1 || bks[0].Reason != "before-import" {
		t.Fatalf("backups %+v", bks)
	}
	// restore the backup: the "second" save is backed up, the "lichii" one is back
	if err := app.Saves.Restore(tom.ID, bks[0].File); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dir, "0100AAAA00010000-AQ", "account.sav")) != "lichii account" || len(app.Saves.Backups(tom.ID, "0100AAAA00010000")) != 2 {
		t.Fatal("restore")
	}

	// a folder without IDs: the user must choose the game
	writeOOTSave(t, filepath.Join(src, "My OOT save"), "plain")
	insp3, _ := app.Saves.Inspect(filepath.Join(src, "My OOT save"), nil, "")
	if len(insp3.Candidates) != 1 || insp3.Candidates[0].Confident || insp3.Candidates[0].TitleID != "" {
		t.Fatalf("unnamed: %+v", insp3)
	}
	if _, err := app.Saves.Import(insp3.Token, tom.ID, []SaveImportPick{{ID: "1"}}); err == nil {
		t.Fatal("imported without a game")
	}
	if _, err := app.Saves.Import(insp3.Token, tom.ID, []SaveImportPick{{ID: "1", TitleID: "0100AAAA00020000"}}); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(dir, "0100AAAA00020000-B", "account.sav")) {
		entries, _ := os.ReadDir(dir)
		t.Fatalf("chosen game: %v", entries)
	}

	// Ryujinx: game and user from ExtraData0
	ryu := filepath.Join(src, "ryujinx", "0000000000000005")
	writeSave(t, filepath.Join(ryu, "0", "progress.bin"), "ryu")
	extra := make([]byte, 0x40)
	binary.LittleEndian.PutUint64(extra, 0x0100AAAA00030000)
	binary.LittleEndian.PutUint64(extra[8:], 1)
	os.WriteFile(filepath.Join(ryu, "ExtraData0"), extra, 0o644)
	insp4, _ := app.Saves.Inspect(filepath.Join(src, "ryujinx"), nil, "")
	if len(insp4.Candidates) != 1 || insp4.Candidates[0].TitleID != "0100AAAA00030000" || !insp4.Candidates[0].Confident || insp4.Format != "Ryujinx save" {
		t.Fatalf("ryujinx %+v", insp4)
	}

	// export Tom's saves; another user imports the ZIP and everything is identified
	var buf bytes.Buffer
	if err := app.Saves.Export(tom.ID, "", &buf); err != nil {
		t.Fatal(err)
	}
	ana, _ := app.Profiles.Create("Ana", Avatar{Kind: "glyph"})
	insp5, err := app.Saves.Inspect("", buf.Bytes(), "export.zip")
	if err != nil || len(insp5.Candidates) != 2 || insp5.Format != "NEXUS saves" {
		t.Fatalf("zip %+v %v", insp5, err)
	}
	var picks []SaveImportPick
	for _, c := range insp5.Candidates {
		if !c.Confident || c.ProfileID != tom.ID {
			t.Fatalf("zip candidate %+v", c)
		}
		picks = append(picks, SaveImportPick{ID: c.ID, TitleID: c.TitleID})
	}
	if _, err := app.Saves.Import(insp5.Token, ana.ID, picks); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(app.Profiles.SaveDir(ana.ID), "0100AAAA00010000-AQ", "slot_00", "data.bin")) != "lichii slot" {
		t.Fatal("zip import")
	}

	// delete: backed up first
	if _, err := app.Saves.Delete(tom.ID, "0100AAAA00020000-B"); err != nil {
		t.Fatal(err)
	}
	if dirExists(filepath.Join(dir, "0100AAAA00020000-B")) || len(app.Saves.Backups(tom.ID, "0100AAAA00020000")) != 1 {
		t.Fatal("delete")
	}
	if _, err := app.Saves.folderFor(tom.ID, `..\x`); err == nil {
		t.Fatal("path escape accepted")
	}
}

// TestLichiiSaveImport: Lichii's own save folder (nand/user/save) is found, inspected without
// changing it, and its saves identified by player and game; caches are not offered as saves.
func TestLichiiSaveImport(t *testing.T) {
	app, _ := newTestApp(t, "")
	app.Library.Scan()
	tom, _ := app.Profiles.Create("Tom", Avatar{Kind: "glyph"})
	ad := t.TempDir()
	t.Setenv("APPDATA", ad)
	save := filepath.Join(ad, "lichii", "nand", "user", "save")
	writeOOTSave(t, filepath.Join(save, "0000000000000000", testUID, "0100AAAA00010000"), "lichii")
	writeSave(t, filepath.Join(save, "0000000000000000", "00000000000000000000000000000000", "0100AAAA00010000", "dev.bin"), "device")
	writeSave(t, filepath.Join(save, "cache", "0100AAAA00010000", "shader.bin"), "cache")
	if lichiiSaveDir() != save {
		t.Fatalf("lichii dir %q", lichiiSaveDir())
	}
	before := mustHash(t, save)
	insp, err := app.Saves.Inspect(save, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(insp.Candidates) != 2 {
		t.Fatalf("%d candidates: %+v", len(insp.Candidates), insp.Candidates)
	}
	var picks []SaveImportPick
	for _, c := range insp.Candidates {
		if c.TitleID != "0100AAAA00010000" || !c.Confident {
			t.Fatalf("%+v", c)
		}
		if !c.Device && (c.PlayerID != testUID || len(c.Items) != 8) {
			t.Fatalf("account save %+v", c)
		}
		picks = append(picks, SaveImportPick{ID: c.ID, TitleID: c.TitleID, Device: c.Device})
	}
	if _, err := app.Saves.Import(insp.Token, tom.ID, picks); err != nil {
		t.Fatal(err)
	}
	dir := app.Profiles.SaveDir(tom.ID)
	if readFile(t, filepath.Join(dir, "0100AAAA00010000-AQ", "account.sav")) != "lichii account" ||
		readFile(t, filepath.Join(dir, "0100AAAA00010000-AQ (device)", "dev.bin")) != "device" {
		t.Fatal("import")
	}
	if mustHash(t, save) != before {
		t.Fatal("the Lichii folder was changed")
	}
}

// mustHash: names, sizes and contents of every file below dir.
func mustHash(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			data, _ := os.ReadFile(p)
			b.WriteString(p + "|" + string(data) + "\n")
		}
		return nil
	})
	return b.String()
}
