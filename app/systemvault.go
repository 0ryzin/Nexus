package main

// The system-files vault: the user's console keys and firmware kept in one permanent place per
// computer user, independent of where NEXUS is extracted and of portable mode.
//
//   Windows:      %LOCALAPPDATA%\NEXUS\SystemFiles\{Keys,Firmware}
//   Linux/SteamOS: $XDG_DATA_HOME/NEXUS-SystemFiles (default ~/.local/share/NEXUS-SystemFiles)
//   macOS:        ~/Library/Application Support/NEXUS-SystemFiles
//
// Why: NEXUS normally keeps its data in %APPDATA%\NEXUS, which survives new versions, but a
// portable copy (portable.txt) keeps everything inside its own folder, so extracting a new
// version elsewhere started without keys and firmware. Now, at start:
//   1. files installed in this copy (data folder) that the vault lacks, or that are newer than
//      the vault's, are copied into the vault (never overwriting a newer vault file);
//   2. files the vault has and this copy lacks are copied into this copy;
//   3. if the vault is empty, other NEXUS copies next to this one (sibling folders with
//      portable.txt, as left by earlier extractions) are searched once and their keys and
//      firmware are taken from the newest one into the vault.
// Nothing is deleted, nothing is overwritten with an older file, and the files never leave the
// computer (they are not part of account synchronisation).

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var vaultMu sync.Mutex

func systemVaultDir() string {
	if d := os.Getenv("NEXUS_SYSTEM_FILES"); d != "" {
		return d
	}
	switch runtime.GOOS {
	case "windows":
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, "NEXUS", "SystemFiles")
		}
	case "darwin":
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, "Library", "Application Support", "NEXUS-SystemFiles")
		}
	}
	if xd := os.Getenv("XDG_DATA_HOME"); xd != "" && filepath.IsAbs(xd) {
		return filepath.Join(xd, "NEXUS-SystemFiles")
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".local", "share", "NEXUS-SystemFiles")
	}
	return ""
}

// vaultFile: the system files NEXUS keeps (keys folder: prod/title keys, amiibo key_retail.bin).
func isKeyFile(name string) bool {
	n := strings.ToLower(name)
	return strings.HasSuffix(n, ".keys") || n == "key_retail.bin"
}

// syncNewer copies files matching keep from src to dst when dst lacks them or has an older copy.
func syncNewer(src, dst string, keep func(string) bool) (n int) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if e.IsDir() || !keep(e.Name()) {
			continue
		}
		si, err := e.Info()
		if err != nil || si.Size() == 0 {
			continue
		}
		target := filepath.Join(dst, e.Name())
		if di, err := os.Stat(target); err == nil && !si.ModTime().After(di.ModTime()) {
			continue // the destination is as new or newer
		}
		_ = os.MkdirAll(dst, 0o755)
		if copyFileAtomic(filepath.Join(src, e.Name()), target) == nil {
			_ = os.Chtimes(target, time.Now(), si.ModTime())
			n++
		}
	}
	return n
}

func isFirmwareFile(name string) bool { return strings.HasSuffix(strings.ToLower(name), ".nca") }

// SyncSystemVault runs the steps above. Returns what was copied, for the log.
func (e *Emulator) SyncSystemVault() (toVault, fromVault int, found string) {
	vaultMu.Lock() // one copy at a time (start-up, key installs and firmware installs can overlap)
	defer vaultMu.Unlock()
	vault := systemVaultDir()
	if vault == "" {
		return
	}
	vk, vf := filepath.Join(vault, "Keys"), filepath.Join(vault, "Firmware")
	keys, fw := e.KeysDir(), e.FirmwareDir()
	if !fileExists(filepath.Join(vk, "prod.keys")) && !e.KeysPresent() {
		if other := e.findOtherCopy(); other != "" {
			found = other
			toVault += syncNewer(filepath.Join(other, "keys"), vk, isKeyFile)
			toVault += syncNewer(filepath.Join(other, "nand", "system", "Contents", "registered"), vf, isFirmwareFile)
		}
	}
	toVault += syncNewer(keys, vk, isKeyFile)
	fromVault += syncNewer(vk, keys, isKeyFile)
	// firmware: as a whole set (mixing two firmware versions would break it)
	if dirHasFiles(fw) && !dirHasFiles(vf) {
		toVault += syncNewer(fw, vf, isFirmwareFile)
	} else if !dirHasFiles(fw) && dirHasFiles(vf) {
		fromVault += syncNewer(vf, fw, isFirmwareFile)
	}
	return
}

// findOtherCopy looks for an earlier NEXUS copy with keys next to this one (one folder up,
// sibling folders and their direct subfolders) and returns its engine data folder.
func (e *Emulator) findOtherCopy() string {
	parent := filepath.Dir(e.app.Paths.Root)
	var best string
	var bestTime time.Time
	check := func(dir string) {
		if filepath.Clean(dir) == filepath.Clean(e.app.Paths.Root) {
			return
		}
		data := filepath.Join(dir, "System", "Engine")
		if fi, err := os.Stat(filepath.Join(data, "keys", "prod.keys")); err == nil && fi.ModTime().After(bestTime) {
			best, bestTime = data, fi.ModTime()
		}
	}
	entries, _ := os.ReadDir(parent)
	for _, s := range entries {
		if !s.IsDir() {
			continue
		}
		dir := filepath.Join(parent, s.Name())
		check(dir)
		sub, _ := os.ReadDir(dir)
		for _, x := range sub {
			if x.IsDir() {
				check(filepath.Join(dir, x.Name()))
			}
		}
	}
	return best
}

// SaveFirmwareToVault replaces the vault's firmware with this copy's (after the user installed
// a firmware version): the new set is copied first, the vault's previous set is removed only
// after that succeeded.
func (e *Emulator) SaveFirmwareToVault() error {
	vaultMu.Lock()
	defer vaultMu.Unlock()
	vault := systemVaultDir()
	if vault == "" || !dirHasFiles(e.FirmwareDir()) {
		return nil
	}
	vf := filepath.Join(vault, "Firmware")
	tmp := vf + ".new"
	_ = os.RemoveAll(tmp)
	if n := syncNewer(e.FirmwareDir(), tmp, isFirmwareFile); n == 0 {
		return nil
	}
	old := vf + ".old"
	_ = os.RemoveAll(old)
	if dirHasFiles(vf) {
		if err := os.Rename(vf, old); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, vf); err != nil {
		_ = os.Rename(old, vf)
		return err
	}
	return os.RemoveAll(old)
}
