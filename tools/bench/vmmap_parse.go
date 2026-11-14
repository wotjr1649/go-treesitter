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
	kernelLineRe = regexp.MustCompile(`\skernel-kernel\s+\[`)
	regionRe     = regexp.MustCompile(`^\s*(\S.*?)\s+([0-9a-f]{6,})-([0-9a-f]{6,})\s+\[[^\]]*\]\s+([r-][w-][x-])/([r-][w-][x-])\s+SM=(\S+)(.*)$`)
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
		// Kernel-side accounting ("page table in kernel  kernel-kernel [...]") has no user address range.
		if kernelLineRe.MatchString(line) {
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

// anonRegions returns the anonymous (not file-backed) regions sorted by address.
func anonRegions(rs []vmRegion) []vmRegion {
	var out []vmRegion
	for _, r := range rs {
		if !r.fileBacked {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].start < out[j].start })
	return out
}

// newEngineGroups finds the engine's linear-memory mappings: the address space of anonymous regions that was not
// mapped at process start (baseline, from anonRegions), with adjacent leftovers merged (the engine's mprotect splits
// its one mapping into a readable+writable head and a PROT_NONE tail; vmmap lists them as separate entries), keeping
// groups of at least min bytes. committed is the readable+writable part.
//
// Subtracting the baseline address space rather than rejecting any group that overlaps it matters on macOS: the kernel
// places the 1 GiB engine mapping right after the Go runtime's own reservations, so the merged range of both overlaps
// the baseline and the engine was reported as "no region" (run 43568897670: 0 of 1 in half of the rows).
func newEngineGroups(rs, baseline []vmRegion, min uint64) []vmGroup {
	var all []vmGroup
	for _, r := range anonRegions(rs) {
		rw := strings.HasPrefix(r.cur, "rw")
		for _, p := range subtract(r.start, r.end, baseline) {
			c := uint64(0)
			if rw {
				c = p[1] - p[0]
			}
			if n := len(all) - 1; n >= 0 && all[n].end == p[0] {
				all[n].end, all[n].committed = p[1], all[n].committed+c
			} else {
				all = append(all, vmGroup{p[0], p[1], c})
			}
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

// subtract returns the parts of [lo,hi) not covered by any baseline region (baseline sorted, non-overlapping).
func subtract(lo, hi uint64, baseline []vmRegion) [][2]uint64 {
	var out [][2]uint64
	for _, b := range baseline {
		if b.end <= lo {
			continue
		}
		if b.start >= hi {
			break
		}
		if b.start > lo {
			out = append(out, [2]uint64{lo, b.start})
		}
		lo = max(lo, b.end)
		if lo >= hi {
			return out
		}
	}
	return append(out, [2]uint64{lo, hi})
}

// vmTag is one row of the "Summary for process" region-type table.
type vmTag struct {
	virtual, resident, dirty, swapped uint64
	count                             int
}

var tagRowRe = regexp.MustCompile(`^(\S.*?)\s+([0-9.]+[BKMGT]?)\s+([0-9.]+[BKMGT]?)\s+([0-9.]+[BKMGT]?)\s+([0-9.]+[BKMGT]?)\s+[0-9.]+[BKMGT]?\s+[0-9.]+[BKMGT]?\s+[0-9.]+[BKMGT]?\s+(\d+)\b`)

func splitSize(tok string) (uint64, error) {
	if n := len(tok) - 1; n > 0 && strings.ContainsRune("BKMGT", rune(tok[n])) {
		return parseSize(tok[:n], tok[n:])
	}
	return parseSize(tok, "")
}

// parseVmmapTags reads the region-type summary (VIRTUAL RESIDENT DIRTY SWAPPED ... COUNT per tag, TOTAL last). It is a
// diagnostic aid: where the footprint sits by tag (VM_ALLOCATE holds the Go heap arenas and the engine mapping).
func parseVmmapTags(out string) (map[string]vmTag, error) {
	tags := map[string]vmTag{}
	in := false
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "REGION TYPE") && strings.Contains(line, "(non-coalesced)"):
			in = true
		case !in || strings.HasPrefix(line, "=="):
		case strings.TrimSpace(line) == "":
			in = false
		default:
			m := tagRowRe.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("unparsed vmmap summary line: %q", line)
			}
			var t vmTag
			var err error
			for i, dst := range []*uint64{&t.virtual, &t.resident, &t.dirty, &t.swapped} {
				if *dst, err = splitSize(m[2+i]); err != nil {
					return nil, fmt.Errorf("vmmap summary line %q: %v", line, err)
				}
			}
			t.count, _ = strconv.Atoi(m[6])
			tags[m[1]] = t
		}
	}
	if _, ok := tags["TOTAL"]; !ok {
		return nil, fmt.Errorf("no TOTAL row in the vmmap region-type summary")
	}
	return tags, nil
}
