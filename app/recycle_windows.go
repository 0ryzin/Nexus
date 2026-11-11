//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

// recycleFile moves a file to the Recycle Bin, so a capture deleted by mistake can be restored.
func recycleFile(path string) error {
	type shFileOpStruct struct {
		Hwnd                  uintptr
		WFunc                 uint32
		PFrom                 *uint16
		PTo                   *uint16
		FFlags                uint16
		FAnyOperationsAborted int32
		HNameMappings         uintptr
		LpszProgressTitle     *uint16
	}
	from := utf16z(path + "\x00") // double NUL-terminated list
	op := shFileOpStruct{WFunc: 3 /*FO_DELETE*/, PFrom: &from[0],
		FFlags: 0x0040 | 0x0010 | 0x0004 | 0x0400 /*ALLOWUNDO|NOCONFIRMATION|SILENT|NOERRORUI*/}
	r, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("SHFileOperationW").Call(uintptr(unsafe.Pointer(&op)))
	if r != 0 || op.FAnyOperationsAborted != 0 {
		return fmt.Errorf("the file could not be moved to the Recycle Bin (error %d)", r)
	}
	return nil
}
