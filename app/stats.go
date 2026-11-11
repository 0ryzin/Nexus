package main

import "sort"

// ProfileSummary is everything the profile page shows.
type GameStatView struct {
	Key        string           `json:"key"`
	Title      string           `json:"title"`
	Seconds    int64            `json:"seconds"`
	Sessions   int              `json:"sessions"`
	Launches   int              `json:"launches"`
	LastPlayed int64            `json:"lastPlayed"`
	FirstPlay  int64            `json:"firstPlayed,omitempty"`
	ByEmulator map[string]int64 `json:"byEmulator,omitempty"`
	Last       *SessionRec      `json:"lastSession,omitempty"`
	History    []SessionRec     `json:"history,omitempty"`
	InLibrary  bool             `json:"inLibrary"`
}

type ProfileSummary struct {
	ProfileID    string           `json:"profileId"`
	TotalSeconds int64            `json:"totalSeconds"`
	GamesPlayed  int              `json:"gamesPlayed"`
	Sessions     int              `json:"sessions"`
	LastPlayed   int64            `json:"lastPlayed"`
	LastGame     string           `json:"lastGame,omitempty"`
	Recent       []GameStatView   `json:"recent"`
	MostPlayed   []GameStatView   `json:"mostPlayed"`
	Games        []GameStatView   `json:"games"` // alphabetical
	ByEmulator   map[string]int64 `json:"byEmulator"`
	LiveSession  *LiveSession     `json:"live,omitempty"`
}

func (a *App) ProfileSummary(id string) ProfileSummary {
	st := a.Profiles.Stats(id)
	out := ProfileSummary{ProfileID: id, ByEmulator: map[string]int64{}}
	var all []GameStatView
	for k, g := range st.Games {
		v := GameStatView{Key: k, Title: g.Title, Seconds: g.Seconds, Sessions: g.Sessions, Launches: g.Launches,
			LastPlayed: g.LastPlayed, FirstPlay: g.FirstPlay, ByEmulator: g.ByEmulator, Last: g.Last, History: g.History}
		if lg, ok := a.Library.Get(k); ok {
			v.InLibrary = true
			v.Title = lg.Title
		}
		if v.Title == "" {
			v.Title = "Unknown game"
		}
		// older stats (before sessions were counted) still count as played
		if v.Sessions == 0 && v.Seconds > 0 {
			v.Sessions = 1
		}
		out.TotalSeconds += v.Seconds
		out.Sessions += v.Sessions
		if v.Seconds > 0 || v.Sessions > 0 {
			out.GamesPlayed++
		}
		for e, s := range v.ByEmulator {
			out.ByEmulator[e] += s
		}
		if v.LastPlayed > out.LastPlayed {
			out.LastPlayed, out.LastGame = v.LastPlayed, v.Title
		}
		all = append(all, v)
	}
	recent := append([]GameStatView{}, all...)
	sort.Slice(recent, func(i, j int) bool { return recent[i].LastPlayed > recent[j].LastPlayed })
	for _, v := range recent {
		if v.LastPlayed > 0 && len(out.Recent) < 8 {
			out.Recent = append(out.Recent, v)
		}
	}
	most := append([]GameStatView{}, all...)
	sort.Slice(most, func(i, j int) bool { return most[i].Seconds > most[j].Seconds })
	for _, v := range most {
		if v.Seconds > 0 && len(out.MostPlayed) < 8 {
			out.MostPlayed = append(out.MostPlayed, v)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Title < all[j].Title })
	out.Games = all
	if out.Recent == nil {
		out.Recent = []GameStatView{}
	}
	if out.MostPlayed == nil {
		out.MostPlayed = []GameStatView{}
	}
	if out.Games == nil {
		out.Games = []GameStatView{}
	}
	if es := a.Emu.State(); es.Running && es.ProfileID == id {
		var ls LiveSession
		if readJSON(a.Profiles.livePath(), &ls) == nil {
			out.LiveSession = &ls
		}
	}
	return out
}
