package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pkgDir(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte("package p\n"+body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCompare(t *testing.T) {
	const base = "type Point struct{ Row, Column uint32 }\ntype Closed struct{ A int; b int }\ntype Emb struct{ *Point; fmt.Stringer }\n"
	for _, tc := range []struct {
		name, head string
		bad        bool
	}{
		{"same", base, false},
		{"field added", "type Point struct{ Row, Column, Extra uint32 }\ntype Closed struct{ A int; b int }\ntype Emb struct{ *Point; fmt.Stringer }\n", true},
		{"fields swapped", "type Point struct{ Column, Row uint32 }\ntype Closed struct{ A int; b int }\ntype Emb struct{ *Point; fmt.Stringer }\n", true},
		{"type changed", "type Point struct{ Row, Column uint64 }\ntype Closed struct{ A int; b int }\ntype Emb struct{ *Point; fmt.Stringer }\n", true},
		{"field made unexported", "type Point struct{ Row, column uint32 }\ntype Closed struct{ A int; b int }\ntype Emb struct{ *Point; fmt.Stringer }\n", true},
		{"closed struct may change", "type Point struct{ Row, Column uint32 }\ntype Closed struct{ A int; b int; c int }\ntype Emb struct{ *Point; fmt.Stringer }\n", false},
		{"embedded changed", "type Point struct{ Row, Column uint32 }\ntype Closed struct{ A int; b int }\ntype Emb struct{ Point; fmt.Stringer }\n", true},
		{"removal is apidiff's", "type Closed struct{ A int; b int }\ntype Emb struct{ *Point; fmt.Stringer }\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := structs(pkgDir(t, base))
			if err != nil {
				t.Fatal(err)
			}
			h, err := structs(pkgDir(t, tc.head))
			if err != nil {
				t.Fatal(err)
			}
			problems := compare(b, h)
			if (len(problems) > 0) != tc.bad {
				t.Fatalf("problems %q, want bad=%v", strings.Join(problems, "\n"), tc.bad)
			}
		})
	}
}
