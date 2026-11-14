package main

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// vmmapSample is written from the documented layout of `vmmap -wide` (header with "Physical footprint", then
// "Non-writable"/"Writable regions" tables). It is NOT recorded tool output: replace it with a file from the
// vmmap-raw artifact of a macOS bench run (see BENCH_VMMAP_DUMP in plat_darwin.go) when one exists.
const vmmapSample = `Process:         bench [4242]
Path:            /Users/runner/work/_temp/bench
Physical footprint:         9.5M
Physical footprint (peak):  1.2G
Idle exit:                  untracked
----

Virtual Memory Map of process 4242 (bench)
Output report format:  2.4  -- 64-bit process
VM page size:  16384 bytes

==== Non-writable regions for process 4242
REGION TYPE                    START - END         [ VSIZE  RSDNT  DIRTY   SWAP] PRT/MAX SHRMOD PURGE    REGION DETAIL
__TEXT                      102f94000-103400000    [ 4528K  4528K     0K     0K] r-x/r-x SM=COW          /Users/runner/work/_temp/bench
__LINKEDIT                  1034c0000-103500000    [  256K    16K     0K     0K] r--/r-- SM=COW          /Users/runner/work/_temp/bench
==== Writable regions for process 4242
__DATA                      103400000-103440000    [  256K   256K   256K     0K] rw-/rwx SM=COW          /Users/runner/work/_temp/bench
MALLOC_LARGE (reserved)     140000000-140400000    [ 4096K     0K     0K     0K] ---/rwx SM=NUL
Go                          c000000000-c004000000  [ 64.0M  8192K  8192K     0K] rw-/rwx SM=PRV
VM_ALLOCATE (reserved)      16b000000-16b400000    [ 4096K     0K     0K     0K] ---/rwx SM=NUL
Stack                       16f000000-16f800000    [ 8192K    80K    80K     0K] rw-/rwx SM=PRV          thread 0
VM_ALLOCATE                 170000000-170400000    [ 4096K  4096K  4096K     0K] rw-/rwx SM=PRV
VM_ALLOCATE (reserved)      170400000-1b0000000    [ 1020M     0K     0K     0K] ---/rwx SM=NUL

==== Summary for process 4242
ReadOnly portion of Libraries: Total=100.0M resident=1.0M(1%) swapped_out_or_unallocated=99.0M(99%)
`

func TestParseSize(t *testing.T) {
	for _, c := range []struct {
		num, unit string
		want      uint64
	}{{"0", "B", 0}, {"512", "", 512}, {"1633", "K", 1633 << 10}, {"9.5", "M", 9<<20 + 1<<19}, {"1.0", "G", 1 << 30}, {"2", "T", 2 << 40}} {
		if got, err := parseSize(c.num, c.unit); err != nil || got != c.want {
			t.Errorf("parseSize(%q,%q) = %d, %v; want %d", c.num, c.unit, got, err, c.want)
		}
	}
	if _, err := parseSize("1", "X"); err == nil {
		t.Error("unit X accepted")
	}
}

func TestParseVmmapFootprint(t *testing.T) {
	got, err := parseVmmapFootprint(vmmapSample)
	if err != nil || got != 9<<20+1<<19 { // the 1.2G peak line must not win
		t.Fatalf("footprint = %d, %v; want 9.5M", got, err)
	}
	if _, err := parseVmmapFootprint("Physical footprint (peak):  1.2G\n"); err == nil {
		t.Error("peak-only output accepted as footprint")
	}
	if _, err := parseVmmapFootprint(""); err == nil {
		t.Error("empty output accepted")
	}
}

func TestParseVmmapRegions(t *testing.T) {
	rs, err := parseVmmapRegions(vmmapSample)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 9 {
		t.Fatalf("%d regions, want 9", len(rs))
	}
	if r := rs[0]; r.start != 0x102f94000 || r.end != 0x103400000 || r.cur != "r-x" || !r.fileBacked {
		t.Errorf("first region = %+v", r)
	}
	if r := rs[8]; r.start != 0x170400000 || r.end != 0x1b0000000 || r.cur != "---" || r.fileBacked {
		t.Errorf("last region = %+v", r)
	}
	if _, err := parseVmmapRegions("garbage\n"); err == nil {
		t.Error("output without regions accepted")
	}
	if _, err := parseVmmapRegions("X  12-34  [ 1K 1K 1K 0K] rw-/rwx SM=PRV\n"); err == nil {
		t.Error("short address (not a region line shape) accepted")
	}
}

// The engine's mapping appears as an rw part followed by a PROT_NONE tail: together exactly 1 GiB here.
func TestEngineGroupsAndBaseline(t *testing.T) {
	rs, err := parseVmmapRegions(vmmapSample)
	if err != nil {
		t.Fatal(err)
	}
	gs := newEngineGroups(rs, nil, 512<<20)
	if len(gs) != 1 || gs[0].start != 0x170000000 || gs[0].end != 0x1b0000000 || gs[0].committed != 4<<20 {
		t.Fatalf("groups = %+v", gs)
	}
	if got := newEngineGroups(rs, anonRegions(rs), 512<<20); len(got) != 0 {
		t.Errorf("a mapping present at start was counted: %+v", got)
	}
	after := rs[:len(rs)-2] // Close: the mapping is gone
	if got := newEngineGroups(after, nil, 512<<20); len(got) != 0 {
		t.Errorf("regions after Close = %d, want 0", len(got))
	}
}

