//go:build linux && cgo

package main

// Helpers for enginehost_dl_test.go (cgo cannot be used in _test.go files): they read back
// what the test engine library received.

/*
#include <stdint.h>
#include <dlfcn.h>
#include <stdlib.h>
static const char* nxt_str(void* f) { return ((const char* (*)(void))f)(); }
static int32_t nxt_i32(void* f) { return ((int32_t (*)(void))f)(); }
static void* nxt_ptr(void* f) { return ((void* (*)(void))f)(); }
static void* nxt_sym(void* h, const char* n) { return dlsym(h, n); }
*/
import "C"

import "unsafe"

func dlSymForTest(e *dlEngine, name string) unsafe.Pointer {
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	return C.nxt_sym(e.handle, cn)
}
func callStrForTest(f unsafe.Pointer) string         { return C.GoString(C.nxt_str(f)) }
func callI32ForTest(f unsafe.Pointer) int32          { return int32(C.nxt_i32(f)) }
func callPtrForTest(f unsafe.Pointer) unsafe.Pointer { return C.nxt_ptr(f) }
