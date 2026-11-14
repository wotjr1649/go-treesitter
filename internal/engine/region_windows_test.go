package engine

import "unsafe"

var procVirtualQuery = kernel32.NewProc("VirtualQuery")

type memoryBasicInfo struct {
	BaseAddress, AllocationBase uintptr
	AllocationProtect           uint32
	_                           uint32
	RegionSize                  uintptr
	State, Protect, Type        uint32
}

// regionState returns the VirtualQuery state of the page at addr: 0x1000 committed, 0x2000 reserved, 0x10000 free.
func regionState(addr uintptr) uint32 {
	var mbi memoryBasicInfo
	procVirtualQuery.Call(addr, uintptr(unsafe.Pointer(&mbi)), unsafe.Sizeof(mbi))
	return mbi.State
}

const (
	stateCommitted = 0x1000
	stateFree      = 0x10000
)

func regionCommitted(addr uintptr) bool { return regionState(addr) == stateCommitted }
func regionFreed(addr uintptr) bool     { return regionState(addr) == stateFree }
