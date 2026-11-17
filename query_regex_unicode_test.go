package treesitter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"testing"
	"unicode"
)

// The fixture lists every scalar-value difference for every Go category/script,
// including scripts added after Rust's Unicode version. Hashes bind the complete
// Rust sets, so removing an inconvenient delta also makes this check fail.
func TestQueryRegexUnicodePropertyDifferences(t *testing.T) {
	var fixture struct {
		GoUnicode   string `json:"go_unicode"`
		RustUnicode string `json:"rust_unicode"`
		CrateSHA256 string `json:"crate_sha256"`
		Properties  []struct {
			Name        string                  `json:"name"`
			Kind        string                  `json:"kind"`
			RustPresent bool                    `json:"rust_present"`
			RustSHA256  string                  `json:"rust_sha256"`
			GoOnly      []struct{ Lo, Hi rune } `json:"go_only"`
			RustOnly    []struct{ Lo, Hi rune } `json:"rust_only"`
		} `json:"properties"`
	}
	b, err := os.ReadFile("testdata/query-regex-unicode.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.RustUnicode != "16.0.0" || fixture.CrateSHA256 != "d6f6ff9a378485b298a5286656da665ba74413d36db0979633275d2e708145d4" {
		t.Fatal("unexpected Rust source identity")
	}
	if fixture.GoUnicode != unicode.Version {
		t.Skipf("recorded property differences use Go Unicode %s; this toolchain uses %s; \\p follows the current toolchain", fixture.GoUnicode, unicode.Version)
	}
	if len(fixture.Properties) != len(unicode.Categories)+len(unicode.Scripts) {
		t.Fatal("property list is incomplete")
	}
	seen := map[string]bool{}
	for _, p := range fixture.Properties {
		key := p.Kind + ":" + p.Name
		if seen[key] {
			t.Fatalf("duplicate property %s", key)
		}
		seen[key] = true
		table := unicode.Categories[p.Name]
		if p.Kind == "script" {
			table = unicode.Scripts[p.Name]
		} else if p.Kind != "category" {
			t.Fatalf("unknown kind %s", key)
		}
		if table == nil {
			t.Fatalf("missing Go property %s", key)
		}
		t.Run(key, func(t *testing.T) {
			points := map[rune]bool{}
			for _, r := range table.R16 {
				for c := rune(r.Lo); c <= rune(r.Hi); c += rune(r.Stride) {
					if c < 0xD800 || c > 0xDFFF {
						points[c] = true
					}
				}
			}
			for _, r := range table.R32 {
				for c := rune(r.Lo); c <= rune(r.Hi); c += rune(r.Stride) {
					points[c] = true
				}
			}
			for _, r := range p.GoOnly {
				for c := r.Lo; c <= r.Hi; c++ {
					if !points[c] {
						t.Fatalf("Go-only U+%04X is absent from Go", c)
					}
					delete(points, c)
				}
			}
			for _, r := range p.RustOnly {
				for c := r.Lo; c <= r.Hi; c++ {
					if points[c] {
						t.Fatalf("Rust-only U+%04X is already in Go", c)
					}
					points[c] = true
				}
			}
			chars := make([]rune, 0, len(points))
			for c := range points {
				chars = append(chars, c)
			}
			sort.Slice(chars, func(i, j int) bool { return chars[i] < chars[j] })
			h := sha256.New()
			for _, c := range chars {
				fmt.Fprintf(h, "%06X\n", c)
			}
			if got := hex.EncodeToString(h.Sum(nil)); got != p.RustSHA256 {
				t.Fatalf("reconstructed Rust table SHA-256 = %s, want %s; regenerate for this Go Unicode version", got, p.RustSHA256)
			}
			if !p.RustPresent && len(chars) != 0 {
				t.Fatal("absent Rust property has a nonempty table")
			}
		})
	}
}

func TestQueryRegexUnicodeTables(t *testing.T) {
	for _, table := range [][]regexRange{rustWord, rustDecimal, rustSpace} {
		for i, r := range table {
			if r.lo > r.hi || (i > 0 && table[i-1].hi >= r.lo) {
				t.Fatalf("unsorted range %d: %v", i, r)
			}
		}
	}
	for i, p := range rustFold {
		if p.from == p.to || (i > 0 && (rustFold[i-1].from > p.from || (rustFold[i-1].from == p.from && rustFold[i-1].to >= p.to))) {
			t.Fatalf("unsorted fold %d: %v", i, p)
		}
		j := sort.Search(len(rustFold), func(j int) bool { return rustFold[j].from >= p.to })
		found := false
		for ; j < len(rustFold) && rustFold[j].from == p.to; j++ {
			if rustFold[j].to == p.from {
				found = true
			}
		}
		if !found {
			t.Fatalf("fold U+%04X -> U+%04X has no reverse", p.from, p.to)
		}
	}
}
