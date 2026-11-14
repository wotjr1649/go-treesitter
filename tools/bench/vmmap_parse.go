package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Pure parsers for the output of `vmmap -wide <pid>` (macOS). They carry no build tag so that they are unit-tested on
// every platform; only plat_darwin.go runs the tool. One vmmap call yields both numbers the memory lane needs:
//   - "Physical footprint:" in the header, the kernel's phys_footprint ledger (dirty + swapped/compressed anonymous
//     memory; pages the allocator gave back with MADV_FREE_REUSABLE are excluded, file-backed clean pages are not
//     counted) - the macOS counterpart of Windows PrivateUsage / Linux RssAnon;
//   - the region list ("START - END [VSIZE RSDNT DIRTY SWAP] PRT/MAX SHRMOD ... DETAIL"), from which the engine's
//     linear-memory reservation (PROT_NONE mmap of the configured limit, 1 GiB by default, committed by mprotect) is
//     found, as /proc/self/maps does on Linux.

var (
	footprintRe = regexp.MustCompile(`(?m)^\s*Physical footprint:\s+([0-9.]+)\s*([BKMGT]?)\s*$`)
	// name  START-END  [ sizes ]  cur/max  SM=xx  [PURGE=x]  [detail]
	regionRe = regexp.MustCompile(`^\s*(\S.*?)\s+([0-9a-f]{6,})-([0-9a-f]{6,})\s+\[[^\]]*\]\s+([r-][w-][x-])/([r-][w-][x-])\s+SM=(\S+)(.*)$`)
)

// parseSize reads vmmap's size notation: digits with an optional fraction and a B/K/M/G/T suffix (powers of 1024).
func parseSize(num, unit string) (uint64, error) {
	f, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, err
	}
	if unit == "" {
		unit = "B"
	}
	shift := strings.Index("BKMGT", unit)
	if shift < 0 {
		return 0, fmt.Errorf("bad size unit %q", unit)
	}
	return uint64(f * float64(uint64(1)<<(10*uint(shift)))), nil
}

// parseVmmapFootprint returns the "Physical footprint:" value in bytes. The peak line ("Physical footprint (peak):")
// does not match. vmmap prints one decimal at most (e.g. 9.5M), so the value is exact to about 0.1 MB below 1 GB.
func parseVmmapFootprint(out string) (uint64, error) {
	m := footprintRe.FindStringSubmatch(out)
	if m == nil {
		return 0, fmt.Errorf("no \"Physical footprint:\" line in vmmap output")
	}
	return parseSize(m[1], m[2])
}

type vmRegion struct {
	start, end uint64
	cur        string // current protection, e.g. "rw-"
	fileBacked bool   // the detail column names a path
}

// parseVmmapRegions returns every region line (non-writable and writable sections alike). A region-shaped line that
// does not parse is an error rather than silently skipped, so a format change cannot turn into "no region seen".
func parseVmmapRegions(out string) ([]vmRegion, error) {
	var rs []vmRegion
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "[") || !strings.Contains(line, "SM=") {
			continue
		}
		m := regionRe.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("unparsed vmmap region line: %q", line)
		}
		lo, err1 := strconv.ParseUint(m[2], 16, 64)
		hi, err2 := strconv.ParseUint(m[3], 16, 64)
		if err1 != nil || err2 != nil || hi < lo {
			return nil, fmt.Errorf("bad address range in vmmap line: %q", line)
		}
		rs = append(rs, vmRegion{lo, hi, m[4], strings.Contains(m[7], "/")})
	}
	if len(rs) == 0 {
		return nil, fmt.Errorf("no region lines in vmmap output")
	}
	return rs, nil
}

type vmGroup struct{ start, end, committed uint64 }

// vmGroups merges adjacent anonymous regions into allocations (the engine's mprotect splits its one mapping into a
// readable+writable part and a PROT_NONE tail; vmmap lists them as separate entries) and keeps those of at least min
// bytes. committed is the readable+writable part.
func vmGroups(rs []vmRegion, min uint64) []vmGroup {
	sort.Slice(rs, func(i, j int) bool { return rs[i].start < rs[j].start })
	var all []vmGroup
	for _, r := range rs {
		if r.fileBacked {
			continue
		}
		c := uint64(0)
		if strings.HasPrefix(r.cur, "rw") {
			c = r.end - r.start
		}
		if n := len(all) - 1; n >= 0 && all[n].end == r.start {
			all[n].end, all[n].committed = r.end, all[n].committed+c
		} else {
			all = append(all, vmGroup{r.start, r.end, c})
		}
	}
	var big []vmGroup
	for _, g := range all {
		if g.end-g.start >= min {
			big = append(big, g)
		}
	}
	return big
}

// newGroups counts the groups that overlap none of the baseline ranges (those present at process start: the Go
// runtime's own large reservations) and sums their committed bytes. Limit, shared with the Linux detector: an engine
// mapping that the kernel placed adjacent to a baseline reservation is merged into it and not counted.
func newGroups(gs []vmGroup, baseline [][2]uint64) (n int, committed uint64) {
	for _, g := range gs {
		seen := false
		for _, b := range baseline {
			seen = seen || (g.start < b[1] && b[0] < g.end)
		}
		if !seen {
			n++
			committed += g.committed
		}
	}
	return n, committed
}
