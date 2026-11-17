//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// tryLockFile takes the engine's kind of lock (LockFileEx on byte 0, as common/fs/file_lock.cpp)
// on path without waiting. busy is true when another process holds it.
func tryLockFile(path string) (unlock func(), busy bool, err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o666)
	if err != nil {
		return nil, false, err
	}
	ol := new(windows.Overlapped)
	err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	if err != nil {
		f.Close()
		return nil, err == windows.ERROR_LOCK_VIOLATION, err
	}
	return func() {
		_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
		f.Close()
	}, false, nil
}
