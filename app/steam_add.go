package main

// Add NEXUS to Steam (Settings › General › Steam): one entry in Steam's shortcut list, so NEXUS
// appears in the library and in Gaming Mode with its name, icon and artwork.
//
// Steam has no supported interface for adding a non-Steam game: its own "Add a Non-Steam Game"
// dialog and tools such as Steam ROM Manager (which EmuDeck uses) all write the user's
// userdata/<id>/config/shortcuts.vdf. NEXUS does the same, carefully:
//   - only when the user asks, and only while Steam is closed (Steam keeps the list in memory and
//     writes it back when it exits, which would undo the change or race with it);
//   - the file is copied to shortcuts.vdf.nexus-backup-<time> first;
//   - the new entry is inserted before the file's two closing bytes: every existing byte, and so
//     every existing shortcut, stays exactly as it was; nothing is edited or removed;
//   - a NEXUS entry that is already there (isNexusShortcut) is never added twice;
//   - the written file is read back and must list the new entry, else the backup is restored.
// The entry: AppName NEXUS, Exe the AppImage (or tar.gz launcher), StartDir its folder, icon the
// NEXUS icon, overlay allowed. Its app ID is Steam's own formula for shortcuts (CRC32 of exe and
// name, top bit set), stored in the entry, so the artwork NEXUS installs matches it.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// SteamAddResult: what Add to Steam did, per Steam user.
type SteamAddResult struct {
	Added   []string `json:"added"`   // Steam user IDs that got the entry
	Already []string `json:"already"` // users that had NEXUS already
	AppID   uint32   `json:"appId"`
	Art     int      `json:"art"` // shortcuts that received artwork
}

// shortcutAppID is Steam's ID for a non-Steam shortcut: CRC32 of the quoted exe and the name,
// with the top bit set.
func shortcutAppID(exe, name string) uint32 {
	return crc32.ChecksumIEEE([]byte(exe+name)) | 0x80000000
}

func vdfPutStr(b *bytes.Buffer, key, val string) {
	b.WriteByte(0x01)
	b.WriteString(key)
	b.WriteByte(0)
	b.WriteString(val)
	b.WriteByte(0)
}

func vdfPutInt(b *bytes.Buffer, key string, v uint32) {
	b.WriteByte(0x02)
	b.WriteString(key)
	b.WriteByte(0)
	_ = binary.Write(b, binary.LittleEndian, v)
}

// shortcutEntry is one shortcut, as Steam writes it, under the given index key.
func shortcutEntry(index int, appID uint32, name, exe, dir, icon string) []byte {
	var b bytes.Buffer
	b.WriteByte(0x00)
	b.WriteString(strconv.Itoa(index))
	b.WriteByte(0)
	vdfPutInt(&b, "appid", appID)
	vdfPutStr(&b, "AppName", name)
	vdfPutStr(&b, "Exe", exe)
	vdfPutStr(&b, "StartDir", dir)
	vdfPutStr(&b, "icon", icon)
	vdfPutStr(&b, "ShortcutPath", "")
	vdfPutStr(&b, "LaunchOptions", "")
	vdfPutInt(&b, "IsHidden", 0)
	vdfPutInt(&b, "AllowDesktopConfig", 1)
	vdfPutInt(&b, "AllowOverlay", 1)
	vdfPutInt(&b, "OpenVR", 0)
	vdfPutInt(&b, "Devkit", 0)
	vdfPutStr(&b, "DevkitGameID", "")
	vdfPutInt(&b, "DevkitOverrideAppID", 0)
	vdfPutInt(&b, "LastPlayTime", 0)
	vdfPutStr(&b, "FlatpakAppID", "")
	b.WriteByte(0x00)
	b.WriteString("tags")
	b.WriteByte(0)
	b.WriteByte(0x08)
	b.WriteByte(0x08)
	return b.Bytes()
}

var emptyShortcuts = []byte("\x00shortcuts\x00\x08\x08")

// appendShortcut returns the file with the entry added before its two closing bytes; existing
// bytes are untouched. The index is one more than the highest one in the list.
func appendShortcut(file []byte, appID uint32, name, exe, dir, icon string) ([]byte, error) {
	if len(file) == 0 {
		file = emptyShortcuts
	}
	if !bytes.HasPrefix(file, []byte("\x00shortcuts\x00")) || !bytes.HasSuffix(file, []byte{0x08, 0x08}) {
		return nil, errors.New("Steam's shortcut list has a layout NEXUS does not recognise; nothing was changed")
	}
	next := shortcutsCount(file)
	if next < 0 {
		return nil, errors.New("Steam's shortcut list could not be read; nothing was changed")
	}
	out := make([]byte, 0, len(file)+512)
	out = append(out, file[:len(file)-2]...)
	out = append(out, shortcutEntry(next, appID, name, exe, dir, icon)...)
	out = append(out, 0x08, 0x08)
	return out, nil
}

