package main

import "testing"

func TestIdentifyPad(t *testing.T) {
	cases := []struct {
		vendor, product int
		typ, path       string
		name, family    string
	}{
		{0x045E, 0x0B12, "xboxone", `\\?\HID#VID_045E&PID_0B12`, "Xbox Series X|S Controller", "xbox"},
		{0x045E, 0x0B13, "xboxone", "", "Xbox Series X|S Controller", "xbox"},
		{0x045E, 0x02EA, "xboxone", "", "Xbox One S Controller", "xbox"},
		{0x045E, 0x028E, "xbox360", "", "Xbox 360 Controller", "xbox"},
		// XInput-only devices all report the Xbox 360 IDs: not named a 360 controller
		{0x045E, 0x028E, "xbox360", "XInput#0", "Xbox controller (XInput)", "xbox"},
		{0x054C, 0x0CE6, "ps5", "", "PlayStation 5 Controller (DualSense)", "playstation"},
		{0x054C, 0x09CC, "ps4", "", "PlayStation 4 Controller (DualShock 4)", "playstation"},
		{0x057E, 0x2009, "switch-pro", "", "Nintendo Switch Pro Controller", "nintendo"},
		{0x0F0D, 0x00C1, "ps4", "", "PlayStation-compatible controller", "playstation"},
		{0x1234, 0x5678, "standard", "", "Fancy Pad", "generic"},
	}
	for _, c := range cases {
		m := identifyPad(c.vendor, c.product, c.typ, c.path, "Fancy Pad")
		if m.Name != c.name || m.Family != c.family {
			t.Errorf("%04x:%04x %s %q: got %q/%s", c.vendor, c.product, c.typ, c.path, m.Name, m.Family)
		}
	}
	if m := identifyPad(0x1234, 0x5678, "standard", "", "Xbox 360 Controller"); m.Name != "Game controller" {
		t.Errorf("a generic pad calling itself Xbox 360: %q", m.Name)
	}
	if len(PadModels()) != len(knownPads) {
		t.Error("PadModels")
	}
}
