package main

import "testing"

func TestHUDGeometry(t *testing.T) {
	// a 1920x1200 window: 16:9 picture with bars top and bottom
	img := gameImageRect(pxRect{100, 50, 2020, 1250}, "16:9")
	if img != (pxRect{100, 110, 2020, 1190}) {
		t.Fatalf("16:9 in 16:10: %+v", img)
	}
	// ultrawide window: bars left and right
	if img := gameImageRect(pxRect{0, 0, 3440, 1440}, "16:9"); img != (pxRect{440, 0, 3000, 1440}) {
		t.Fatalf("16:9 in 21:9: %+v", img)
	}
	if img := gameImageRect(pxRect{0, 0, 800, 600}, "stretch"); img != (pxRect{0, 0, 800, 600}) {
		t.Fatal("stretch")
	}
	if img := gameImageRect(pxRect{0, 0, 1280, 800}, "unknown"); img.H() != 720 {
		t.Fatal("unknown aspect should be 16:9")
	}
	for _, c := range []struct {
		h     int32
		scale int
		want  int32
	}{{800, 100, 25}, {1080, 100, 34}, {2160, 100, 48}, {300, 100, 22}, {1080, 200, 69}} {
		if got := hudBarHeight(pxRect{0, 0, 10, c.h}, c.scale); got != c.want {
			t.Errorf("height %d @%d%%: %d, want %d", c.h, c.scale, got, c.want)
		}
	}
}

func TestHUDSegments(t *testing.T) {
	fps, cpu, ct, ram, rt := 59.6, 37.0, 61.0, 6144.0, 16384.0
	s := PerfSample{FPS: &fps, CPU: &cpu, CPUTemp: &ct, RAMUsedMB: &ram, RAMMB: &rt}
	got := perfSegments(s, []string{"fps", "frametime", "cpu", "cputemp", "gpu", "ram"})
	want := []PerfSeg{{"fps", "FPS", "60", ""}, {"frame", "FRAME", "N/A", ""}, {"cpu", "CPU", "37", "%"}, {"temp", "", "61", "°C"}, {"gpu", "GPU", "N/A", ""}, {"mem", "RAM", "6.0", "/ 16 GB"}}
	if len(got) != len(want) {
		t.Fatalf("%+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: %+v want %+v", i, got[i], want[i])
		}
	}
	// a temperature that cannot be measured is shown as N/A on its own, never invented
	if g := perfSegments(PerfSample{CPU: &cpu}, []string{"cpu", "cputemp"}); len(g) != 2 || g[1].Value != "N/A" || g[1].Label != "CPU" {
		t.Fatalf("%+v", g)
	}
}
