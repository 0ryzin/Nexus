package main

// Pointing NEXUS's Steam entry at a new version: each NEXUS release is a new AppImage file
// (NEXUS-<version>-Linux-amd64.AppImage), so the Steam entry made for the old one starts a file
// that may be gone. Adding the new one would give a second "NEXUS" entry with another app ID and
// without the artwork. Instead, with the user's go-ahead and while Steam is closed, NEXUS changes
// only its own entry's Exe and StartDir. The entry keeps its app ID, so Steam keeps its artwork,
// play time, controller layout and collections. Every other entry is byte-for-byte unchanged;
// the list is backed up first and read back after writing.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type vdfNode struct {
	typ   byte
	key   string
	str   string
	u32   uint32
	u64   uint64
	items []*vdfNode
}

func decodeVDF(b []byte) (*vdfNode, error) {
	r := bytes.NewReader(b)
	readStr := func() (string, error) {
		var s []byte
		for {
			c, err := r.ReadByte()
			if err != nil {
				return "", err
			}
			if c == 0 {
				return string(s), nil
			}
			s = append(s, c)
		}
	}
	var walk func(n *vdfNode) error
	walk = func(n *vdfNode) error {
		for {
			t, err := r.ReadByte()
			if err != nil {
				return err
			}
			if t == 0x08 {
				return nil
			}
			k, err := readStr()
			if err != nil {
				return err
			}
			c := &vdfNode{typ: t, key: k}
			switch t {
			case 0x00:
				if err := walk(c); err != nil {
					return err
				}
			case 0x01:
				if c.str, err = readStr(); err != nil {
					return err
				}
			case 0x02:
				if err := binary.Read(r, binary.LittleEndian, &c.u32); err != nil {
					return err
				}
			case 0x07:
				if err := binary.Read(r, binary.LittleEndian, &c.u64); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unknown VDF type %#x", t)
			}
			n.items = append(n.items, c)
		}
	}
	root := &vdfNode{}
	if err := walk(root); err != nil {
		return nil, err
	}
	if r.Len() != 0 {
		return nil, errors.New("data after the end of Steam's shortcut list")
	}
	return root, nil
}

func encodeVDF(n *vdfNode, out *bytes.Buffer) {
	for _, c := range n.items {
		out.WriteByte(c.typ)
		out.WriteString(c.key)
		out.WriteByte(0)
		switch c.typ {
		case 0x00:
			encodeVDF(c, out)
		case 0x01:
			out.WriteString(c.str)
			out.WriteByte(0)
		case 0x02:
			_ = binary.Write(out, binary.LittleEndian, c.u32)
		case 0x07:
			_ = binary.Write(out, binary.LittleEndian, c.u64)
		}
	}
	out.WriteByte(0x08)
}

// repointShortcut changes the Exe and StartDir of the entry with this app ID.
func repointShortcut(file []byte, appID uint32, exe, dir string) ([]byte, error) {
	root, err := decodeVDF(file)
	if err != nil {
		return nil, err
	}
	// re-encoding must give the same bytes back, or NEXUS does not understand this list
	var check bytes.Buffer
	encodeVDF(root, &check)
	if !bytes.Equal(check.Bytes()[:check.Len()-1], file) && !bytes.Equal(check.Bytes(), file) {
		return nil, errors.New("Steam's shortcut list has a layout NEXUS does not recognise; nothing was changed")
	}
	found := false
	for _, top := range root.items {
		if top.typ != 0x00 || !strings.EqualFold(top.key, "shortcuts") {
			continue
		}
		for _, e := range top.items {
			isIt := false
			for _, f := range e.items {
				if f.typ == 0x02 && strings.EqualFold(f.key, "appid") && f.u32 == appID {
					isIt = true
				}
			}
			if !isIt {
				continue
			}
			for _, f := range e.items {
				if f.typ == 0x01 && strings.EqualFold(f.key, "Exe") {
					f.str = exe
				}
				if f.typ == 0x01 && strings.EqualFold(f.key, "StartDir") {
					f.str = dir
				}
			}
			found = true
		}
	}
	if !found {
		return nil, errors.New("NEXUS's entry was not found in Steam's shortcut list")
	}
	var out bytes.Buffer
	encodeVDF(root, &out)
	b := out.Bytes()
	if !bytes.Equal(check.Bytes(), file) { // the file had no outer terminator
		b = b[:len(b)-1]
	}
	return b, nil
}

// RepointSteamEntry points NEXUS's Steam entries that start another (older) NEXUS at this one.
func (a *App) RepointSteamEntry() (int, error) {
	st := a.SteamStatus()
	if !st.Supported {
		return 0, errors.New("This is for Linux and the Steam Deck.")
	}
	if steamRunning() {
		return 0, errors.New("Close Steam first: Steam keeps its shortcut list in memory and writes it back when it exits. In Desktop Mode: Steam › Exit. Then try again and start Steam afterwards.")
	}
	exe, dir := `"`+st.Launcher+`"`, `"`+filepath.Dir(st.Launcher)+`"`
	n := 0
	for _, u := range steamUserdata() {
		path := filepath.Join(u, "config", "shortcuts.vdf")
		old, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		data := old
		changed := 0
		for _, sc := range parseShortcuts(old) {
			if !isNexusShortcut(sc, st.Launcher) || strings.Trim(sc.exe, `"`) == st.Launcher {
				continue
			}
			next, err := repointShortcut(data, sc.appID, exe, dir)
			if err != nil {
				return n, err
			}
			data, changed = next, changed+1
		}
		if changed == 0 {
			continue
		}
		backup := path + ".nexus-backup-" + time.Now().Format("20060102-150405")
		if err := os.WriteFile(backup, old, 0o644); err != nil {
			return n, err
		}
		if err := writeFileAtomic(path, data); err != nil {
			return n, err
		}
		back, _ := os.ReadFile(path)
		before, after := parseShortcuts(old), parseShortcuts(back)
		ok := len(before) == len(after)
		for i := range before {
			if !ok {
				break
			}
			if before[i].appID != after[i].appID || before[i].name != after[i].name {
				ok = false
			}
			if isNexusShortcut(before[i], st.Launcher) && strings.Trim(before[i].exe, `"`) != st.Launcher {
				ok = ok && after[i].exe == exe
			} else {
				ok = ok && after[i].exe == before[i].exe
			}
		}
		if !ok {
			_ = os.WriteFile(path, old, 0o644)
			return n, errors.New("Steam's shortcut list did not read back as expected; the original was restored")
		}
		n += changed
		a.Log.Printf("steam: %d NEXUS entr%s of Steam user %s now start %s (backup %s)", changed, map[bool]string{true: "y", false: "ies"}[changed == 1], filepath.Base(u), st.Launcher, backup)
	}
	return n, nil
}
