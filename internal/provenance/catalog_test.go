package provenance

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogLicenseInventory(t *testing.T) {
	data, err := os.ReadFile("../../LICENSES/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	type file struct{ Path, SHA256 string }
	var catalog struct {
		Rows []struct {
			Grammar, Repository, Commit, License, Distribution, Basis, Source string
			BlobSHA256                                                        string `json:"blob_sha256"`
			Notice                                                            file
			AdditionalEvidence                                                []file `json:"additional_evidence"`
		}
		StandardTexts        []file `json:"standard_texts"`
		CorrespondingSources []file `json:"corresponding_sources"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	check := func(f file) {
		t.Helper()
		if !filepath.IsLocal(f.Path) {
			t.Fatalf("nonlocal license path: %s", f.Path)
		}
		data, err := os.ReadFile(filepath.Join("../..", filepath.FromSlash(f.Path)))
		if err != nil || fmt.Sprintf("%x", sha256.Sum256(data)) != f.SHA256 {
			t.Fatalf("license/artifact identity changed: %s: %v", f.Path, err)
		}
	}
	seen := map[string]bool{}
	counts := map[string]int{}
	for _, row := range catalog.Rows {
		if seen[row.Grammar] || len(row.Commit) != 40 || row.License == "" ||
			!strings.Contains(row.Source, row.Commit) {
			t.Fatalf("invalid fixed license identity: %s", row.Grammar)
		}
		seen[row.Grammar] = true
		counts[row.Distribution]++
		if row.Basis != "original_notice" && row.Basis != "spdx_declaration" {
			t.Fatalf("unrecorded license basis: %s", row.Grammar)
		}
		if row.Distribution == "gpl" {
			if row.Grammar != "caddy" && row.Grammar != "disassembly" && row.Grammar != "jq" {
				t.Fatalf("unexpected optional grammar: %s", row.Grammar)
			}
			if _, err := os.Stat("../runtime/grammars/grammar_blobs/" + row.Grammar + ".bin"); !os.IsNotExist(err) {
				t.Fatalf("GPL blob entered base carrier: %s", row.Grammar)
			}
			continue // The separate module owns and validates these notice bytes.
		}
		if strings.Contains(row.License, "GPL") {
			t.Fatalf("GPL license entered base distribution: %s", row.Grammar)
		}
		check(row.Notice)
		for _, f := range row.AdditionalEvidence {
			check(f)
		}
		check(file{"internal/runtime/grammars/grammar_blobs/" + row.Grammar + ".bin", row.BlobSHA256})
	}
	if len(seen) != 206 || counts["main"] != 203 || counts["gpl"] != 3 {
		t.Fatalf("catalog distribution changed: %v", counts)
	}
	for _, f := range append(catalog.StandardTexts, catalog.CorrespondingSources...) {
		check(f)
	}
}
