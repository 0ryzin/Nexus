//go:build !windows

package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

// discordSocketDirs: where the Discord app puts its socket (native, Flatpak and Snap Discord;
// a Steam Deck usually has the Flatpak one).
func discordSocketDirs() []string {
	var bases []string
	for _, v := range []string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"} {
		if d := os.Getenv(v); d != "" {
			bases = append(bases, d)
		}
	}
	bases = append(bases, "/tmp", fmt.Sprintf("/run/user/%d", os.Getuid()))
	var out []string
	for _, b := range bases {
		out = append(out, b, filepath.Join(b, "app", "com.discordapp.Discord"), filepath.Join(b, "app", "com.discordapp.DiscordCanary"),
			filepath.Join(b, "snap.discord"), filepath.Join(b, ".flatpak", "com.discordapp.Discord", "xdg-run"))
	}
	return out
}

func dialDiscord() (io.ReadWriteCloser, error) {
	for _, dir := range discordSocketDirs() {
		for i := 0; i < 10; i++ {
			c, err := net.DialTimeout("unix", filepath.Join(dir, fmt.Sprintf("discord-ipc-%d", i)), time.Second)
			if err == nil {
				return c, nil
			}
		}
	}
	return nil, fmt.Errorf("Discord is not running")
}
