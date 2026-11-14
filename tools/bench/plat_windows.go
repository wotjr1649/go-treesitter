//go:build windows

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	k32     = syscall.NewLazyDLL("kernel32.dll")
	psapi   = syscall.NewLazyDLL("psapi.dll")
	pQPC    = k32.NewProc("QueryPerformanceCounter")
	pQPF    = k32.NewProc("QueryPerformanceFrequency")
	pMemory = psapi.NewProc("GetProcessMemoryInfo")
	pQuery  = k32.NewProc("VirtualQuery")
	qpf     int64
)

// processMemoryCounters is PROCESS_MEMORY_COUNTERS.
type processMemoryCounters struct {
	cb, pageFaultCount                 uint32
	peakWorkingSetSize, workingSetSize uintptr
	quotaPeakPagedPool, quotaPagedPool uintptr
	quotaPeakNonPaged, quotaNonPaged   uintptr
	pagefileUsage, peakPagefileUsage   uintptr
	privateUsage                       uintptr // PROCESS_MEMORY_COUNTERS_EX
}

// engineRegionMin is the smallest reservation counted as a candidate engine linear-memory region (the engine reserves
// 1 GiB of address space up front, internal/engine MinLimit). The runtime itself also holds a few large reservations
// from process start (512, 512 and 4096 MiB here), so engineRegions reports only allocations that were not there at
// start.
const engineRegionMin = 512 << 20

type memoryBasicInformation struct {
	baseAddress, allocationBase uintptr
	allocationProtect           uint32
	partitionID, _              uint16
	regionSize                  uintptr
	state, protect, typ         uint32
	_                           uint32
}

var baselineRegions map[uintptr]bool

// engineRegions returns how many private allocations of at least engineRegionMin reserved bytes exist that were not
// present at the first call (made at process start), and how many bytes of them are committed.
func engineRegions() (n int, committed uint64) {
	const memCommit, memReserve, memPrivate = 0x1000, 0x2000, 0x20000
	type alloc struct{ reserved, committed uint64 }
	by := map[uintptr]*alloc{}
	var addr uintptr
	for {
		var m memoryBasicInformation
		if r, _, _ := pQuery.Call(addr, uintptr(unsafe.Pointer(&m)), unsafe.Sizeof(m)); r == 0 {
			break
		}
		if m.typ == memPrivate && (m.state == memCommit || m.state == memReserve) {
			a := by[m.allocationBase]
			if a == nil {
				a = &alloc{}
				by[m.allocationBase] = a
			}
			a.reserved += uint64(m.regionSize)
			if m.state == memCommit {
				a.committed += uint64(m.regionSize)
			}
		}
		next := m.baseAddress + m.regionSize
		if next <= addr {
			break
		}
		addr = next
	}
	first := baselineRegions == nil
	if first {
		baselineRegions = map[uintptr]bool{}
	}
	for base, a := range by {
		if a.reserved < engineRegionMin {
			continue
		}
		if first {
			baselineRegions[base] = true
		} else if !baselineRegions[base] {
			n++
			committed += a.committed
		}
	}
	return n, committed
}

func now() int64 {
	var c int64
	pQPC.Call(uintptr(unsafe.Pointer(&c)))
	return c
}

func ms(a, b int64) float64 { return float64(b-a) * 1000 / float64(qpf) }

func counters() processMemoryCounters {
	var m processMemoryCounters
	m.cb = uint32(unsafe.Sizeof(m))
	h, _ := syscall.GetCurrentProcess()
	if r, _, err := pMemory.Call(uintptr(h), uintptr(unsafe.Pointer(&m)), uintptr(m.cb)); r == 0 {
		fmt.Fprintln(os.Stderr, "GetProcessMemoryInfo:", err)
		os.Exit(2)
	}
	return m
}

func initPlatform() { pQPF.Call(uintptr(unsafe.Pointer(&qpf))) }
