//go:build linux || darwin

package engine

import (
	"syscall"
	"unsafe"
)

// osRegion is one anonymous mmap reservation (PROT_NONE); commit makes the tail readable and writable. Fresh
// anonymous pages read as zero, as wasm requires. UNEXECUTED in session 15: compiled by cross-vet only.
type osRegion struct{ raw []byte }

func (r *osRegion) addr() uintptr { return uintptr(unsafe.Pointer(unsafe.SliceData(r.raw))) }

func (r *osRegion) reserve(size int) error {
	b, err := syscall.Mmap(-1, 0, size, syscall.PROT_NONE, syscall.MAP_ANON|syscall.MAP_PRIVATE|mapExtra)
	if err != nil {
		return err
	}
	r.raw = b
	return nil
}

func (r *osRegion) commit(off, n int) error {
	return syscall.Mprotect(r.raw[off:off+n], syscall.PROT_READ|syscall.PROT_WRITE)
}

func (r *osRegion) slice() []byte { return r.raw }

func (r *osRegion) release() {
	syscall.Munmap(r.raw) // r.raw is the exact slice Mmap returned, as Munmap requires
	r.raw = nil
}
