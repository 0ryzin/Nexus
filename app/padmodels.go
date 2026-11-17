package main

import (
	"fmt"
	"strings"
)

// Controller identification (docs/CONTROLLERS.md). A controller is named from its USB/Bluetooth
// vendor and product IDs, which identify the model; SDL's type is the fallback, and a generic
// name is used when neither says more. Nothing is guessed:
//   - The Xbox Series X and Series S ship the same controller (model 1914), with the same IDs, so
//     it is named "Xbox Series X|S Controller".
//   - A controller seen only through XInput (SDL path "XInput#n") reports the Xbox 360 IDs
//     whatever it is; it is named "Xbox controller (XInput)", not "Xbox 360 Controller".
//   - The browser's Gamepad API that drives NEXUS menus calls every XInput pad "Xbox 360
//     Controller"; NEXUS shows the name from the engine's detection instead (web/pads.js).

// PadModel: what a controller is, for names and button prompts.
type PadModel struct {
	Model  string `json:"model"`  // e.g. xbox-series, dualsense, switch-pro
	Name   string `json:"name"`   // shown to people
	Family string `json:"family"` // xbox | playstation | nintendo | steam | generic (button prompts)
}

type padID struct{ vendor, product int }

const (
	vendorMicrosoft = 0x045E
	vendorSony      = 0x054C
	vendorNintendo  = 0x057E
	vendorValve     = 0x28DE
)

// knownPads: model names by vendor and product ID (from SDL's usb_ids.h and the devices' own IDs).
var knownPads = map[padID]PadModel{
	{vendorMicrosoft, 0x028E}: {"xbox-360", "Xbox 360 Controller", "xbox"},
	{vendorMicrosoft, 0x028F}: {"xbox-360", "Xbox 360 Wireless Controller", "xbox"},
	{vendorMicrosoft, 0x0719}: {"xbox-360", "Xbox 360 Wireless Controller", "xbox"},
	{vendorMicrosoft, 0x02D1}: {"xbox-one", "Xbox One Controller", "xbox"},
	{vendorMicrosoft, 0x02DD}: {"xbox-one", "Xbox One Controller", "xbox"},
	{vendorMicrosoft, 0x02EA}: {"xbox-one-s", "Xbox One S Controller", "xbox"},
	{vendorMicrosoft, 0x02E0}: {"xbox-one-s", "Xbox One S Controller", "xbox"},
	{vendorMicrosoft, 0x02FD}: {"xbox-one-s", "Xbox One S Controller", "xbox"},
	{vendorMicrosoft, 0x0B20}: {"xbox-one-s", "Xbox One S Controller", "xbox"},
	{vendorMicrosoft, 0x02E3}: {"xbox-elite", "Xbox Elite Controller", "xbox"},
	{vendorMicrosoft, 0x0B00}: {"xbox-elite-2", "Xbox Elite Controller Series 2", "xbox"},
	{vendorMicrosoft, 0x0B05}: {"xbox-elite-2", "Xbox Elite Controller Series 2", "xbox"},
	{vendorMicrosoft, 0x0B22}: {"xbox-elite-2", "Xbox Elite Controller Series 2", "xbox"},
	{vendorMicrosoft, 0x0B0A}: {"xbox-adaptive", "Xbox Adaptive Controller", "xbox"},
	{vendorMicrosoft, 0x0B21}: {"xbox-adaptive", "Xbox Adaptive Controller", "xbox"},
	{vendorMicrosoft, 0x0B12}: {"xbox-series", "Xbox Series X|S Controller", "xbox"},
	{vendorMicrosoft, 0x0B13}: {"xbox-series", "Xbox Series X|S Controller", "xbox"},
	{vendorSony, 0x0268}:      {"dualshock3", "PlayStation 3 Controller (DualShock 3)", "playstation"},
	{vendorSony, 0x05C4}:      {"dualshock4", "PlayStation 4 Controller (DualShock 4)", "playstation"},
	{vendorSony, 0x09CC}:      {"dualshock4", "PlayStation 4 Controller (DualShock 4)", "playstation"},
	{vendorSony, 0x0BA0}:      {"dualshock4", "PlayStation 4 Controller (DualShock 4)", "playstation"},
	{vendorSony, 0x0CE6}:      {"dualsense", "PlayStation 5 Controller (DualSense)", "playstation"},
	{vendorSony, 0x0DF2}:      {"dualsense-edge", "PlayStation 5 Controller (DualSense Edge)", "playstation"},
	{vendorNintendo, 0x2009}:  {"switch-pro", "Nintendo Switch Pro Controller", "nintendo"},
	{vendorNintendo, 0x2006}:  {"joycon", "Joy-Con (L)", "nintendo"},
	{vendorNintendo, 0x2007}:  {"joycon", "Joy-Con (R)", "nintendo"},
	{vendorValve, 0x1205}:     {"steam-deck", "Steam Deck controls", "steam"},
	{vendorValve, 0x1206}:     {"steam-deck", "Steam Deck controls", "steam"},
}

// identifyPad names a controller. path is SDL's device path ("XInput#0" for XInput-only).
func identifyPad(vendor, product int, typ, path, sdlName string) PadModel {
	xinputOnly := strings.HasPrefix(strings.ToLower(path), "xinput#")
	if m, ok := knownPads[padID{vendor, product}]; ok {
		if xinputOnly && m.Model == "xbox-360" {
			return PadModel{"xbox", "Xbox controller (XInput)", "xbox"}
		}
		return m
	}
	switch typ {
	case "xbox360":
		if xinputOnly {
			return PadModel{"xbox", "Xbox controller (XInput)", "xbox"}
		}
		return PadModel{"xbox", "Xbox-compatible controller", "xbox"}
	case "xboxone":
		return PadModel{"xbox", "Xbox-compatible controller", "xbox"}
	case "ps3", "ps4", "ps5":
		return PadModel{"playstation", "PlayStation-compatible controller", "playstation"}
	case "switch-pro", "joycon-left", "joycon-right", "joycon-pair":
		return PadModel{"nintendo", "Nintendo-style controller", "nintendo"}
	}
	switch vendor {
	case vendorMicrosoft:
		return PadModel{"xbox", "Xbox controller", "xbox"}
	case vendorSony:
		return PadModel{"playstation", "PlayStation controller", "playstation"}
	case vendorNintendo:
		return PadModel{"nintendo", "Nintendo-style controller", "nintendo"}
	}
	if n := strings.TrimSpace(sdlName); n != "" && !strings.Contains(strings.ToLower(n), "xbox 360") {
		return PadModel{"generic", n, "generic"}
	}
	return PadModel{"generic", "Game controller", "generic"}
}

// PadModels: the table for the interface (web/pads.js), keyed "vvvv:pppp".
func PadModels() map[string]PadModel {
	out := make(map[string]PadModel, len(knownPads))
	for id, m := range knownPads {
		out[fmt.Sprintf("%04x:%04x", id.vendor, id.product)] = m
	}
	return out
}
