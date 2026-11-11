package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestApp builds a NEXUS root (portable layout) with keys in the engine data folder and
// three games:
//
//	A 0100AAAA00010000  has an embedded icon + name
//	B 0100AAAA00020000  program NCA only (no icon) -> online lookup
//	C 0100AAAA00030000  ID only from the file name; online DB returns a different game -> rejected
//
// It enables the in-process test engine (NEXUS_FAKE_ENGINE), which follows the real engine's
// state machine and audio gain path but never runs a game.
func newTestApp(t *testing.T, nlib string) (*App, string) {
	t.Setenv("NEXUS_FAKE_ENGINE", "1")
	root := t.TempDir()
	keys := filepath.Join(root, "System", "Engine", "keys")
	os.MkdirAll(keys, 0o755)
	k := testKeys{header: bytes.Repeat([]byte{0x11, 0x22}, 16), kak: bytes.Repeat([]byte{0x33}, 16), kek: bytes.Repeat([]byte{0x44}, 16)}
	writeKeys(t, keys, k)
	os.MkdirAll(filepath.Join(root, "Games"), 0o755)
	type f = struct {
		name string
		data []byte
	}
	icon := jpegBytes(256, color.RGBA{200, 30, 30, 255})
	romfs := buildRomFS(map[string][]byte{"control.nacp": makeNACP("Alpha Quest", "1.0"), "icon_AmericanEnglish.dat": icon})
	os.WriteFile(filepath.Join(root, "Games", "a.nsp"), buildPFS0([]f{{"p.nca", buildNCA(t, k, 0, 0x0100AAAA00010000, nil, nil, nil)}, {"c.nca", buildNCA(t, k, 2, 0x0100AAAA00010000, romfs, nil, nil)}}), 0o644)
	os.WriteFile(filepath.Join(root, "Games", "b.nsp"), buildPFS0([]f{{"p.nca", buildNCA(t, k, 0, 0x0100AAAA00020000, nil, nil, nil)}}), 0o644)
	os.WriteFile(filepath.Join(root, "Games", "Charlie Racing [0100AAAA00030000].xci"), []byte("unreadable image: ID only from file name"), 0o644)
	app, err := testNewApp(t, root)
	if err != nil {
		t.Fatal(err)
	}
	app.Settings.Update(func(s *GlobalSettings) { s.Library.NlibURL = nlib })
	return app, root
}

func jpegBytes(size int, c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
	}
	var b bytes.Buffer
	jpeg.Encode(&b, img, nil)
	return b.Bytes()
}

