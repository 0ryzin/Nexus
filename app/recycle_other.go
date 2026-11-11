//go:build !windows

package main

import "os"

func recycleFile(path string) error { return os.RemoveAll(path) }
