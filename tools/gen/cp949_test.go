package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func readTestFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestCP949TableIsCurrent: the committed root-module table is byte-for-byte what the generator makes from the committed,
// hash-pinned index. It needs no network: the index is testdata/index-euc-kr.txt.
func TestCP949TableIsCurrent(t *testing.T) {
	got, err := genCP949(readTestFile(t, "testdata/index-euc-kr.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if want := readTestFile(t, "../../cp949_table.go"); !bytes.Equal(got, want) {
		t.Fatalf("../../cp949_table.go is out of date (%d bytes, generator makes %d): run `go run . cp949` in tools/gen", len(want), len(got))
	}
}

// TestCP949IndexIsPinned: any change to the index bytes, or to what the parser accepts, is an error.
func TestCP949IndexIsPinned(t *testing.T) {
	index := readTestFile(t, "testdata/index-euc-kr.txt")
	if sumBytes(index) != cp949IndexSHA256 {
		t.Fatalf("testdata/index-euc-kr.txt sha256 %s, pinned %s", sumBytes(index), cp949IndexSHA256)
	}
	table, err := parseEUCKRIndex(index)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, cp := range table {
		if cp != 0 {
			n++
		}
	}
	if n != cp949Mappings || table[0] != 0xAC02 || table[cp949TableLen-1] == 0 {
		t.Fatalf("%d mappings, first U+%04X", n, table[0])
	}
	// the user-defined areas stay unmapped: lead 0xC9 and lead 0xFE
	for _, lead := range []int{0xC9, 0xFE} {
		for trail := 0x41; trail <= 0xFE; trail++ {
			if p := (lead-0x81)*190 + trail - 0x41; p < cp949TableLen && table[p] != 0 {
				t.Fatalf("lead %#x trail %#x is mapped", lead, trail)
			}
		}
	}
	mutants := map[string][]byte{
		"changed byte": bytes.Replace(index, []byte("0xAC02"), []byte("0xAC03"), 1),
		"truncated":    index[:len(index)-1],
	}
	for name, m := range mutants {
		if _, err := parseEUCKRIndex(m); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestCP949LicenseNotice: the BSD text the generated header carries is the BSD section of the WHATWG licence file, and the
// manifest records that file's hash.
func TestCP949LicenseNotice(t *testing.T) {
	lic := readTestFile(t, "../../LICENSES/whatwg-encoding.txt")
	if !strings.Contains(string(lic), bsd3) {
		t.Fatal("LICENSES/whatwg-encoding.txt does not contain the BSD text the generator embeds")
	}
	var manifest []struct{ Component, Path, License, Source, SHA256 string }
	if err := json.Unmarshal(readTestFile(t, "../../LICENSES/manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	for _, e := range manifest {
		if e.Component == "whatwg-encoding" {
			if e.SHA256 != sumBytes(lic) || e.Path != "LICENSES/whatwg-encoding.txt" || !strings.Contains(e.Source, "/whatwg/encoding/blob/") {
				t.Fatalf("manifest entry does not match the licence file: %+v", e)
			}
			return
		}
	}
	t.Fatal("LICENSES/manifest.json has no whatwg-encoding entry")
}
