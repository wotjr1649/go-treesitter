//go:build lang_postgres

package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	treesitter "github.com/wotjr1649/go-treesitter"
)

func TestPostgresSmallWorkload(t *testing.T) {
	src, err := os.ReadFile("testdata/postgres/rentals.sql")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(src)); got != "60832cb4371298e2c5cb3026e5bb70e80747da40e56bd6507087a1e2420c937e" {
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
