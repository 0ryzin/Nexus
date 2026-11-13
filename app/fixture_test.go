package main

import (
	"bytes"
	"encoding/binary"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMakeFixtures writes synthetic (non-copyrighted) game files for UI testing.
// Run with NEXUS_FIXTURE_ROOT=/path go test -run TestMakeFixtures
func TestMakeFixtures(t *testing.T) {
	root := os.Getenv("NEXUS_FIXTURE_ROOT")
	if root == "" {
		t.Skip("NEXUS_FIXTURE_ROOT not set")
	}
	icons := filepath.Join(root, "fixture_icons")
	k := testKeys{header: bytes.Repeat([]byte{0x11, 0x22}, 16), kak: bytes.Repeat([]byte{0x33}, 16), kek: bytes.Repeat([]byte{0x44}, 16)}
	keysDir := filepath.Join(root, "Emulator", "Lichii", "user", "keys")
	os.MkdirAll(keysDir, 0o755)
	writeKeys(t, keysDir, k)
	games := filepath.Join(root, "Games")
	os.MkdirAll(filepath.Join(games, "Sub"), 0o755)
	mk := func(file, name string, id uint64, icon int) {
		ic, _ := os.ReadFile(filepath.Join(icons, string(rune('0'+icon))+".jpg"))
		romfs := buildRomFS(map[string][]byte{"control.nacp": makeNACP(name, "1.0.0"), "icon_AmericanEnglish.dat": ic})
		nsp := buildPFS0([]struct {
			name string
			data []byte
		}{{"p.nca", buildNCA(t, k, 0, id, nil, nil, nil)}, {"c.nca", buildNCA(t, k, 2, id, romfs, nil, nil)}})
		os.WriteFile(filepath.Join(games, file), nsp, 0o644)
	}
	mk("starfall.nsp", "Starfall Rally", 0x0100AAAA00010000, 0)
	mk("Sub/moss.nsp", "Moss & Moon", 0x0100AAAA00020000, 1)
	mk("harbor.nsz", "Pixel Harbor", 0x0100AAAA00030000, 2)
	mk("starfall-update.nsp", "Starfall Rally", 0x0100AAAA00010800, 0)
	os.WriteFile(filepath.Join(games, "Cloud Garden [0100BBBB00010000][v0].xci"), []byte("not a real image"), 0o644)
	// homebrew NRO
	nro := make([]byte, 0x100)
	copy(nro[0x10:], "NRO0")
	binary.LittleEndian.PutUint32(nro[0x18:], 0x100)
	ic, _ := os.ReadFile(filepath.Join(icons, "3.jpg"))
	nacp := makeNACP("Homebrew Menu", "2.4")
	aset := make([]byte, 0x38)
	copy(aset, "ASET")
	binary.LittleEndian.PutUint64(aset[8:], 0x38)
	binary.LittleEndian.PutUint64(aset[16:], uint64(len(ic)))
	binary.LittleEndian.PutUint64(aset[24:], uint64(0x38+len(ic)))
	binary.LittleEndian.PutUint64(aset[32:], uint64(len(nacp)))
	nro = append(append(append(nro, aset...), ic...), nacp...)
	os.WriteFile(filepath.Join(games, "hbmenu.nro"), nro, 0o644)
}

// TestMakeOnlineFixture adds a game with no icon inside (artwork must come online).
func TestMakeOnlineFixture(t *testing.T) {
	root := os.Getenv("NEXUS_FIXTURE_ROOT")
	if root == "" {
		t.Skip()
	}
	k := testKeys{header: bytes.Repeat([]byte{0x11, 0x22}, 16), kak: bytes.Repeat([]byte{0x33}, 16), kek: bytes.Repeat([]byte{0x44}, 16)}
	nsp := buildPFS0([]struct {
		name string
		data []byte
	}{{"p.nca", buildNCA(t, k, 0, 0x0100AAAA00090000, nil, nil, nil)}})
	os.WriteFile(filepath.Join(root, "Games", "new-download.nsp"), nsp, 0o644)
}

// TestMakeFirmwareFixture writes user-supplied-style system files (keys + a NAND
// avatar archive) whose icons are original test drawings from NEXUS_FIXTURE_ICONS.
func TestMakeFirmwareFixture(t *testing.T) {
	root, icons := os.Getenv("NEXUS_FIXTURE_ROOT"), os.Getenv("NEXUS_FIXTURE_ICONS")
	if root == "" || icons == "" {
		t.Skip()
	}
	chars, bgs := map[string][]byte{}, map[string][]byte{}
	entries, _ := os.ReadDir(icons)
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(icons, e.Name()))
		n := strings.TrimSuffix(e.Name(), ".rgba")
		if strings.HasPrefix(n, "bg_") {
			bgs[n] = b
		} else {
			chars[n] = b
		}
	}
	writeFakeFirmware(t, root, chars, bgs)
}

