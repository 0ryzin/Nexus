package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func vdfStr(b *bytes.Buffer, t byte, k, v string) {
	b.WriteByte(t)
	b.WriteString(k)
	b.WriteByte(0)
	b.WriteString(v)
	b.WriteByte(0)
}

func TestParseSteamShortcuts(t *testing.T) {
	var b bytes.Buffer
	b.WriteByte(0)
	b.WriteString("shortcuts\x00")
	for i, sc := range []struct {
		id        uint32
		name, exe string
	}{{3123456789, "NEXUS", `"/home/deck/Applications/NEXUS-0.7.0-Linux-amd64.AppImage"`}, {42, "Firefox", `"/usr/bin/firefox"`}} {
		b.WriteByte(0)
		b.WriteString(string(rune('0'+i)) + "\x00")
		b.WriteByte(2)
		b.WriteString("appid\x00")
		binary.Write(&b, binary.LittleEndian, sc.id)
		vdfStr(&b, 1, "AppName", sc.name)
		vdfStr(&b, 1, "Exe", sc.exe)
		b.WriteByte(0)
		b.WriteString("tags\x00")
		vdfStr(&b, 1, "0", "favorite")
		b.WriteByte(8)
		b.WriteByte(8)
	}
	b.WriteByte(8)
	b.WriteByte(8)
	got := parseShortcuts(b.Bytes())
	if len(got) != 2 || got[0].appID != 3123456789 || got[0].name != "NEXUS" || got[1].name != "Firefox" {
		t.Fatalf("%+v", got)
	}
	if !isNexusShortcut(got[0], "/elsewhere/NEXUS.AppImage") || isNexusShortcut(got[1], "/x") {
		t.Fatal("NEXUS shortcut recognition")
	}
	if parseShortcuts([]byte{0, 's', 0, 2}) != nil {
		t.Fatal("truncated file")
	}
	if desktopQuote("/home/deck/My Games/NEXUS.AppImage") != `"/home/deck/My Games/NEXUS.AppImage"` {
		t.Fatal(desktopQuote("/home/deck/My Games/NEXUS.AppImage"))
	}
}
