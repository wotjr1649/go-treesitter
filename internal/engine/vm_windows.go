package engine

import (
	"syscall"
	"unsafe"
)

const (
	memCommit     = 0x1000
	memReserve    = 0x2000
	memRelease    = 0x8000
	pageNoAccess  = 0x01
	pageReadWrite = 0x04
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procVirtualAlloc = kernel32.NewProc("VirtualAlloc")
	procVirtualFree  = kernel32.NewProc("VirtualFree")
)

// osRegion is one VirtualAlloc reservation.
type osRegion struct {
	base uintptr
	size int
}

func (r *osRegion) addr() uintptr { return r.base }

func (r *osRegion) reserve(size int) error {
	p, _, e := procVirtualAlloc.Call(0, uintptr(size), memReserve, pageNoAccess)
	if p == 0 {
		return e
	}
	r.base, r.size = p, size
	return nil
}

func (r *osRegion) commit(off, n int) error {
	p, _, e := procVirtualAlloc.Call(r.base+uintptr(off), uintptr(n), memCommit, pageReadWrite)
	if p == 0 {
		return e
	}
	return nil
}

// slice views the whole reservation; only the committed prefix may be touched.
func (r *osRegion) slice() []byte {
	u := r.base
	return unsafe.Slice((*byte)(*(*unsafe.Pointer)(unsafe.Pointer(&u))), r.size)
}

func (r *osRegion) release() {
	procVirtualFree.Call(r.base, 0, memRelease)
	*r = osRegion{}
}