// fakeNlib mimics the documented Nlib API (GET /nx/:tid?fields=..., GET /nx/:tid/icon/:size).
func fakeNlib(t *testing.T) *httptest.Server {
	db := map[string]string{"0100AAAA00020000": "Bravo Island Deluxe", "0100AAAA00030000": "Totally Different Game", "0100AAAA00010000": "Alpha Quest"}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 2 || parts[0] != "nx" {
			w.WriteHeader(404)
			return
		}
		name, ok := db[parts[1]]
		if !ok {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"success":false}`)
			return
		}
		if len(parts) == 2 {
			json.NewEncoder(w).Encode(map[string]string{"id": parts[1], "name": name, "icon": "http://x/nx/" + parts[1] + "/icon"})
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(jpegBytes(510, color.RGBA{20, 120, 220, 255}))
	}))
}

func waitOnline(app *App) {
	for i := 0; i < 100; i++ {
		st := app.Library.online.Status()
		if !st["busy"].(bool) && st["pending"].(int) == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func waitEvent(t *testing.T, ch chan []byte, typ string) map[string]any {
	deadline := time.After(2 * time.Second)
	for {
		select {
		case raw := <-ch:
			var ev struct {
				Type string         `json:"type"`
				Data map[string]any `json:"data"`
			}
			if json.Unmarshal(raw, &ev) == nil && ev.Type == typ {
				return ev.Data
			}
		case <-deadline:
			return nil
		}
	}
}

func waitIdle(t *testing.T, app *App) {
	for i := 0; i < 400 && app.Emu.State().Running; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if app.Emu.State().Running {
		t.Fatal("session did not end")
	}
}

func TestOnlineArtwork(t *testing.T) {
	srv := fakeNlib(t)
	defer srv.Close()
	app, root := newTestApp(t, srv.URL)
	if err := app.Library.Scan(); err != nil {
		t.Fatal(err)
	}
	waitOnline(app)
	app.Library.RefreshArt()
	a, _ := app.Library.Get("t-0100aaaa00010000")
	b, _ := app.Library.Get("t-0100aaaa00020000")
	c, _ := app.Library.Get("t-0100aaaa00030000")
	if a.ArtSource != "embedded" || a.Title != "Alpha Quest" {
		t.Fatalf("A should use its embedded icon: %+v", a)
	}
	if b.ArtSource != "online" || b.Title != "Bravo Island Deluxe" {
		t.Fatalf("B should use downloaded art + online name: %+v", b)
	}
	if !fileExists(filepath.Join(root, "System", "GameProfiles", "0100AAAA00020000", "online-icon.jpg")) {
		t.Fatal("online icon not cached")
	}
	if c.ArtSource != "" || !strings.Contains(c.OnlineErr, "name mismatch") {
		t.Fatalf("C must be rejected (name mismatch): %+v", c)
	}
	if err := app.Library.SetCustomArt(b.Key, jpegBytes(1024, color.RGBA{0, 200, 0, 255}), ".jpg"); err != nil {
		t.Fatal(err)
	}
	b, _ = app.Library.Get(b.Key)
	if b.ArtSource != "custom" {
		t.Fatalf("custom art: %+v", b)
	}
	app.Library.SetArtOptions(b.Key, "online", "")
	app2, _ := testNewApp(t, root)
	app2.Library.Scan()
	b2, _ := app2.Library.Get(b.Key)
	if b2.ArtSource != "online" || !b2.HasCustom {
		t.Fatalf("artwork choice lost after restart: %+v", b2)
	}
}

func TestNamesMatch(t *testing.T) {
	cases := []struct {
		a, b string
		ok   bool
	}{
		{"The Legend of Zelda™: Breath of the Wild", "The Legend of Zelda: Breath of the Wild", true},
		{"Mario Kart 8 Deluxe", "MARIO KART 8 DELUXE", true},
		{"Pokémon Scarlet", "Pokémon Scarlet and Pokémon Violet", true},
		{"Charlie Racing", "Totally Different Game", false},
	}
	for _, c := range cases {
		if namesMatch(c.a, c.b) != c.ok {
			t.Errorf("namesMatch(%q,%q) != %v", c.a, c.b, c.ok)
		}
	}
}

func TestAvatarImport(t *testing.T) {
	root := t.TempDir()
	app, _ := testNewApp(t, root)
	src := filepath.Join(t.TempDir(), "my-icons")
	png := jpegBytes(64, color.RGBA{1, 2, 3, 255})
	for _, p := range []string{"super-series/characters/c1.png", "super-series/characters/c2.png", "super-series/backgrounds/b1.png", "super-series/frames/f1.png", "other/x.png", "README.md"} {
		os.MkdirAll(filepath.Dir(filepath.Join(src, p)), 0o755)
		os.WriteFile(filepath.Join(src, p), png, 0o644)
	}
	n, cats, err := app.Avatars.ImportFolder(src)
	if err != nil || n != 5 || len(cats) != 2 {
		t.Fatalf("import: %d %v %v", n, cats, err)
	}
	if n2, _, _ := app.Avatars.ImportFolder(src); n2 != 0 {
		t.Fatal("re-import should skip existing files")
	}
}

func TestDataDirSurvivesReinstall(t *testing.T) {
	install1, data := t.TempDir(), t.TempDir()
	app, err := testNewAppWithPaths(t, NewPathsSplit(install1, data, false))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := app.Profiles.Create("Tom", Avatar{})
	os.MkdirAll(app.Profiles.SaveDir(p.ID), 0o755)
	os.WriteFile(filepath.Join(app.Profiles.SaveDir(p.ID), "slot.bin"), []byte("save"), 0o644)
	app.Close()
	os.RemoveAll(install1)
	install2 := t.TempDir()
	app2, err := testNewAppWithPaths(t, NewPathsSplit(install2, data, false))
	if err != nil {
		t.Fatal(err)
	}
	if len(app2.Profiles.List()) != 1 || !fileExists(filepath.Join(app2.Profiles.SaveDir(p.ID), "slot.bin")) {
		t.Fatal("profile/saves lost after reinstall")
	}
	if !strings.HasPrefix(app2.Emu.DataDir(), data) {
		t.Fatal("engine data (keys, firmware, NAND) must live in the user data folder, not the install folder")
	}
}

func TestMigrateOldLayout(t *testing.T) {
	install, data := t.TempDir(), t.TempDir()
	old := filepath.Join(install, "System", "Profiles")
	os.MkdirAll(old, 0o755)
	os.WriteFile(filepath.Join(old, "profiles.json"), []byte(`{"profiles":[{"id":"abc","name":"Old"}]}`), 0o644)
	os.MkdirAll(filepath.Join(install, "System", "Saves", "abc"), 0o755)
	os.WriteFile(filepath.Join(install, "System", "Saves", "abc", "s.bin"), []byte("x"), 0o644)
	app, _ := testNewAppWithPaths(t, NewPathsSplit(install, data, false))
	app.migrateToDataDir()
	pr, _ := LoadProfiles(app.Paths)
	if len(pr.List()) != 1 || !fileExists(filepath.Join(data, "Saves", "abc", "s.bin")) {
		t.Fatal("old data not migrated")
	}
}

// TestImportPreviousEngineData: keys/firmware from the emulator data folder NEXUS 1.x used
// are brought into the NEXUS engine data folder once, without touching the originals.
func TestImportPreviousEngineData(t *testing.T) {
	appdata := t.TempDir()
	t.Setenv("APPDATA", appdata)
	eden := filepath.Join(appdata, "eden")
	os.MkdirAll(filepath.Join(eden, "keys"), 0o755)
	os.WriteFile(filepath.Join(eden, "keys", "prod.keys"), []byte("header_key = 00\n"), 0o644)
	reg := filepath.Join(eden, "nand", "system", "Contents", "registered")
	os.MkdirAll(reg, 0o755)
	os.WriteFile(filepath.Join(reg, "a.nca"), []byte("nca"), 0o644)
	os.MkdirAll(filepath.Join(eden, "nand", "user", "save"), 0o755)
	os.WriteFile(filepath.Join(eden, "nand", "user", "save", "s.bin"), []byte("save"), 0o644)
	app, _ := testNewAppWithPaths(t, NewPathsSplit(t.TempDir(), t.TempDir(), false))
	app.importPreviousEngineData()
	if !app.Emu.KeysPresent() || !app.Emu.FirmwarePresent() {
		t.Fatal("keys/firmware not imported")
	}
	if fileExists(filepath.Join(app.Emu.DataDir(), "nand", "user", "save", "s.bin")) {
		t.Fatal("saves must only be imported when the user asks")
	}
	if !fileExists(filepath.Join(eden, "keys", "prod.keys")) || !fileExists(filepath.Join(reg, "a.nca")) {
		t.Fatal("originals must be kept")
	}
	if app.Emu.NandSaveDir() == "" {
		t.Fatal("previous saves should be offered for import")
	}
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		r    int
	}{{"1.2.0", "1.10.0", -1}, {"v2.0.1", "2.0.1", 0}, {"build-150", "build-130", 1}, {"", "0.1", -1}} {
		if got := CompareVersions(c.a, c.b); got != c.r {
			t.Errorf("CompareVersions(%q,%q)=%d want %d", c.a, c.b, got, c.r)
		}
	}
}

// TestEngineSessionAndPlaytime plays a game through the engine host: the session
// configuration is written, play time counts only while the engine renders frames,
// checkpoints are written and recovered, compatibility is recorded, and no program is started.
func TestEngineSessionAndPlaytime(t *testing.T) {
	app, root := newTestApp(t, "")
	t.Setenv("NEXUS_FAKE_PLAY", "2500ms")
	checkpointEvery = 400 * time.Millisecond
	defer func() { checkpointEvery = 30 * time.Second }()
	app.Library.Scan()
	g, _ := app.Library.Get("t-0100aaaa00010000")
	p, _ := app.Profiles.Create("Tom", Avatar{Kind: "glyph"})
	app.Emu.SetGlobal(map[string]*string{"graphics.backend": sp("opengl")})
	app.Emu.SetGameSettings(g, map[string]*string{"graphics.scale": sp("2")}, nil)

	exits := app.Events.Subscribe()
	defer app.Events.Unsubscribe(exits)
	if err := app.Emu.Launch(&g, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := app.Emu.Launch(&g, p.ID); err == nil {
		t.Fatal("a second game must be refused while one runs")
	}
	// session configuration: global + game + per-user save folder
	ini, err := LoadIni(app.Emu.SessionPath())
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := ini.GetSetting("Renderer", "backend"); v != "0" {
		t.Fatalf("global backend not applied: %q", v)
	}
	if v, _ := ini.GetSetting("Renderer", "resolution_setup"); v != "6" {
		t.Fatalf("per-game scale not applied: %q", v)
	}
	if v, _ := ini.Get("Data%20Storage", "save_directory"); !strings.Contains(v, p.ID) {
		t.Fatalf("save directory %q is not the user's", v)
	}
	sawCheckpoint, sawPlaying := false, false
	for i := 0; i < 400 && app.Emu.State().Running; i++ {
		if fileExists(filepath.Join(app.Paths.Profiles, "live-session.json")) {
			sawCheckpoint = true
		}
		sawPlaying = sawPlaying || app.Emu.State().Phase == "playing"
		time.Sleep(20 * time.Millisecond)
	}
	if !sawCheckpoint || !sawPlaying {
		t.Fatalf("checkpoint=%v playing=%v", sawCheckpoint, sawPlaying)
	}
	// the session is finished (play time, compatibility) when emulator.exit is sent
	if waitEvent(t, exits, "emulator.exit") == nil {
		t.Fatal("no emulator.exit event")
	}
	gs := app.Profiles.Stats(p.ID).Games[g.Key]
	if gs == nil || gs.Sessions != 1 || gs.Seconds < 2 || gs.Seconds > 3 || gs.ByEmulator[engineID] != gs.Seconds {
		t.Fatalf("stats %+v", gs)
	}
	if fileExists(filepath.Join(app.Paths.Profiles, "live-session.json")) {
		t.Fatal("checkpoint should be cleared after a clean exit")
	}
	c := app.Compat.Get(g.Key)
	if c.Sessions != 1 || c.Booted != 1 || c.Status != "boots" || c.System != "switch1" {
		t.Fatalf("compat %+v", c)
	}
	// crash recovery
	now := nowUnix()
	app.Profiles.Checkpoint(&LiveSession{ProfileID: p.ID, Key: g.Key, Title: g.Title, Emulator: engineID, Start: now - 600, LastSeen: now - 60})
	app2, _ := testNewApp(t, root)
	if rec := app2.Profiles.RecoverLiveSession(); rec == nil || rec.Seconds != 540 {
		t.Fatalf("recover %+v", rec)
	}
	// user rating overrides the automatic result and persists
	if err := app2.Compat.Rate(g.Key, g.Title, "works", ""); err != nil {
		t.Fatal(err)
	}
	if v := NewCompatStore(app.Paths.Settings).Get(g.Key); v.Status != "works" || v.Source != "user" {
		t.Fatalf("rating %+v", v)
	}
	if app2.Compat.Rate(g.Key, g.Title, "perfect", "") == nil {
		t.Fatal("unknown rating accepted")
	}
}

func TestEngineLoadFailureAndStop(t *testing.T) {
	app, root := newTestApp(t, "")
	os.WriteFile(filepath.Join(root, "Games", "bad [0100AAAA00090000].nsp"), []byte("x"), 0o644)
	app.Library.Scan()
	var bad Game
	for _, g := range app.Library.Games() {
		if strings.Contains(g.Path, "bad") {
			bad = g
		}
	}
	results := app.Events.Subscribe()
	defer app.Events.Unsubscribe(results)
	if err := app.Emu.Launch(&bad, ""); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, app)
	res := waitEvent(t, results, "emulator.exit")
	le, ok := res["error"].(map[string]any)
	if !ok || le["code"] != "load" {
		t.Fatalf("load error not reported: %+v", res)
	}
	if c := app.Compat.Get(bad.Key); c.Status != "unsupported" {
		t.Fatalf("compat %+v", c)
	}
	// stop from NEXUS
	t.Setenv("NEXUS_FAKE_PLAY", "30s")
	g, _ := app.Library.Get("t-0100aaaa00010000")
	app.Emu.Launch(&g, "")
	for i := 0; i < 100 && app.Emu.State().Phase != "playing"; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if err := app.Emu.Stop(false); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, app)
}

// TestVolumeReachesEngineAudio checks the 0-200 % volume end to end: it is stored, sent to the
// running engine at once, changes the engine's audio output with the engine's own gain curve
// (soft-limited above 100 %), and survives a restart.
func TestVolumeReachesEngineAudio(t *testing.T) {
	app, root := newTestApp(t, "")
	t.Setenv("NEXUS_FAKE_PLAY", "30s")
	upd := filepath.Join(root, "Games", "Alpha Quest Update [0100AAAA00010800].nsp")
	os.WriteFile(upd, []byte("update container"), 0o644)
	os.WriteFile(filepath.Join(root, "Games", "Other Update [0100BBBB00010800].nsp"), []byte("x"), 0o644)
	app.Library.Scan()
	g, _ := app.Library.Get("t-0100aaaa00010000")
	if err := app.Emu.SetVolume(100); err != nil {
		t.Fatal(err)
	}
	if err := app.Emu.Launch(&g, ""); err != nil {
		t.Fatal(err)
	}
	defer func() { app.Emu.Stop(true); waitIdle(t, app) }()
	for i := 0; i < 100 && app.Emu.State().Phase != "playing"; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	h, _ := app.Emu.Host()
	fe := h.(*fakeEngine)
	if len(fe.req.Contents) != 1 || fe.req.Contents[0] != upd || !fe.req.AutoMapController {
		t.Fatalf("update/DLC and controller mapping not passed to the engine: %+v", fe.req)
	}
	peak := func() int {
		time.Sleep(120 * time.Millisecond)
		return fe.Peak()
	}
	ref := peak()
	app.Emu.SetVolume(0)
	if p := peak(); p != 0 {
		t.Fatalf("0%% must be silent, peak %d", p)
	}
	app.Emu.SetVolume(50)
	if p := peak(); p < ref/2-2 || p > ref/2+2 {
		t.Fatalf("50%%: peak %d, reference %d", p, ref)
	}
	app.Emu.SetVolume(101)
	if p := peak(); p < ref {
		t.Fatalf("101%% must not be quieter than 100%% (upstream bug): %d < %d", p, ref)
	}
	app.Emu.SetVolume(200)
	p200 := peak()
	if p200 <= ref || p200 > 32767 {
		t.Fatalf("200%%: peak %d (ref %d)", p200, ref)
	}
	if st := h.Status(); st.Volume != 200 {
		t.Fatalf("engine volume %d", st.Volume)
	}
	if app.Emu.SetVolume(250); app.Emu.Volume() != 200 {
		t.Fatal("volume must be clamped to 200%")
	}
	app.Emu.SetVolume(140)
	app2, _ := testNewApp(t, root)
	if app2.Emu.Volume() != 140 {
		t.Fatalf("volume not persisted: %d", app2.Emu.Volume())
	}
}

func TestGainCurve(t *testing.T) {
	// same formula as the engine's src/audio_core/sink/nexus_gain.h
	if softLimit(1000) != 1000 || softLimit(-1000) != -1000 {
		t.Fatal("below the knee the limiter must not change the signal")
	}
	prev := 0.0
	for x := 0.0; x < 32767*3; x += 97 {
		y := softLimit(x)
		if y < prev || y > 32767 {
			t.Fatalf("limiter not monotonic/bounded at %v: %v", x, y)
		}
		prev = y
	}
}

// ---------- engine builds

func fakeDLL(tag string) []byte {
	dll := append([]byte("MZ"), make([]byte, 0x3e)...)
	dll[0x3c] = 0x40
	dll = append(dll, []byte("PE\x00\x00\x64\x86")...)
	dll = append(dll, make([]byte, 2048)...)
	return append(dll, []byte(tag)...)
}

// makeEngineZip builds a packaged engine in the layout package_engine.py produces:
// nexus_engine.dll, Libraries/SDL3.dll, engine.json with the SHA-256 of both.
func makeEngineZip(t *testing.T, version string, api int, corrupt bool) string {
	files := map[string][]byte{"nexus_engine.dll": fakeDLL(version), "Libraries/SDL3.dll": fakeDLL("sdl" + version)}
	m := EngineManifest{Name: "NEXUS Engine", Version: version, API: api, Commit: "abc", UpstreamBase: "build-130", Files: map[string]string{}}
	for n, d := range files {
		sum := sha256.Sum256(d)
		m.Files[n] = hex.EncodeToString(sum[:])
	}
	mj, _ := json.Marshal(m)
	p := filepath.Join(t.TempDir(), "nexus-engine-"+version+"-win64.zip")
	f, _ := os.Create(p)
	zw := zip.NewWriter(f)
	for n, d := range files {
		w, _ := zw.Create(n)
		if corrupt && n == "Libraries/SDL3.dll" {
			d = append([]byte{}, d...)
			d[len(d)-1] ^= 0xff
		}
		w.Write(d)
	}
	w, _ := zw.Create("engine.json")
	w.Write(mj)
	zw.Close()
	f.Close()
	return p
}

// TestBundledEngine: the engine inside the NEXUS folder (Engine\) is found relative to NEXUS,
// verified, used without any install step, and survives the NEXUS folder being moved.
func TestBundledEngine(t *testing.T) {
	root := t.TempDir()
	if err := extractZipTo(makeEngineZip(t, "2.1.0", engineAPIVersion, false), filepath.Join(root, "Engine")); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(t.TempDir(), "Somewhere else", "NEXUS")
	os.MkdirAll(filepath.Dir(moved), 0o755)
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	app, err := testNewApp(t, moved)
	if err != nil {
		t.Fatal(err)
	}
	b := app.Emu.Builds
	if !b.BundledPresent() || b.activeVersion() != bundledVersion || b.ActiveDir() != filepath.Join(moved, "Engine") || b.ActiveVersionLabel() != "2.1.0" {
		t.Fatalf("bundled engine not found: %q %q", b.activeVersion(), b.ActiveDir())
	}
	if err := b.VerifyActive(); err != nil {
		t.Fatal(err)
	}
	if l := b.List(); len(l) != 1 || !l[0].Bundled || !l[0].Active {
		t.Fatalf("list %+v", l)
	}
	if err := b.Remove(bundledVersion); err == nil {
		t.Fatal("the bundled engine must not be removable")
	}
	// a damaged dependency is detected
	os.WriteFile(filepath.Join(moved, "Engine", "Libraries", "SDL3.dll"), []byte("MZ damaged"), 0o644)
	app2, _ := testNewApp(t, moved)
	if err := app2.Emu.Builds.VerifyActive(); err == nil || !strings.Contains(err.Error(), "SDL3.dll") {
		t.Fatalf("damaged library not detected: %v", err)
	}
	if ok, why := app2.Emu.EngineAvailable(); ok || !strings.Contains(why, "Extract the complete NEXUS ZIP") {
		t.Fatalf("unhelpful message: %v %q", ok, why)
	}
}

func TestEngineBuildsInstallActivateRollback(t *testing.T) {
	app, root := newTestApp(t, "")
	b := app.Emu.Builds
	if b.HasActive() {
		t.Fatal("no engine expected")
	}
	extractZipTo(makeEngineZip(t, "2.1.0", engineAPIVersion, false), filepath.Join(root, "Engine"))
	if _, err := b.InstallZip(makeEngineZip(t, "0.9.0", engineAPIVersion, true), true); err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Fatalf("corrupt build accepted: %v", err)
	}
	if _, err := b.InstallZip(makeEngineZip(t, "0.9.0", 1, false), true); err == nil || !strings.Contains(err.Error(), "interface version") {
		t.Fatalf("wrong API accepted: %v", err)
	}
	if _, err := b.InstallZip(makeEngineZip(t, "bundled", engineAPIVersion, false), true); err == nil {
		t.Fatal("a build may not claim to be the bundled engine")
	}
	if len(b.List()) != 1 {
		t.Fatal("rejected builds must not be installed")
	}
	if _, err := b.InstallZip(makeEngineZip(t, "1.0.0", engineAPIVersion, false), true); err != nil {
		t.Fatal(err)
	}
	if _, err := b.InstallZip(makeEngineZip(t, "1.1.0", engineAPIVersion, false), true); err != nil {
		t.Fatal(err)
	}
	if v := b.activeVersion(); v != "1.1.0" || !b.HasActive() || len(b.List()) != 3 {
		t.Fatalf("active %s, %d builds", v, len(b.List()))
	}
	if !strings.HasPrefix(b.BuildsDir(), app.Paths.System) {
		t.Fatal("added builds belong in the user data folder, not the NEXUS folder")
	}
	if err := b.Remove("1.1.0"); err == nil {
		t.Fatal("active build must not be removable")
	}
	if v, err := b.Rollback(); err != nil || v != "1.0.0" || b.activeVersion() != "1.0.0" {
		t.Fatalf("rollback %s %v", v, err)
	}
	if v, err := b.Rollback(); err != nil || v != bundledVersion || b.ActiveDir() != filepath.Join(root, "Engine") {
		t.Fatalf("rollback to bundled %s %v", v, err)
	}
	if _, err := b.Rollback(); err == nil {
		t.Fatal("nothing earlier than the bundled engine")
	}
	os.WriteFile(filepath.Join(b.BuildsDir(), "1.1.0", "nexus_engine.dll"), []byte("MZ broken"), 0o644)
	if err := b.Activate("1.1.0"); err == nil {
		t.Fatal("damaged build activated")
	}
	if err := b.Remove("1.1.0"); err != nil {
		t.Fatal(err)
	}
	if err := b.Activate("../x"); err == nil {
		t.Fatal("path traversal accepted")
	}
}

// TestNoEmulatorProgramsRemain guards the central v2 rule: NEXUS must not start an external
// emulator program to run a game, and the UI must not offer other emulators.
func TestNoEmulatorProgramsRemain(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, _ := os.ReadFile(f)
		s := string(src)
		for _, bad := range []string{"lichii.exe", "Ryujinx.exe", "eden-cli.exe", "yuzu-cmd"} {
			if strings.Contains(strings.ToLower(s), strings.ToLower(bad)) {
				t.Errorf("%s still refers to %s", f, bad)
			}
		}
		for _, line := range strings.Split(s, "\n") {
			if strings.Contains(line, "exec.Command(") && !strings.Contains(line, "explorer.exe") &&
				!strings.Contains(line, "xdg-open") && !strings.Contains(line, "exec.Command(p, args...)") { // p = msedge.exe fallback UI
				t.Errorf("%s starts an external program: %s", f, strings.TrimSpace(line))
			}
		}
	}
	for _, f := range []string{"web/index.html", "web/app.js"} {
		src, _ := os.ReadFile(f)
		for _, bad := range []string{"Lichii", "Ryujinx", "lichii", "ryujinx"} {
			if n := strings.Count(string(src), bad); n > 0 {
				t.Errorf("%s mentions %s %d times", f, bad, n)
			}
		}
	}
}

var _ = fmt.Sprint

// TestHomeSuspendKeepsSession: Home while playing pauses the game (engine suspend), keeps the
// session, does not count the paused time as play time, and resumes on request.
func TestHomeSuspendKeepsSession(t *testing.T) {
	app, _ := newTestApp(t, "")
	t.Setenv("NEXUS_FAKE_PLAY", "30s")
	app.Library.Scan()
	g, _ := app.Library.Get("t-0100aaaa00010000")
	p, _ := app.Profiles.Create("Kai", Avatar{Kind: "glyph"})
	ev := app.Events.Subscribe()
	defer app.Events.Unsubscribe(ev)
	if err := app.Emu.Launch(&g, p.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100 && app.Emu.State().Phase != "playing"; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	if err := app.Emu.Suspend(true); err != nil {
		t.Fatal(err)
	}
	if waitEvent(t, ev, "emulator.home") == nil {
		t.Fatal("no emulator.home event")
	}
	st := app.Emu.State()
	if !st.Running || !st.Suspended {
		t.Fatalf("session must stay loaded and suspended: %+v", st)
	}
	h, _ := app.Emu.Host()
	f1 := h.Status().Frames
	time.Sleep(1500 * time.Millisecond) // paused time
	if f2 := h.Status().Frames; f2 != f1 {
		t.Fatalf("the game kept running while suspended (%d -> %d frames)", f1, f2)
	}
	if err := app.Emu.Suspend(false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(600 * time.Millisecond)
	if app.Emu.State().Suspended {
		t.Fatal("not resumed")
	}
	app.Emu.Stop(false)
	waitIdle(t, app)
	gs := app.Profiles.Stats(p.ID).Games[g.Key]
	if gs == nil || gs.Seconds > 2 {
		t.Fatalf("suspended time must not count as play time: %+v", gs)
	}
}
