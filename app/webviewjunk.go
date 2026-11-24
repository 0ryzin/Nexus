package main

import (
	"os"
	"path/filepath"
	"unicode/utf16"
	"unicode/utf8"
)

// misnamedWebViewFolder reports whether name looks like the folder that NEXUS 0.8.x–0.9.2
// could create next to NEXUS.exe on Windows: WebView2's data path read as UTF-16 after the
// path text was reused (every character outside ASCII) and holding only WebView2's cache.
func misnamedWebViewFolder(dir, name string) bool {
	if name == "" || !utf8.ValidString(name) {
		return false
	}
	for _, r := range name {
		if r < 0x80 {
			return false // a real folder name with any ASCII in it is left alone
		}
	}
	if !looksLikePathAsUTF16(name) {
		return false // real words in Chinese or Japanese: not the damaged path
	}
	entries, err := os.ReadDir(filepath.Join(dir, name))
	if err != nil || len(entries) == 0 {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() != "EBWebView" {
			return false // anything other than WebView2's own folder: not ours
		}
	}
	return true
}

// removeMisnamedWebViewFolders deletes such folders from the NEXUS folder. They hold only
// the browser cache of the NEXUS window (no saves, settings or account data).
func removeMisnamedWebViewFolders(a *App) {
	dir := a.Paths.Root
	if dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && misnamedWebViewFolder(dir, e.Name()) {
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err == nil {
				a.Log.Printf("startup: removed a misnamed WebView2 cache folder left by an earlier version")
			} else {
				a.Log.Printf("startup: could not remove a misnamed WebView2 cache folder: %v", err)
			}
		}
	}
}

// looksLikePathAsUTF16 reports whether name, read back as the bytes of its UTF-16 code units,
// is ASCII path text (the damaged folder seen on 24 Nov decodes to "…sers\\tmill\\AppData\\
// Roaming…"). Real Chinese or Japanese words do not decode to printable ASCII with a backslash.
func looksLikePathAsUTF16(name string) bool {
	var b []byte
	for _, u := range utf16.Encode([]rune(name)) {
		b = append(b, byte(u), byte(u>>8))
	}
	printable, slash := 0, false
	for _, c := range b {
		if c >= 0x20 && c < 0x7f {
			printable++
		}
		if c == '\\' || c == '/' {
			slash = true
		}
	}
	return slash && len(b) >= 6 && printable*10 >= len(b)*9
}
