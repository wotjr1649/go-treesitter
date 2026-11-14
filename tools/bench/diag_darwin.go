package main

import (
	"fmt"
	"sort"
)

// platDiag prints the vmmap region-type summary (one line per tag with at least 1 MiB resident, and TOTAL). VM_ALLOCATE
// is where the Go heap arenas and the engine mapping show up.
func platDiag(stage string) {
	out := vmmap()
	fp, err := parseVmmapFootprint(out)
	if err != nil {
		fmt.Printf("diag stage=%s vmmap-footprint-error=%q\n", stage, err)
		return
	}
	tags, err := parseVmmapTags(out)
	if err != nil {
		fmt.Printf("diag stage=%s vmmap-summary-error=%q d_footprint=%d\n", stage, err, fp)
		return
	}
	names := make([]string, 0, len(tags))
	for n, t := range tags {
		if t.resident >= 1<<20 || n == "TOTAL" {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		t := tags[n]
		fmt.Printf("diag stage=%s d_footprint=%d tag=%q virtual=%d resident=%d dirty=%d swapped=%d count=%d\n", stage, fp, n, t.virtual, t.resident, t.dirty, t.swapped, t.count)
	}
}
