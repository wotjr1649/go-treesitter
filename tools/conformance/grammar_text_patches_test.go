package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	treesitter "github.com/wotjr1649/go-treesitter"
	proc "github.com/wotjr1649/go-treesitter/tools/bench/proc"
)

// The shared fixture lets the native unpatched comparison exercise the same inputs as CI.
func TestScannerPatchesGrammarText(t *testing.T) {
	var fixtures map[string][]struct {
		Name, Source string
		Nodes        []string
		// Errors is the exact "kind text" list of ERROR nodes and Missing the kinds of MISSING nodes;
		// both empty means the tree must be error-free.
		Errors, Missing []string
	}
	if err := json.Unmarshal(readFile(t, "testdata/grammar-text-patches.json"), &fixtures); err != nil {
		t.Fatal(err)
	}
	kinds := map[string][]string{
		"go":         {"call_expression", "type_identifier", "int_literal", "binary_expression"},
		"rust":       {"block_comment", "inner_doc_comment_marker", "outer_doc_comment_marker", "doc_comment", "line_comment", "let_declaration", "function_item"},
		"javascript": {"jsx_text", "html_character_reference", "string", "binary_expression"},
		"html":       {"text", "entity", "attribute_value"},
	}
	for _, lang := range langs(t) {
		if len(kinds[lang]) == 0 {
			continue
		}
		if len(fixtures[lang]) == 0 {
			t.Fatalf("no grammar text patch inputs for %s", lang)
		}
		t.Run(lang, func(t *testing.T) {
			p := newParser(t, lang)
			for _, c := range fixtures[lang] {
				t.Run(c.Name, func(t *testing.T) {
					snap, err := p.ParseSnapshot(bg, []byte(c.Source))
					if err != nil {
						t.Fatal(err)
					}
					if snap.Root().HasError() != (len(c.Errors)+len(c.Missing) > 0) || snap.Root().EndByte() != uint32(len(c.Source)) {
						t.Fatalf("expected a complete tree with errors %q and missing %q\n%s", c.Errors, c.Missing, Dump(snap))
					}
					var got, errs, missing []string
					snap.Walk(func(n treesitter.Node, _ int) treesitter.WalkAction {
						if slices.Contains(kinds[lang], n.Kind()) {
							got = append(got, n.Kind()+" "+n.Text())
						}
						if n.IsError() {
							errs = append(errs, n.Kind()+" "+n.Text())
						}
						if n.IsMissing() {
							missing = append(missing, n.Kind())
						}
						return treesitter.Continue
					})
					if !slices.Equal(got, c.Nodes) {
						t.Fatalf("nodes = %q, want %q", got, c.Nodes)
					}
					if !slices.Equal(errs, c.Errors) {
						t.Fatalf("errors = %q, want %q", errs, c.Errors)
					}
					if !slices.Equal(missing, c.Missing) {
						t.Fatalf("missing = %q, want %q", missing, c.Missing)
					}
					t.Run("native", func(t *testing.T) {
						exe, ok := nativeExe(lang)
						if !ok {
							if os.Getenv("TSG_REQUIRE_FIELD_LOOKUP") == "1" {
								t.Fatalf("no native executable for %s (TSG_NATIVE_EXE_DIR=%q)", lang, os.Getenv("TSG_NATIVE_EXE_DIR"))
							}
							t.Skip("no native executable for " + lang)
						}
						file := filepath.Join(t.TempDir(), "input")
						if err := os.WriteFile(file, []byte(c.Source), 0600); err != nil {
							t.Fatal(err)
						}
						ctx, cancel := context.WithTimeout(bg, 30*time.Second)
						defer cancel()
						want, err := proc.CommandContext(ctx, exe, "dump", file).Output()
						if err != nil {
							t.Fatal(err)
						}
						if got := Dump(snap); !bytes.Equal(got, want) {
							t.Fatal(firstDiff(got, want))
						}
					})
				})
			}
		})
	}
}
