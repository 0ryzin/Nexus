package main

// Play statistics are kept per library key. The same game file can have had another key in the
// past: "f-…" (from its file name) while NEXUS could not read its Title ID - before keys were
// installed, or before NEXUS could read that package format (NUP, NEXUS 0.3.0) - and "t-<Title
// ID>" afterwards. The user then saw the game twice in their statistics. After each library
// scan the old file-name entries of games that are now identified are merged into the game's
// entry. Only that exact mapping is used (same file name -> its current key): different games,
// regional versions and editions have different Title IDs and are never merged.

import (
	"sort"
	"strings"
)

// statAliases maps an old file-name key to the current key of the game with that file.
func (l *Library) statAliases() map[string]string {
	out := map[string]string{}
	for _, g := range l.Games() {
		if g.Path == "" || !strings.HasPrefix(g.Key, "t-") && !strings.HasPrefix(g.Key, "x-") {
			continue
		}
		if old := gameKey(GameInfo{}, g.Path); old != g.Key {
			out[old] = g.Key
		}
	}
	return out
}

func mergeGameStat(dst, src *GameStat) {
	dst.Seconds += src.Seconds
	dst.Launches += src.Launches
	dst.Sessions += src.Sessions
	if src.LastPlayed > dst.LastPlayed {
		dst.LastPlayed = src.LastPlayed
		if src.Last != nil {
			dst.Last = src.Last
		}
	}
	if src.FirstPlay != 0 && (dst.FirstPlay == 0 || src.FirstPlay < dst.FirstPlay) {
		dst.FirstPlay = src.FirstPlay
	}
	if dst.Title == "" {
		dst.Title = src.Title
	}
	for k, v := range src.ByEmulator {
		if dst.ByEmulator == nil {
			dst.ByEmulator = map[string]int64{}
		}
		dst.ByEmulator[k] += v
	}
	dst.History = append(dst.History, src.History...)
	sort.Slice(dst.History, func(i, j int) bool { return dst.History[i].Start > dst.History[j].Start })
	if len(dst.History) > 200 {
		dst.History = dst.History[:200]
	}
}

// MergeStatAliases folds statistics recorded under old keys into the current ones, for every
// user. Returns how many entries were merged.
func (st *ProfileStore) MergeStatAliases(aliases map[string]string) int {
	if len(aliases) == 0 {
		return 0
	}
	n := 0
	for _, p := range st.List() {
		st.mu.Lock()
		s := st.loadStatsLocked(p.ID)
		changed := false
		for old, cur := range aliases {
			src, ok := s.Games[old]
			if !ok {
				continue
			}
			if dst, ok := s.Games[cur]; ok {
				mergeGameStat(dst, src)
			} else {
				s.Games[cur] = src
			}
			delete(s.Games, old)
			changed = true
			n++
		}
		if changed {
			_ = writeJSONAtomic(st.statsPath(p.ID), s)
		}
		st.mu.Unlock()
	}
	return n
}