// The kernel can place the engine mapping directly behind a reservation the Go runtime made before main: merged, the
// two overlap the baseline. Only the address space that was not mapped at start may count (run 43568897670).
func TestEngineGroupAdjacentToBaseline(t *testing.T) {
	const base = `VM_ALLOCATE                 302c4c000-320000000    [ 467.7M     0K     0K     0K] ---/rwx SM=NUL
`
	const now = base + `VM_ALLOCATE                 320000000-3200a0000    [  640K   640K   640K     0K] rw-/rwx SM=COW
VM_ALLOCATE                 3200a0000-360000000    [  1.0G     0K     0K     0K] ---/rwx SM=NUL
`
	b, err := parseVmmapRegions(base)
	if err != nil {
		t.Fatal(err)
	}
	n, err := parseVmmapRegions(now)
	if err != nil {
		t.Fatal(err)
	}
	gs := newEngineGroups(n, anonRegions(b), 512<<20)
	if len(gs) != 1 || gs[0].start != 0x320000000 || gs[0].end != 0x360000000 || gs[0].committed != 640<<10 {
		t.Fatalf("groups = %+v, want one 1 GiB mapping at 320000000 with 640 KiB committed", gs)
	}
}

// Real macOS 26 arm64 output (bench run 43567021988). It contains a kernel accounting line without an address range,
// which the first parser rejected; every other region line must still parse.
func TestParseVmmapRealOutput(t *testing.T) {
	files := []string{filepath.Join("testdata", "vmmap-macos26-arm64.txt")}
	if dir := os.Getenv("TSG_VMMAP_DIR"); dir != "" {
		more, _ := filepath.Glob(filepath.Join(dir, "*", "*.txt"))
		files = append(files, more...)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if fp, err := parseVmmapFootprint(string(b)); err != nil || fp == 0 {
			t.Errorf("%s: footprint %d, %v", f, fp, err)
		}
		rs, err := parseVmmapRegions(string(b))
		if err != nil || len(rs) == 0 {
			t.Errorf("%s: %d regions, %v", f, len(rs), err)
			continue
		}
		t.Logf("%s: %d regions, %d groups >= 512 MiB", filepath.Base(f), len(rs), len(newEngineGroups(rs, nil, 512<<20)))
	}
}

func readGz(t *testing.T, name string) string {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "run43568897670", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(z)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Recorded vmmap output of three bench-lane processes (macOS 26.7.2 arm64, run 43568897670), each at the three points
// the lane looks at: process start (baseline), parser open, parser closed. In json/snapshot and csharp/parse the
// engine mapping (320000000-360000000 and the like) sits directly behind a Go reservation of the baseline, and the
// old detector reported no region there; the footprint line, tag summary and engine mapping must all be found.
func TestEngineRegionsRecordedRuns(t *testing.T) {
	for _, c := range []struct {
		name         string
		minCommitted uint64 // the engine's rw part while open (the heap of the wasm instance), lower bound
	}{{"json-snapshot", 100 << 20}, {"csharp-parse", 15 << 20}, {"pydecimal-parse", 1 << 20}} {
		regs := func(phase string) []vmRegion {
			rs, err := parseVmmapRegions(readGz(t, c.name+"-"+phase+".txt.gz"))
			if err != nil {
				t.Fatalf("%s %s: %v", c.name, phase, err)
			}
			return rs
		}
		base := anonRegions(regs("start"))
		open := newEngineGroups(regs("open"), base, engineRegionMin)
		if len(open) != 1 || open[0].end-open[0].start != 1<<30 || open[0].committed < c.minCommitted {
			t.Errorf("%s: open = %+v, want one 1 GiB mapping with >= %d bytes committed", c.name, open, c.minCommitted)
		}
		if closed := newEngineGroups(regs("closed"), base, engineRegionMin); len(closed) != 0 {
			t.Errorf("%s: after Close = %+v, want none", c.name, closed)
		}
		if got := newEngineGroups(regs("start"), base, engineRegionMin); len(got) != 0 {
			t.Errorf("%s: baseline against itself = %+v, want none", c.name, got)
		}
	}
}

func TestParseVmmapTagsRecorded(t *testing.T) {
	out := readGz(t, "json-snapshot-open.txt.gz")
	tags, err := parseVmmapTags(out)
	if err != nil {
		t.Fatal(err)
	}
	fp, _ := parseVmmapFootprint(out)
	total := tags["TOTAL"]
	// The summary prints one decimal, the header too: the two footprint figures agree to about 0.1 MB per tag.
	if d := int64(total.dirty) - int64(fp); d > 2<<20 || d < -2<<20 {
		t.Errorf("TOTAL dirty %d vs footprint %d", total.dirty, fp)
	}
	if va := tags["VM_ALLOCATE"]; va.dirty < 200<<20 || va.virtual < 1<<30 || va.count == 0 {
		t.Errorf("VM_ALLOCATE = %+v", va)
	}
	if _, ok := tags["MALLOC guard page"]; !ok {
		t.Error("tag names with spaces were not parsed")
	}
	if _, err := parseVmmapTags("no summary here"); err == nil {
		t.Error("output without a summary accepted")
	}
}
