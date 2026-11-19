package main

import (
	"strings"
	"testing"
)

// TestSwitch2MemoryNote: the Deck (16 GB, ~14.5 GB visible) gets an explanation, not an alarm;
// below 12 GB a real warning; 16 GB PCs nothing; shown once per game unless memory is short.
func TestSwitch2MemoryNote(t *testing.T) {
	deck := Hardware{GPUs: []GPUInfo{{VendorID: vendorAMD, DeviceID: 0x163f}}}
	deck.Memory.TotalMB = 14830
	if m := switch2MemoryNote("OoT", deck); !strings.Contains(m, "Steam Deck has 16 GB") || strings.Contains(m, "may close when") {
		t.Fatalf("deck: %q", m)
	}
	small := Hardware{}
	small.Memory.TotalMB = 8 * 1024
	if m := switch2MemoryNote("OoT", small); !strings.Contains(m, "may close when it runs out") {
		t.Fatalf("8 GB: %q", m)
	}
	big := Hardware{}
	big.Memory.TotalMB = 16000
	if m := switch2MemoryNote("OoT", big); m != "" {
		t.Fatalf("16 GB: %q", m)
	}
	a, _ := newTestApp(t, "")
	if !a.Emu.firstMemoryNote("g1") || a.Emu.firstMemoryNote("g1") || !a.Emu.firstMemoryNote("g2") {
		t.Fatal("once per game")
	}
}
