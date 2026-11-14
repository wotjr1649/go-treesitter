//go:build linux || darwin

package main

import "time"

// Linux and macOS counterparts of plat_windows.go. The field names of processMemoryCounters are kept so mem() is
// shared; what each field means here: workingSetSize = resident set now, peakWorkingSetSize = peak resident set
// (VmHWM / ru_maxrss), privateUsage = resident anonymous memory (Linux RssAnon; macOS has no per-process equivalent
// without cgo, so it is the resident set), peakPagefileUsage = the peak resident set (no peak-private counter exists).
type processMemoryCounters struct {
	workingSetSize, peakWorkingSetSize, privateUsage, peakPagefileUsage uintptr
}

// engineRegionMin is the smallest reservation counted as a candidate engine linear-memory region (the engine reserves
// 1 GiB of address space up front, internal/engine MinLimit), as on Windows.
const engineRegionMin = 512 << 20

var clockBase time.Time

func initPlatform() { clockBase = time.Now() }

func now() int64 { return int64(time.Since(clockBase)) } // monotonic nanoseconds

func ms(a, b int64) float64 { return float64(b-a) / 1e6 }
