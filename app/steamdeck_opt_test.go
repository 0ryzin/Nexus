package main

import "testing"

func TestSteamDeckSwitch2Tuning(t *testing.T) {
	deck := Hardware{GPUs: []GPUInfo{{VendorID: vendorAMD, DeviceID: 0x163f, Model: steamDeckGPUs[0x163f], Kind: "integrated", Integrated: true, Primary: true}},
		Memory: MemoryInfo{TotalMB: 14800}, Displays: []Display{{Width: 1280, Height: 800, RefreshHz: 60, Primary: true}}}
	if !isSteamDeckHW(deck) || isSteamDeckHW(Hardware{GPUs: []GPUInfo{{VendorID: vendorAMD, DeviceID: 0x73df}}}) {
		t.Fatal("Steam Deck detection")
	}
	for _, p := range []OptProfile{OptPerformance, OptBalanced} {
		v, why := recommend(deck, classifyDevice(deck), p, "heavy")
		switch2Limits(v, why)
		steamDeckSwitch2(v, why, p)
		want := map[OptProfile]string{OptPerformance: "0.5", OptBalanced: "0.75"}[p]
		if v["system.docked"] != "handheld" || v["graphics.scale"] != want || v["graphics.fps_limit"] != "30" || v["adv.reactive_flushing"] != "true" || v["adv.vram"] != "conservative" {
			t.Fatalf("%s: %v", p, v)
		}
		for id, val := range v {
			if why[id] == "" {
				t.Fatalf("%s=%s without a reason", id, val)
			}
			if _, _, ok := engineValue(id, val); !ok && engineBinds[id].key != "" {
				t.Fatalf("%s=%s is not a value the engine takes", id, val)
			}
		}
	}
}
