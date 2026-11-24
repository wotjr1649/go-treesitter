package engine

import (
	"testing"
	"unsafe"
)

var procVirtualQuery = kernel32.NewProc("VirtualQuery")

type memoryBasicInfo struct {
	BaseAddress, AllocationBase uintptr
	AllocationProtect           uint32
	_                           uint32
	RegionSize                  uintptr
	State, Protect, Type        uint32
}

func queryRegion(addr uintptr) (memoryBasicInfo, error) {
	var mbi memoryBasicInfo
	n, _, err := procVirtualQuery.Call(addr, uintptr(unsafe.Pointer(&mbi)), unsafe.Sizeof(mbi))
	if n == 0 {
		return mbi, err
	}
	return mbi, nil
}

func regionCommitted(addr uintptr) bool {
	mbi, err := queryRegion(addr)
	return err == nil && mbi.State == memCommit
}

func reportReleaseFailure(t *testing.T, addr uintptr, err error) {
	t.Helper()
	mbi, queryErr := queryRegion(addr)
	t.Errorf("reservation at %#x not released: err=%v, State=%#x, AllocationBase=%#x, VirtualQuery err=%v",
		addr, err, mbi.State, mbi.AllocationBase, queryErr)
}