// shortcutsCount: one more than the highest entry index in the list (-1: unreadable).
func shortcutsCount(b []byte) int {
	r := bytes.NewReader(b)
	readStr := func() (string, bool) {
		var s []byte
		for {
			c, err := r.ReadByte()
			if err != nil {
				return "", false
			}
			if c == 0 {
				return string(s), true
			}
			s = append(s, c)
		}
	}
	next := 0
	var walk func(depth int) bool
	walk = func(depth int) bool {
		for {
			t, err := r.ReadByte()
			if err != nil {
				return false
			}
			if t == 0x08 {
				return true
			}
			key, ok := readStr()
			if !ok {
				return false
			}
			switch t {
			case 0x00:
				if depth == 1 {
					if n, err := strconv.Atoi(key); err == nil && n+1 > next {
						next = n + 1
					}
				}
				if !walk(depth + 1) {
					return false
				}
			case 0x01:
				if _, ok := readStr(); !ok {
					return false
				}
			case 0x02:
				if _, err := r.Seek(4, 1); err != nil {
					return false
				}
			case 0x07:
				if _, err := r.Seek(8, 1); err != nil {
					return false
				}
			default:
				return false
			}
		}
	}
	if !walk(0) {
		return -1
	}
	return next
}

// AddNexusToSteam adds NEXUS to every Steam user's shortcut list that does not have it yet.
func (a *App) AddNexusToSteam() (SteamAddResult, error) {
	var res SteamAddResult
	st := a.SteamStatus()
	if !st.Supported {
		return res, errors.New("Adding NEXUS to Steam is for Linux and the Steam Deck; on Windows add it in Steam › Games › Add a Non-Steam Game.")
	}
	users := steamUserdata()
	if len(users) == 0 {
		return res, errors.New("Steam was not found on this device (no Steam user folders). Install Steam and sign in once, then try again.")
	}
	if steamRunning() {
		return res, errors.New("Close Steam first: Steam keeps its shortcut list in memory and writes it back when it exits. In Desktop Mode: Steam › Exit (not just closing the window). Then choose Add NEXUS to Steam again; start Steam afterwards.")
	}
	exe := `"` + st.Launcher + `"`
	dir := `"` + filepath.Dir(st.Launcher) + `"`
	icon := st.IconPath
	res.AppID = shortcutAppID(exe, "NEXUS")
	for _, u := range users {
		id := filepath.Base(u)
		path := filepath.Join(u, "config", "shortcuts.vdf")
		old, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return res, err
		}
		have := false
		for _, sc := range parseShortcuts(old) {
			if isNexusShortcut(sc, st.Launcher) {
				have = true
			}
		}
		if have {
			res.Already = append(res.Already, id)
			continue
		}
		data, err := appendShortcut(old, res.AppID, "NEXUS", exe, dir, icon)
		if err != nil {
			return res, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return res, err
		}
		backup := ""
		if len(old) > 0 {
			backup = path + ".nexus-backup-" + time.Now().Format("20060102-150405")
			if err := os.WriteFile(backup, old, 0o644); err != nil {
				return res, err
			}
		}
		if err := writeFileAtomic(path, data); err != nil {
			return res, err
		}
		check, _ := os.ReadFile(path)
		found := false
		for _, sc := range parseShortcuts(check) {
			found = found || sc.appID == res.AppID && sc.name == "NEXUS"
		}
		if !found || len(parseShortcuts(check)) != len(parseShortcuts(old))+1 {
			if backup != "" {
				_ = os.WriteFile(path, old, 0o644)
			}
			return res, fmt.Errorf("Steam's shortcut list did not read back as expected; the original was restored")
		}
		_ = os.Chmod(path, 0o644)
		res.Added = append(res.Added, id)
		a.Log.Printf("steam: NEXUS added to Steam user %s's shortcuts (app ID %d); backup %s", id, res.AppID, firstNonEmpty(backup, "none (new list)"))
	}
	if n, err := a.InstallSteamArtwork(); err == nil {
		res.Art = n
	}
	return res, nil
}
