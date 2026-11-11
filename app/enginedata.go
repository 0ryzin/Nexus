package main

// NEXUS engine data: NEXUS creates and owns the engine's data folder (<data>\System\Engine) and
// its configuration. NEXUS never reads another emulator's folders: system files (keys, firmware)
// are installed by the user through Settings › System files, saves through Save Data, and updates
// and mods per game.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// EngineDataStatus describes the NEXUS engine data folder.
type EngineDataStatus struct {
	DataDir string `json:"dataDir"`
	Ready   bool   `json:"ready"` // folder and configuration created
}

func (e *Emulator) EngineDataStatus() EngineDataStatus {
	return EngineDataStatus{DataDir: e.DataDir(), Ready: fileExists(e.ConfigPath())}
}

// EnsureEngineData creates the engine's data folder, its standard sub-folders and the NEXUS
// engine configuration if they are missing. Called at every start; it never changes existing
// files.
func (e *Emulator) EnsureEngineData() error {
	dd := e.DataDir()
	for _, sub := range []string{"", "keys", filepath.Join("nand", "system"), filepath.Join("nand", "user"), "sdmc", "load", "log",
		"config", "dump", "shader", "screenshots", "lossless", "streamline"} {
		if err := os.MkdirAll(filepath.Join(dd, sub), 0o755); err != nil {
			return fmt.Errorf("the NEXUS engine data folder could not be created (%s): %w", dd, err)
		}
	}
	if !fileExists(e.ConfigPath()) {
		ini, err := LoadIni(e.ConfigPath())
		if err != nil {
			return err
		}
		seedNewConfig(ini)
		if err := ini.Save(e.ConfigPath()); err != nil {
			return fmt.Errorf("the NEXUS engine configuration could not be created: %w", err)
		}
		e.app.Log.Printf("engine data: created the NEXUS engine configuration in %s", e.ConfigDir())
	}
	return nil
}

// copyFileTmpRename copies through a temporary file in the destination folder, so a crash never
// leaves a partly written file under the final name.
func copyFileTmpRename(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := to + ".nexus-tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if st, err := in.Stat(); err == nil {
		_ = os.Chtimes(tmp, st.ModTime(), st.ModTime())
		_ = os.Chmod(tmp, st.Mode().Perm())
	}
	return os.Rename(tmp, to)
}

func backupTree(from, to string) error {
	return filepath.Walk(from, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(to, rel), 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFileTmpRename(p, filepath.Join(to, rel))
	})
}
