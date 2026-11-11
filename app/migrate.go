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