// TestMakeV16Fixtures writes (synthetic) files for the 1.6 UI walkthrough:
// an Ocarina of Time remake look-alike NSP plus near-misses, and a keys file
// and firmware dump in NEXUS_FIXTURE_SYS. Nothing here is Nintendo data.
func TestMakeV16Fixtures(t *testing.T) {
	root, sys := os.Getenv("NEXUS_FIXTURE_ROOT"), os.Getenv("NEXUS_FIXTURE_SYS")
	if root == "" || sys == "" {
		t.Skip("NEXUS_FIXTURE_ROOT / NEXUS_FIXTURE_SYS not set")
	}
	k := testKeys{header: bytes.Repeat([]byte{0x11, 0x22}, 16), kak: bytes.Repeat([]byte{0x33}, 16), kek: bytes.Repeat([]byte{0x44}, 16)}
	type f = struct {
		name string
		data []byte
	}
	nsp := func(file, name, pub string, id uint64, ic color.RGBA) {
		rom := buildRomFS(map[string][]byte{"control.nacp": nacpWith(name, pub), "icon_AmericanEnglish.dat": jpegBytes(256, ic)})
		os.WriteFile(filepath.Join(root, "Games", file), buildPFS0([]f{{"p.nca", buildNCA(t, k, 0, id, nil, nil, nil)}, {"c.nca", buildNCA(t, k, 2, id, rom, nil, nil)}}), 0o644)
	}
	nsp("Ocarina of Time (Switch 2).nsp", "The Legend of Zelda: Ocarina of Time", "Nintendo", 0x0400486024DEC000, color.RGBA{40, 90, 40, 255})
	nsp("Ocarina of Time 3D.nsp", "The Legend of Zelda: Ocarina of Time 3D", "Nintendo", 0x0100BBBB00020000, color.RGBA{90, 60, 30, 255})
	os.MkdirAll(sys, 0o755)
	writeKeys(t, sys, k)
	os.WriteFile(filepath.Join(sys, "bad.keys"), []byte("hello\nthis is not a keys file\n"), 0o644)
	writeTestFirmware(t, filepath.Join(sys, "Firmware 19.0.1"), k, "19.0.1", true, 40)
	zipFirmwareDir(t, filepath.Join(sys, "Firmware 19.0.1"), filepath.Join(sys, "Firmware 19.0.1.zip"))
	writeTestFirmware(t, filepath.Join(sys, "incomplete"), k, "", false, 3)
}

func init() { allowFakeEngine = true }

// testNewApp / testNewAppWithPaths close the app (its log file) when the test ends, before the
// test's temporary folders are removed: Windows cannot delete a file that is still open.
func testNewApp(t testing.TB, root string) (*App, error) {
	if os.Getenv("NEXUS_SYSTEM_FILES") == "" {
		t.Setenv("NEXUS_SYSTEM_FILES", filepath.Join(root, ".vault-test"))
	}
	a, err := NewApp(root)
	if a != nil {
		t.Cleanup(a.Close)
		t.Cleanup(func() { stopTestSession(a) })
	}
	return a, err
}

func testNewAppWithPaths(t testing.TB, p *Paths) (*App, error) {
	if os.Getenv("NEXUS_SYSTEM_FILES") == "" {
		t.Setenv("NEXUS_SYSTEM_FILES", filepath.Join(p.Data, ".vault-test"))
	}
	a, err := NewAppWithPaths(p)
	if a != nil {
		t.Cleanup(a.Close)
	}
	return a, err
}

// stopTestSession ends a test's game session and waits until NEXUS has written its records, so
// nothing writes into the test folder while it is removed.
func stopTestSession(a *App) {
	if !a.Emu.State().Running {
		return
	}
	_ = a.Emu.Stop(true)
	for i := 0; i < 250 && a.Emu.State().Running; i++ {
		time.Sleep(20 * time.Millisecond)
	}
}
