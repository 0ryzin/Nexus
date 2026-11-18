//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSysfsSteamDeck: a Steam Deck's sysfs (values as its kernel reports them).
func TestPerfSysfsSteamDeck(t *testing.T) {
	root := t.TempDir()
	w := func(rel, v string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(v+"\n"), 0o644)
	}
	dev := "class/drm/card0/device/"
	w(dev+"gpu_busy_percent", "87")
	w(dev+"mem_info_vram_total", "1073741824")
	w(dev+"mem_info_vram_used", "805306368")
	w(dev+"mem_info_gtt_total", "8589934592")
	w(dev+"mem_info_gtt_used", "2147483648")
	w(dev+"pp_dpm_sclk", "0: 200Mhz\n1: 1600Mhz *\n2: 1600Mhz")
	w(dev+"hwmon/hwmon3/temp1_input", "71000")
	w(dev+"hwmon/hwmon3/power1_average", "14200000")
	w("class/drm/card0-eDP-1/device/gpu_busy_percent", "1") // a connector, not a card
	w("class/hwmon/hwmon1/name", "acpitz")
	w("class/hwmon/hwmon1/temp1_input", "40000")
	w("class/hwmon/hwmon2/name", "k10temp")
	w("class/hwmon/hwmon2/temp1_input", "76500")
	w("devices/system/cpu/cpu0/cpufreq/scaling_cur_freq", "2800000")
	w("devices/system/cpu/cpu1/cpufreq/scaling_cur_freq", "3400000")
	w("class/power_supply/BAT1/capacity", "76")
	w("class/power_supply/BAT1/status", "Discharging")
	w("class/power_supply/BAT1/power_now", "17300000")
	var p PerfSample
	sysfsSample(&p, root)
	chk := func(name string, v *float64, want float64) {
		t.Helper()
		if v == nil || *v != want {
			t.Errorf("%s: %v want %v", name, v, want)
		}
	}
	chk("GPU %", p.GPU, 87)
	chk("GPU memory used MB", p.VRAMUsedMB, 768+2048)
	chk("GPU memory total MB", p.VRAMMB, 1024+8192)
	chk("GPU MHz", p.GPUMHz, 1600)
	chk("GPU °C", p.GPUTemp, 71)
	chk("power W", p.PowerW, 14.2)
	chk("CPU °C", p.CPUTemp, 76.5)
	chk("CPU MHz", p.CPUMHz, 3100)
	chk("battery %", p.BatteryPct, 76)
	chk("battery W", p.BatteryW, 17.3)
	segs := perfSegments(p, []string{"clocks", "power", "battery"})
	if len(segs) != 4 || segs[0].Value != "3.1" || segs[1].Value != "1600" || segs[2].Value != "14.2" || segs[3].Unit != "% −17.3 W" {
		t.Fatalf("%+v", segs)
	}
	// nothing there: everything unavailable, nothing invented
	var q PerfSample
	sysfsSample(&q, t.TempDir())
	if q.GPU != nil || q.CPUTemp != nil || q.BatteryPct != nil || q.PowerW != nil {
		t.Fatalf("%+v", q)
	}
}
