//go:build linux || darwin

package engine

import (
	"syscall"
	"unsafe"
)

// mprotect on an unmapped page fails with ENOMEM; on a mapped page it succeeds.
func probe(addr uintptr, prot int) error {
	u := addr
	b := unsafe.Slice((*byte)(*(*unsafe.Pointer)(unsafe.Pointer(&u))), 4096)
	return syscall.Mprotect(b, prot)
}

func regionCommitted(addr uintptr) bool {
	return probe(addr, syscall.PROT_READ|syscall.PROT_WRITE) == nil
}
func regionFreed(addr uintptr) bool { return probe(addr, syscall.PROT_NONE) != nil }
