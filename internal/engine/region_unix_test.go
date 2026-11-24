//go:build linux || darwin

package engine

import (
	"syscall"
	"testing"
	"unsafe"
)

func regionCommitted(addr uintptr) bool {
	u := addr
	b := unsafe.Slice((*byte)(*(*unsafe.Pointer)(unsafe.Pointer(&u))), 4096)
	return syscall.Mprotect(b, syscall.PROT_READ|syscall.PROT_WRITE) == nil
}

func reportReleaseFailure(t *testing.T, addr uintptr, err error) {
	t.Helper()
	t.Errorf("reservation at %#x not released: err=%v", addr, err)
}
