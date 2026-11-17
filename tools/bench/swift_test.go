//go:build lang_swift

package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	treesitter "github.com/wotjr1649/go-treesitter"
)

func TestSwiftSmallWorkload(t *testing.T) {
	src, err := os.ReadFile("testdata/swift/rentals.swift")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(src)); got != "1a0a9a11f18cfaeab99eaeaceb05feb101054119dd2ebaa8d31fca977b96a5a6" {
		t.Fatalf("workload identity drift: %s", got)
	}
	p, err := treesitter.NewParser(language())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	s, err := p.ParseSnapshot(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if s.Root().HasError() || s.Root().EndByte() != uint32(len(src)) {
		t.Fatalf("root has_error=%v end=%d bytes=%d", s.Root().HasError(), s.Root().EndByte(), len(src))
	}
}
