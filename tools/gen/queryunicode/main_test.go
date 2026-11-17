package main

import (
	"bytes"
	"os"
	"testing"
)

func TestRustTableParser(t *testing.T) {
	tables, err := parseTables(`pub const SAMPLE: &'static [(char, char)] = &[('\0', '\u{1f}'), ('한', '힣'), ('\'', '\'')];`)
	if err == nil || tables != nil {
		t.Fatal("accepted unordered ranges")
	}
	tables, err = parseTables(`pub const SAMPLE: &'static [(char, char)] = &[('\0', '\u{1f}'), ('\'', '\''), ('한', '힣')];`)
	if err != nil {
		t.Fatal(err)
	}
	want := []span{{0, 31}, {39, 39}, {0xD55C, 0xD7A3}}
	if len(tables["SAMPLE"]) != len(want) {
		t.Fatalf("ranges = %v", tables)
	}
	for i, r := range want {
		if tables["SAMPLE"][i] != r {
			t.Fatalf("range %d = %v, want %v", i, tables["SAMPLE"][i], r)
		}
	}
}

func TestCrateHashRejectedBeforeParsing(t *testing.T) {
	path := t.TempDir() + "/crate"
	// A local archive is not trusted merely because it has the expected name.
	if err := os.WriteFile(path, []byte("not the pinned crate"), 0644); err != nil {
		t.Fatal(err)
	}
	err := run(path, t.TempDir(), true)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("crate SHA-256")) {
		t.Fatalf("error = %v", err)
	}
}
