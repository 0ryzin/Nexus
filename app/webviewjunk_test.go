package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMisnamedWebViewFolder(t *testing.T) {
	dir := t.TempDir()
	mk := func(p string) {
		if err := os.MkdirAll(filepath.Join(dir, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	junk := asUTF16Name(`C:\Users\tmill\AppData\Roaming\NEXUS\WebView`)
	mk(filepath.Join(junk, "EBWebView", "Default"))
	mk(filepath.Join("Games", "EBWebView")) // ASCII name: never touched
	mk(filepath.Join("ゲーム", "Saves"))       // non-ASCII but not WebView2's
	mk(filepath.Join("游戏", "EBWebView"))
	mk(filepath.Join("设置", "EBWebView")) // real Chinese word, only WebView2's folder: kept    // plus a file next to it below
	os.WriteFile(filepath.Join(dir, "游戏", "save.dat"), []byte("x"), 0o644)
	if !misnamedWebViewFolder(dir, junk) {
		t.Fatal("junk folder not recognised")
	}
	for _, n := range []string{"Games", "ゲーム", "游戏", "设置"} {
		if misnamedWebViewFolder(dir, n) {
			t.Fatalf("%s would be deleted", n)
		}
	}
}

// asUTF16Name: ASCII text read as UTF-16 code units, as WebView2 did with the damaged path
func asUTF16Name(s string) string {
	var r []rune
	for i := 0; i+1 < len(s); i += 2 {
		r = append(r, rune(s[i])|rune(s[i+1])<<8)
	}
	return string(r)
}
