package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFieldFullCompareShim(t *testing.T) {
	product, err := os.ReadFile(filepath.Join("shim", "tsg_abi.c"))
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	p, err := fullCompareShim(opts{root: ".", work: work})
	if err != nil {
		t.Fatal(err)
	}
	variant, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(variant, []byte(fullCompareGuard)) || !bytes.Contains(variant, []byte("TSG_FIELDS_FULL_COMPARE")) {
		t.Fatal("pre-check call not replaced")
	}
	// Exactly one line differs: restoring the guard gives back the product shim byte for byte.
	back := bytes.Replace(variant, []byte("  /* TSG_FIELDS_FULL_COMPARE: pre-check removed */\n"), []byte(fullCompareGuard), 1)
	if !bytes.Equal(back, product) {
		t.Fatal("variant differs from the product shim in more than the pre-check call")
	}
	for name, src := range map[string]string{"absent": "int x;\n", "twice": fullCompareGuard + fullCompareGuard} {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "shim"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "shim", "tsg_abi.c"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := fullCompareShim(opts{root: root, work: work}); err == nil || !strings.Contains(err.Error(), "exactly once") {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestFieldFullCompareOutputBoundary(t *testing.T) {
	repo := t.TempDir()
	o := opts{repo: repo, out: filepath.Join(repo, "grammars", "json"), work: filepath.Join(repo, "work")}
	err := validateTestOnlyOutput(o, "-field-full-compare")
	if err == nil || !strings.Contains(err.Error(), "-field-full-compare") {
		t.Fatalf("product output accepted or flag not named: %v", err)
	}
}
