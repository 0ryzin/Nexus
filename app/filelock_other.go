//go:build !windows

package main

import (
	"errors"
	"os"
	"syscall"
)

// tryLockFile takes the engine's kind of lock (flock, as common/fs/file_lock.cpp) on path without
// waiting. busy is true when another process holds it.
func tryLockFile(path string) (unlock func(), busy bool, err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o666)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.Is(err, syscall.EWOULDBLOCK), err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, false, nil
}
