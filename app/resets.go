package main

// Restoring defaults (usability update, 17 Nov 2026): the whole of NEXUS's settings, and the
// keyboard shortcuts and controller buttons on their own. Single settings are restored by the
// interface with the normal settings calls (web/settings.js), from the same defaults.
//
// Never touched here: users, saves, games, play time, keys and firmware, engine files, the
// NEXUS account / GitHub sign-in (System/Account), the update access token, artwork and
// wallpaper files, library folders, per-game settings, Home folders, pages, favourites, the
// custom order and hidden games.

import (
	"errors"
	"net/http"
)

// resetGlobalSettings: every NEXUS setting back to its default, keeping what is bookkeeping or
// the user's content rather than a preference.
func resetGlobalSettings(g *GlobalSettings) {
	keep := *g
	*g = DefaultSettings()
	g.LastProfileID = keep.LastProfileID                                        // who used NEXUS last
	g.Library.Dirs = keep.Library.Dirs                                          // where the games are (content, not a preference)
	g.Display.Width, g.Display.Height = keep.Display.Width, keep.Display.Height // the window's last size
	g.Updates.SkipVersion, g.Updates.LastCheck = keep.Updates.SkipVersion, keep.Updates.LastCheck
}

// resetProfilePrefs: one user's appearance, Home screen look and controller settings back to
// the defaults. Their pictures, Home folders and pages, order, favourites and hidden games stay.
func resetProfilePrefs(p *Profile) {
	wp := p.Theme.Wallpaper
	p.Theme = DefaultTheme()
	p.Theme.Wallpaper.File, p.Theme.Wallpaper.Animated, p.Theme.Wallpaper.AnimatedType, p.Theme.Wallpaper.Poster, p.Theme.Wallpaper.Use =
		wp.File, wp.Animated, wp.AnimatedType, wp.Poster, wp.Use
	pref, prefName := p.Controls.PreferredPad, p.Controls.PreferredName
	p.Controls = DefaultControls()
	p.Controls.PreferredPad, p.Controls.PreferredName = pref, prefName // which controller is theirs
	p.Sort = "recent"
	h := p.Home
	p.Home = HomePrefs{Order: h.Order, Favorites: h.Favorites, Hidden: h.Hidden, Folders: h.Folders, Pages: h.Pages}
	normaliseHome(&p.Home)
}

// resetKeybinds: NEXUS's keyboard shortcuts and controller buttons only.
func resetKeybinds(g *GlobalSettings) {
	d := DefaultSettings()
	g.Emulator.HomeKey, g.Emulator.FullscreenKey, g.Emulator.ScreenshotKey = d.Emulator.HomeKey, d.Emulator.FullscreenKey, d.Emulator.ScreenshotKey
	g.Emulator.HomeButton, g.Emulator.CaptureButton, g.Emulator.QuitCombo = d.Emulator.HomeButton, d.Emulator.CaptureButton, d.Emulator.QuitCombo
	g.Recording.Key = d.Recording.Key
}

func resetProfileKeybinds(p *Profile) {
	d := DefaultControls()
	p.Controls.ConfirmButton, p.Controls.Mapping = d.ConfirmButton, d.Mapping
	p.Controls.GameLayout = GameLayout{}
}

// Mutate changes one profile in place (normalised and saved).
func (st *ProfileStore) Mutate(id string, fn func(*Profile)) (Profile, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, p := range st.Profiles {
		if p.ID == id {
			fn(p)
			st.normalise(p)
			return *p, st.saveLocked()
		}
	}
	return Profile{}, errors.New("Profile not found.")
}

// profileDefaults: what a new user starts with, for the interface's "Changed" marks and resets.
func profileDefaults() map[string]any {
	h := HomePrefs{}
	normaliseHome(&h)
	return map[string]any{"theme": DefaultTheme(), "controls": DefaultControls(), "sort": "recent", "home": h}
}

func (s *Server) resetRoutes(m *http.ServeMux) {
	a := s.app
	m.HandleFunc("GET /api/profiles/defaults", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, profileDefaults()) })
	// body: {"profile": "<id>"} also resets that user's preferences
	m.HandleFunc("POST /api/settings/reset-all", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Profile string }
		_ = readBody(r, &b)
		st, err := a.Settings.Update(resetGlobalSettings)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		out := map[string]any{"settings": st}
		if b.Profile != "" {
			p, err := a.Profiles.Mutate(b.Profile, resetProfilePrefs)
			if err != nil {
				writeErr(w, 404, err)
				return
			}
			out["profile"] = p
		}
		a.Log.Printf("settings: all settings restored to their defaults%s", map[bool]string{true: " (with this user's preferences)", false: ""}[b.Profile != ""])
		a.Events.Emit("settings.changed", nil)
		writeJSON(w, out)
	})
	m.HandleFunc("POST /api/keybinds/reset", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Profile string }
		_ = readBody(r, &b)
		st, err := a.Settings.Update(resetKeybinds)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		out := map[string]any{"settings": st}
		if b.Profile != "" {
			p, err := a.Profiles.Mutate(b.Profile, resetProfileKeybinds)
			if err != nil {
				writeErr(w, 404, err)
				return
			}
			out["profile"] = p
		}
		a.Log.Printf("settings: keyboard shortcuts and controller buttons restored to their defaults")
		writeJSON(w, out)
	})
}
