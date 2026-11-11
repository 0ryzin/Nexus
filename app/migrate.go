package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// mergeTree copies src into dst, skipping files that already exist in dst.
func mergeTree(src, dst string, skip func(rel string) bool) (int, error) {
	n := 0
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		if rel != "." && skip != nil && skip(rel) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if fileExists(target) {
			return nil
		}
		if err := copyFile(path, target); err != nil {
			return err
		}
		n++
		return nil
	})
	return n, err
}

// migrateToDataDir moves data that older NEXUS builds kept inside the NEXUS
// folder to the per-user data folders, so it survives deleting/updating NEXUS.
// Files are copied (never overwriting) and the old folders renamed, not deleted.
func (a *App) migrateToDataDir() {
	p := a.Paths
	if p.IsPortable {
		return
	}
	stamp := time.Now().Format("20060102-150405")
	old := filepath.Join(p.Root, "System")
	if dirHasFiles(filepath.Join(old, "Profiles")) || dirHasFiles(filepath.Join(old, "Saves")) {
		n, err := mergeTree(old, p.System, func(rel string) bool { return strings.EqualFold(rel, "WebView") })
		if err == nil {
			_ = os.Rename(old, old+".migrated-"+stamp)
			a.Log.Printf("migrate: copied %d files from %s to %s", n, old, p.System)
			a.migrated = append(a.migrated, fmt.Sprintf("Profiles, saves and settings were moved to %s", p.System))
		} else {
			a.Log.Printf("migrate: %v (old data left in place)", err)
		}
	}
	// A portable emulator "user" folder that NEXUS 1.x kept inside its own folder would be lost
	// on update: copy it to that emulator's standard data folder (%APPDATA%\eden) first; the
	// step below then brings it into the NEXUS engine's data folder.
	lu := filepath.Join(p.Root, "Emulator", "Lichii", "user") // NEXUS 1.x layout
	if ad := os.Getenv("APPDATA"); ad != "" && dirExists(lu) {
		hasData := dirHasFiles(filepath.Join(lu, "nand")) || fileExists(filepath.Join(lu, "keys", "prod.keys")) || dirHasFiles(filepath.Join(lu, "config"))
		eden := filepath.Join(ad, "eden")
		if hasData {
			n, err := mergeTree(lu, eden, nil)
			if err != nil {
				a.Log.Printf("migrate old emulator data: %v (left in place)", err)
				return
			}
			a.Log.Printf("migrate: copied %d files from %s to %s", n, lu, eden)
		}
		_ = os.Rename(lu, lu+".migrated-"+stamp)
	}
}

// importPreviousEngineData brings keys, firmware, installed updates/DLC, mods and the game-list
// cache from the emulator data folder NEXUS 1.x used (%APPDATA%\eden) into the NEXUS engine's
// own data folder, once. Files are hard-linked when possible (no extra disk space) or copied;
// nothing is moved or deleted, and existing engine files are never overwritten. Saves are not
// touched here: they are imported per user from Settings, only when the user asks.
func (a *App) importPreviousEngineData() {
	ad := os.Getenv("APPDATA")
	if ad == "" || a.Paths.IsPortable {
		return
	}
	src := filepath.Join(ad, "eden")
	dst := a.Emu.DataDir()
	stamp := filepath.Join(dst, ".imported-previous-data")
	if !dirExists(src) || fileExists(stamp) {
		return
	}
	total := 0
	for _, sub := range []string{"keys", filepath.Join("nand", "system"), filepath.Join("nand", "user", "Contents"), filepath.Join("config", "input"),
		"load", filepath.Join("cache", "game_list")} {
		from := filepath.Join(src, sub)
		if !dirExists(from) {
			continue
		}
		_ = filepath.Walk(from, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(src, p)
			to := filepath.Join(dst, rel)
			if info.IsDir() {
				return os.MkdirAll(to, 0o755)
			}
			if fileExists(to) {
				return nil
			}
			if linkOrCopy(p, to) == nil {
				total++
			}
			return nil
		})
	}
	_ = os.MkdirAll(dst, 0o755)
	_ = os.WriteFile(stamp, []byte(time.Now().Format(time.RFC3339)+"\n"), 0o644)
	if total > 0 {
		a.Log.Printf("migrate: %d files (keys, firmware, installed content, mods) from %s into the NEXUS engine data folder", total, src)
		a.migrated = append(a.migrated, fmt.Sprintf("Your keys, firmware and installed updates were brought into the NEXUS engine (%d files; the originals in %s are untouched).", total, src))
	}
}
