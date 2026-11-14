package main

import (
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
	gs := vmGroups(rs, 512<<20)
	if len(gs) != 1 || gs[0].start != 0x170000000 || gs[0].end != 0x1b0000000 || gs[0].committed != 4<<20 {
		t.Fatalf("groups = %+v", gs)
	}
	if n, c := newGroups(gs, nil); n != 1 || c != 4<<20 {
		t.Errorf("new groups = %d, %d; want 1, 4 MiB", n, c)
	}
	if n, _ := newGroups(gs, [][2]uint64{{gs[0].start, gs[0].end}}); n != 0 {
		t.Errorf("a group present at start was counted")
	}
	after := rs[:len(rs)-2] // Close: the mapping is gone
	if n, _ := newGroups(vmGroups(after, 512<<20), nil); n != 0 {
		t.Errorf("regions after Close = %d, want 0", n)
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
		t.Logf("%s: %d regions, %d groups >= 512 MiB", filepath.Base(f), len(rs), len(vmGroups(rs, 512<<20)))
	}
}
