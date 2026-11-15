package main

import (
	"fmt"
	"testing"
)

func dump(err bool, end int, kind string) []byte {
	n := 0
	if err {
		n = 1
	}
	return []byte(fmt.Sprintf("#tsg-dump/1 root_has_error=%d nodes=1\n0\t-\t%s\t1\t0\t0\t0\t%d\t0\t%d\t0:0\t0:%d\n", n, kind, n, end, end))
}
func TestCompare(t *testing.T) {
	for _, tc := range []struct {
		name          string
		src           []byte
		before, after []byte
		allow, want   bool
	}{
		{"clean identical", []byte("abc"), dump(false, 3, "root"), dump(false, 3, "root"), false, true},
		{"clean changed", []byte("abc"), dump(false, 3, "root"), dump(false, 3, "other"), false, false},
		{"error changed", []byte("abc"), dump(true, 1, "root"), dump(true, 3, "other"), false, true},
		{"remaining root fault", []byte("abc"), dump(true, 1, "root"), dump(true, 2, "other"), false, false},
		{"NUL fix opt in", []byte("a\x00b"), dump(false, 1, "root"), dump(true, 3, "root"), true, true},
		{"NUL fix no opt in", []byte("a\x00b"), dump(false, 1, "root"), dump(true, 3, "root"), false, false},
		{"unrelated later NUL", []byte("abc\x00d"), dump(false, 1, "root"), dump(true, 5, "root"), true, false},
		{"NUL no error", []byte("a\x00b"), dump(false, 1, "root"), dump(false, 3, "root"), true, false},
		{"non NUL clean truncation", []byte("abc"), dump(false, 1, "root"), dump(true, 3, "root"), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, e := compare(input{tc.name, tc.src}, tc.before, tc.after, tc.allow)
			if e != nil {
				t.Fatal(e)
			}
			if r.Pass != tc.want {
				t.Fatalf("result=%+v want pass=%t", r, tc.want)
			}
		})
	}
	if _, e := parseTree([]byte("bad")); e == nil {
		t.Fatal("bad dump accepted")
	}
}
func TestFuzzSeeds(t *testing.T) {
	src := []byte("package p\nfunc FuzzParseYAML(f *F) { fuzzParse(f, \"yaml\", \"é\\x00\\n\", `a: [`, \"\") }\n")
	seeds, e := fuzzSeeds("seeds.go", src, "FuzzParseYAML")
	if e != nil {
		t.Fatal(e)
	}
	if len(seeds) != 3 || string(seeds[0]) != "é\x00\n" || string(seeds[1]) != "a: [" || len(seeds[2]) != 0 {
		t.Fatalf("seeds=%q", seeds)
	}
	if _, e := fuzzSeeds("seeds.go", src, "missing"); e == nil {
		t.Fatal("missing target accepted")
	}
	if _, e := fuzzSeeds("seeds.go", []byte("package p; func FuzzParseYAML(f *F) { fuzzParse(f, \"yaml\", variable) }"), "FuzzParseYAML"); e == nil {
		t.Fatal("variable seed accepted")
	}
}
