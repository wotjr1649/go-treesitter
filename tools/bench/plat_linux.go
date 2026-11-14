package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

func counters() processMemoryCounters {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		fatal("/proc/self/status:", err)
	}
	defer f.Close()
	var m processMemoryCounters
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, _ := strings.Cut(sc.Text(), ":")
		var dst *uintptr
		switch k {
		case "VmHWM":
			dst = &m.peakWorkingSetSize
		case "VmRSS":
			dst = &m.workingSetSize
		case "RssAnon":
			dst = &m.privateUsage
		default:
			continue
		}
		kb, _ := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(v), " kB"), 10, 64)
		*dst = uintptr(kb << 10)
	}
	m.peakPagefileUsage = m.peakWorkingSetSize
	return m
}

var baselineRegions [][2]uint64

// engineRegions is the /proc/self/maps counterpart of the Windows VirtualQuery scan: it merges adjacent anonymous
// mappings into allocations and counts those with at least engineRegionMin reserved bytes that do not overlap one seen
// at the first call (made at process start); committed is the readable+writable part (the engine commits by mprotect).
// Limit: the engine's mapping is told apart from the Go runtime's only by address, so a region that the kernel merged
// into a runtime reservation present at start would not be counted.
func engineRegions() (n int, committed uint64) {
	b, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		fatal("/proc/self/maps:", err)
	}
	type group struct{ start, end, committed uint64 }
	var groups []group
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) != 5 || f[4] != "0" { // file-backed, [heap], [stack]... have inode or a path
			continue
		}
		lo, hi, _ := strings.Cut(f[0], "-")
		start, err1 := strconv.ParseUint(lo, 16, 64)
		end, err2 := strconv.ParseUint(hi, 16, 64)
		if err1 != nil || err2 != nil {
			fatal("bad /proc/self/maps line:", line)
		}
		c := uint64(0)
		if strings.HasPrefix(f[1], "rw") {
			c = end - start
		}
		if g := len(groups) - 1; g >= 0 && groups[g].end == start {
			groups[g].end, groups[g].committed = end, groups[g].committed+c
		} else {
			groups = append(groups, group{start, end, c})
		}
	}
	first := baselineRegions == nil
	for _, g := range groups {
		if g.end-g.start < engineRegionMin {
			continue
		}
		if first {
			baselineRegions = append(baselineRegions, [2]uint64{g.start, g.end})
			continue
		}
		seen := false
		for _, r := range baselineRegions {
			seen = seen || (g.start < r[1] && r[0] < g.end)
		}
		if !seen {
			n++
			committed += g.committed
		}
	}
	if first {
		baselineRegions = append(baselineRegions, [2]uint64{}) // non-nil even when none was found
	}
	return n, committed
}
