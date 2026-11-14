package main

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// macOS: the peak is getrusage ru_maxrss (bytes) and the current resident set comes from ps(1). Without cgo there is
// no VirtualQuery/maps equivalent, so engineRegions reports -1 (not measured): the region-return criterion is NOT_RUN
// on this platform, and the resident-set delta stands in for the private-bytes delta.
func counters() processMemoryCounters {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		fatal("getrusage:", err)
	}
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(syscall.Getpid())).Output()
	if err != nil {
		fatal("ps:", err)
	}
	kb, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		fatal("ps rss:", err)
	}
	rss := uintptr(kb << 10)
	return processMemoryCounters{workingSetSize: rss, peakWorkingSetSize: uintptr(ru.Maxrss), privateUsage: rss, peakPagefileUsage: uintptr(ru.Maxrss)}
}

func engineRegions() (n int, committed uint64) { return -1, 0 }
