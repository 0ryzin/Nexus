package main

// SettingsView is what the NEXUS configuration pages show: the NEXUS settings schema, which
// settings the NEXUS engine applies, and the global/per-game values.
type SettingsView struct {
	Sections      []EmuChoice                   `json:"sections"`
	Settings      []NSetting                    `json:"settings"`
	Caps          map[string]map[string]Support `json:"caps"`         // keyed by engine id ("nexus")
	Global        map[string]string             `json:"global"`       // NEXUS global values
	EngineValues  map[string]map[string]string  `json:"engineValues"` // what the engine configuration says
	Engines       []EngineChoice                `json:"engines"`
	DefaultEngine string                        `json:"defaultEngine"`
	Firmware      string                        `json:"firmware"`
	InputProfiles []string                      `json:"inputProfiles"`

	// per-game only
	Game          *Game             `json:"game,omitempty"`
	Overrides     map[string]string `json:"overrides"`
	InputProfile  string            `json:"inputProfile"`
	Emulator      string            `json:"emulator"`
	Effective     string            `json:"effective"`
	Compat        *CompatView       `json:"compat,omitempty"`
	PerGameOK     bool              `json:"perGameOk"`
	PerGameReason string            `json:"perGameReason,omitempty"`
}

type EngineChoice struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	FormatOK  bool   `json:"formatOk"`
	Version   string `json:"version"`
}

func (e *Emulator) baseSettingsView() SettingsView {
	installed, _ := e.EngineAvailable()
	v := SettingsView{Sections: NexusSections, Settings: NexusSettings,
		Caps:   map[string]map[string]Support{engineID: EngineCaps()},
		Global: e.GlobalNexusSettings(), EngineValues: map[string]map[string]string{engineID: e.EngineValues()},
		Engines:       []EngineChoice{{ID: engineID, Name: "NEXUS Engine", Installed: installed, FormatOK: true, Version: e.Builds.ActiveVersionLabel()}},
		DefaultEngine: engineID, InputProfiles: e.InputProfiles()}
	if fi, st := e.app.Sys.FirmwareStatus(); fi != nil && st == "ok" {
		v.Firmware = fi.Version
	}
	return v
}

func (e *Emulator) GlobalSettingsView() SettingsView { return e.baseSettingsView() }

// PerGameSupport says whether per-game settings can be applied to g.
func (e *Emulator) PerGameSupport(g Game) (bool, string) {
	if g.Kind == "update" || g.Kind == "dlc" {
		return false, "Updates and DLC use the base game's settings."
	}
	return true, ""
}

func (e *Emulator) GameSettingsView(g Game) SettingsView {
	v := e.baseSettingsView()
	p := e.app.Library.GP.Get(g.Key)
	gg := g
	v.Game = &gg
	v.Overrides = gameNexusSettings(p)
	if v.Overrides == nil {
		v.Overrides = map[string]string{}
	}
	v.InputProfile = p.InputProfile
	v.Emulator, v.Effective = "", engineID
	c := e.app.Compat.Get(g.Key)
	v.Compat = &c
	v.PerGameOK, v.PerGameReason = e.PerGameSupport(g)
	return v
}
