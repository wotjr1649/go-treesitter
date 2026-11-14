package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// macOS. Without cgo there is no task_info/proc_pid_rusage and no mach_vm_region, so the two ADR-0022 numbers come from
// vmmap(1) (part of the base system, run on our own pid; parsers and their limits are in vmmap_parse.go):
//   - privateUsage = "Physical footprint" (phys_footprint). Not ps rss: rss counts file-backed image pages and pages
//     the Go runtime returned with MADV_FREE_REUSABLE but the kernel has not reclaimed (run 43556380715 failed on that);
//   - engineRegions = the engine's PROT_NONE reservation of the configured limit, found in the region list.
//
// The peak stays getrusage ru_maxrss and workingSetSize stays ps rss, so the new footprint can be read next to the old
// number. A vmmap failure is fatal: a lane that cannot measure must not pass.
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
	fp, err := parseVmmapFootprint(vmmap())
	if err != nil {
		fatal("vmmap:", err)
	}
	return processMemoryCounters{workingSetSize: uintptr(kb << 10), peakWorkingSetSize: uintptr(ru.Maxrss), privateUsage: uintptr(fp), peakPagefileUsage: uintptr(ru.Maxrss)}
}

var (
	vmmapCalls  int
	baselineVM  []vmRegion
	baselineSet bool
)

// vmmap runs `vmmap -wide <pid>`. If BENCH_VMMAP_DUMP names a directory, each raw output is saved there (benchci sets
// it for the first run of every input), which is how real tool output becomes the parser's test fixtures.
func vmmap() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pid := strconv.Itoa(os.Getpid())
	out, err := exec.CommandContext(ctx, "vmmap", "-wide", pid).Output()
	if err != nil {
		var se []byte
		if ee, ok := err.(*exec.ExitError); ok {
			se = ee.Stderr
		}
		fatal("vmmap -wide "+pid+":", err, firstBytes(se), firstBytes(out))
	}
	if d := os.Getenv("BENCH_VMMAP_DUMP"); d != "" {
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, fmt.Sprintf("vmmap-%s-%02d.txt", pid, vmmapCalls)), out, 0o644)
	}
	vmmapCalls++
	return string(out)
}

func firstBytes(b []byte) string {
	if len(b) > 400 {
		b = b[:400]
	}
	return string(b)
}

// engineRegions is the vmmap counterpart of the Windows/Linux scans: the first call (at process start) records the
// address space of the anonymous regions then and reports none; later calls count the engine-sized mappings in the
// address space that was not mapped at start (see newEngineGroups for why the baseline is subtracted, not matched).
func engineRegions() (n int, committed uint64) {
	rs, err := parseVmmapRegions(vmmap())
	if err != nil {
		fatal("vmmap:", err)
	}
	if !baselineSet {
		baselineSet = true
		baselineVM = anonRegions(rs)
		return 0, 0
	}
	for _, g := range newEngineGroups(rs, baselineVM, engineRegionMin) {
		n++
		committed += g.committed
	}
	return n, committed
}
