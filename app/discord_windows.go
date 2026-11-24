package main

import (
	"fmt"
	"io"
	"os"
)

// dialDiscord opens the Discord app's named pipe (discord-ipc-0..9).
func dialDiscord() (io.ReadWriteCloser, error) {
	for i := 0; i < 10; i++ {
		if f, err := os.OpenFile(fmt.Sprintf(`\\.\pipe\discord-ipc-%d`, i), os.O_RDWR, 0); err == nil {
			return f, nil
		}
	}
	return nil, fmt.Errorf("Discord is not running")
}
