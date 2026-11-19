package conformance

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	treesitter "github.com/wotjr1649/go-treesitter"
	proc "github.com/wotjr1649/go-treesitter/tools/bench/proc"
)

// JSON stores byte slices as base64, preserving malformed UTF-8 and line endings.
func scannerPatchInputs(t *testing.T, group string) map[string][]byte {
	t.Helper()
	path := filepath.Join("testdata", "scanner-patches", group+".json")
	var data []byte
	if group == "yaml" || group == "python" {
		path += ".gz"
		z, err := gzip.NewReader(bytes.NewReader(readFile(t, path)))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		defer z.Close()
		data, err = io.ReadAll(z)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	} else {
		data = readFile(t, path)
	}
	var cases map[string][]byte
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if len(cases) == 0 {
		t.Fatalf("%s: no scanner patch inputs", path)
	}
	return cases
}

func scannerPatchInput(t *testing.T, cases map[string][]byte, name string) []byte {
	t.Helper()
	src, ok := cases[name]
	if !ok {
		t.Fatalf("missing scanner patch input %q", name)
	}
	return src
}

// Keep the ADR-0023 scanner fixes exercised beyond the upstream corpora.
func TestScannerPatches(t *testing.T) {
	for _, lang := range langs(t) {
		if lang != "bash" && lang != "ruby" && lang != "yaml" && lang != "python" {
			continue
		}
		t.Run(lang, func(t *testing.T) {
			exe, ok := nativeExe(lang)
			if !ok {
				t.Skip("no native executable for " + lang)
			}
			cases := scannerPatchInputs(t, lang)
			topLevel := map[string]int{} // cases whose heredoc must swallow every body line
			clean := map[string]bool{}   // cases that must parse without ERROR or MISSING nodes
			pairs := map[string]int{}    // cases with an exact block_mapping_pair count
			switch lang {
			case "bash":
				// An ASCII delimiter closes only on ASCII: the body line shares the low bytes of the delimiter.
				topLevel["ascii-delimiter-alias-line"] = 2 // redirected_statement, echo
			case "ruby":
				topLevel["ascii-delimiter-alias-line"] = 3 // assignment, heredoc_body, puts
			case "yaml":
				clean["253"] = true // the 0002 bound; deeper inputs are a documented ERROR
				// 0003: rows past 32,767 and 65,535 (int16_t overflow), the 12-byte header at the 0002 bound,
				// and a blk_imp_row more than 32,767 rows behind (saturated serialized distance).
				for _, name := range []string{
					"late-pair-32769",
					"key-then-blank-gap-40000",
					"alias-after-32768",
					"pair-gap-65535-blank", // one scan jumps exactly 65,536 rows
					"explicit-key-gap-32768",
					"explicit-key-gap-40000",
					"explicit-key-gap-70000", // control: the int16 wrap already gave the right tree
					// CR and LF each count as a row, so ":" on line 16,385 is scanner row 32,768.
					"crlf-explicit-key-gap-16383",
					// These three pin the serialized blk_imp_row against wrap, zero and absolute int16 encoding.
					"explicit-seq-gap-40000",
					"saturated-dist-wrap-65536",
					"anchored-keys-row-40000",
					"flat-32769",       // upstream issue #49
					"deep-mapping-252", // 253 indent pairs: the 12-byte header still fits them
					"deep-seq-253-late",
				} {
					clean[name] = true
				}
				for _, name := range []string{"explicit-key-gap-32768", "explicit-key-gap-40000", "explicit-key-gap-70000", "crlf-explicit-key-gap-16383"} {
					pairs[name] = 1 // before 0003, 32768, 40000 and the CRLF case gave a nested block_mapping with a second pair; 70000 was already right
				}
			}
			p := newParser(t, lang)
			for name, src := range cases {
				t.Run(name, func(t *testing.T) {
					snap, err := p.ParseSnapshot(bg, src)
					if err != nil {
						t.Fatal(err)
					}
					if snap.Root().EndByte() != uint32(len(src)) {
						t.Fatal("truncated root")
					}
					if (lang == "bash" || lang == "ruby") && snap.Root().HasError() {
						t.Fatal("heredoc did not close cleanly")
					}
					if clean[name] && snap.Root().HasError() {
						t.Fatal("error-free input produced an ERROR tree")
					}
					if want, ok := pairs[name]; ok {
						got := 0
						snap.Walk(func(n treesitter.Node, _ int) treesitter.WalkAction {
							if n.Kind() == "block_mapping_pair" {
								got++
							}
							return treesitter.Continue
						})
						if got != want {
							t.Fatalf("%d block_mapping_pair nodes, want %d", got, want)
						}
					}
					if n, ok := topLevel[name]; ok && snap.Root().NamedChildCount() != n {
						t.Fatalf("heredoc closed early: %d top-level nodes, want %d", snap.Root().NamedChildCount(), n)
					}
					file := filepath.Join(t.TempDir(), "input")
					if err := os.WriteFile(file, src, 0600); err != nil {
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
			}
		})
	}
}

// End-node positions detect Ruby's error-free swallowing and both scanners' low-byte aliases.
func TestHeredocUTF8(t *testing.T) {
	for _, lang := range langs(t) {
		if lang != "bash" && lang != "ruby" {
			continue
		}
		t.Run(lang, func(t *testing.T) {
			p := newParser(t, lang)
			type heredocCase struct {
				name, src string
				ends      []string
			}
			var cases []heredocCase
			expectedEnds := map[string][]uint32{}
			expectedError := map[string]bool{}
			inputs := scannerPatchInputs(t, lang+"-heredoc")
			add := func(name string, words ...string) {
				cases = append(cases, heredocCase{name, string(scannerPatchInput(t, inputs, name)), words})
			}
			for _, word := range []string{"끝", "종료", "한글끝", "END한글9", "가", "걁", "é", "Ā", "😀"} {
				add(fmt.Sprintf("quoted-%x", word), word)
			}
			add("unquoted", "끝")
			add("double-quoted", "종료")
			add("dash", "한글끝")
			add("ascii-alias", "A")
			add("invalid-alias", "\xfe")
			add("replacement-is-valid", "�")
			add("invalid-error-counts-short-line", "\xfe\xff")
			add("prefix-line-exact", "끝")
			if lang == "bash" {
				for _, word := range []string{"EOF", "끝"} {
					for _, opening := range []string{"<<" + word, "<<'" + word + "'", "<<\"" + word + "\"", "<<-" + word} {
						add(fmt.Sprintf("exact-line-%x-%s", word, opening), word)
					}
				}
				// Existing upstream limit: plain <<EOF ignores leading spaces/tabs, unlike Bash.
				for _, indent := range []string{" ", "\t"} {
					name := fmt.Sprintf("plain-leading-whitespace-known-close-%x", indent)
					add(name, "EOF")
					expectedEnds[name] = []uint32{uint32(strings.Index(cases[len(cases)-1].src, "\n"+indent+"EOF") + 1 + len(indent+"EOF"))}
				}
				add("prefix-before-expansions", "EOF")
				add("suffix-after-substitution", "EOF")
				add("embedded-cr-prefix", "EOF")
				for _, word := range []string{"EOF", "끝"} {
					name := fmt.Sprintf("crlf-prefix-%x", word)
					add(name, word)
					c := &cases[len(cases)-1]
					expectedEnds[name] = []uint32{uint32(strings.LastIndex(c.src, word) + len(word) + 1)}
				}
				add("no-final-newline", "EOF")
			}
			add("dash-wide-space", "끝")
			if lang == "bash" {
				// Existing upstream bug: <<- strips iswspace, although Bash permits only tabs.
				expectedEnds["dash-wide-space"] = []uint32{uint32(strings.Index(cases[len(cases)-1].src, "　끝") + len("　끝"))}
			}
			lastFit := 337
			if lang == "ruby" {
				lastFit = 338
			}
			word := strings.Repeat("끝", lastFit)
			add("serialization-last-fitting-word", word)
			word += "끝"
			add("serialization-overflow-known-state-loss", word)
			// Overflow also loses state on fresh parses; Ruby can still report HasError=false.
			expectedEnds["serialization-overflow-known-state-loss"] = nil
			if lang == "bash" {
				expectedError["serialization-overflow-known-state-loss"] = true
				expectedEnds["serialization-overflow-known-state-loss"] = []uint32{uint32(len(cases[len(cases)-1].src))} // missing end at EOF
			}
			add("crlf", "끝")
			if lang == "bash" {
				// The exact-line lookahead consumes CR before accepting LF.
				expectedEnds["crlf"] = []uint32{uint32(strings.LastIndex(cases[len(cases)-1].src, "끝") + len("끝\r"))}
			}
			if lang == "ruby" {
				add("squiggly", "끝")
				add("repeated-word-known-ruby-close", "끝")
				// Existing upstream bug: Ruby accepts repeated complete end words on one line.
				expectedEnds["repeated-word-known-ruby-close"] = []uint32{uint32(strings.Index(cases[len(cases)-1].src, "\n끝끝") + 1 + len("끝끝"))}
				add("multiple", "끝", "종료")
			}
			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) {
					snap, err := p.ParseSnapshot(bg, []byte(c.src))
					if err != nil {
						t.Fatal(err)
					}
					if snap.Root().HasError() != expectedError[c.name] || snap.Root().EndByte() != uint32(len(c.src)) {
						t.Fatalf("heredoc error=%v end=%d, want error=%v end=%d", snap.Root().HasError(), snap.Root().EndByte(), expectedError[c.name], len(c.src))
					}
					var got, want []uint32
					substitutions := 0
					snap.Walk(func(n treesitter.Node, _ int) treesitter.WalkAction {
						if n.Kind() == "command_substitution" {
							substitutions++
						}
						if n.Kind() == "heredoc_end" {
							got = append(got, n.EndByte())
						}
						return treesitter.Continue
					})
					if c.name == "prefix-before-expansions" && substitutions != 2 {
						t.Fatalf("lost command substitutions after delimiter prefix: got %d, want 2", substitutions)
					}
					for _, word := range c.ends {
						want = append(want, uint32(strings.LastIndex(c.src, word)+len(word)))
					}
					if ends, ok := expectedEnds[c.name]; ok {
						want = ends
					}
					if !slices.Equal(got, want) {
						t.Fatalf("heredoc ends at %v, want %v", got, want)
					}
					if exe, ok := nativeExe(lang); ok {
						file := filepath.Join(t.TempDir(), "input")
						if err := os.WriteFile(file, []byte(c.src), 0600); err != nil {
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
					}
				})
			}
		})
	}
}

func TestHeredocUTF8Incremental(t *testing.T) {
	for _, lang := range langs(t) {
		if lang != "bash" && lang != "ruby" {
			continue
		}
		t.Run(lang, func(t *testing.T) {
			old := "cat <<한글끝\n한글끝났다\nfirst $value\nsecond\n한글끝\necho after\n"
			if lang == "ruby" {
				old = "x = <<~한글끝\nfirst #{1}\nsecond\n한글끝\nputs 1\n"
			}
			replacement := "둘째"
			if lang == "bash" {
				replacement = "한글끝추가"
			}
			next := strings.Replace(old, "second", replacement, 1)
			start := uint32(strings.Index(old, "second"))
			oldEnd, newEnd := start+uint32(len("second")), start+uint32(len(replacement))
			p := newParser(t, lang)
			oldTree, err := p.Parse(bg, []byte(old), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer oldTree.Close()
			if err := oldTree.Edit(treesitter.EditFor([]byte(old), []byte(next), start, oldEnd, newEnd)); err != nil {
				t.Fatal(err)
			}
			inc, err := p.Parse(bg, []byte(next), oldTree)
			if err != nil {
				t.Fatal(err)
			}
			defer inc.Close()
			snap, err := inc.Snapshot(bg)
			if err != nil {
				t.Fatal(err)
			}
			fresh, err := p.ParseSnapshot(bg, []byte(next))
			if err != nil {
				t.Fatal(err)
			}
			if snap.Root().HasError() || !bytes.Equal(Dump(snap), Dump(fresh)) {
				t.Fatal("incremental heredoc differs from a clean fresh parse")
			}
			// Both trees can be wrong the same way, so also pin heredoc_end to the end of the closing line.
			wantEnd := []uint32{uint32(strings.LastIndex(next, "한글끝") + len("한글끝"))}
			for name, walk := range map[string]func(func(treesitter.Node, int) treesitter.WalkAction){"incremental": snap.Walk, "fresh": fresh.Walk} {
				var got []uint32
				walk(func(n treesitter.Node, _ int) treesitter.WalkAction {
					if n.Kind() == "heredoc_end" {
						got = append(got, n.EndByte())
					}
					return treesitter.Continue
				})
				if !slices.Equal(got, wantEnd) {
					t.Fatalf("%s heredoc ends at %v, want %v", name, got, wantEnd)
				}
			}
			if exe, ok := nativeExe(lang); ok {
				dir := t.TempDir()
				for name, src := range map[string]string{"old": old, "new": next} {
					if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0600); err != nil {
						t.Fatal(err)
					}
				}
				args := []string{filepath.Join(dir, "old"), filepath.Join(dir, "new"), strconv.Itoa(int(start)), strconv.Itoa(int(oldEnd)), strconv.Itoa(int(newEnd))}
				ctx, cancel := context.WithTimeout(bg, 30*time.Second)
				defer cancel()
				want, err := proc.CommandContext(ctx, exe, append([]string{"edit"}, args...)...).Output()
				if err != nil {
					t.Fatal(err)
				}
				if got := Dump(snap); !bytes.Equal(got, want) {
					t.Fatal(firstDiff(got, want))
				}
				ranges, err := inc.ChangedRanges(oldTree)
				if err != nil {
					t.Fatal(err)
				}
				var got strings.Builder
				for _, r := range ranges {
					fmt.Fprintf(&got, "%d %d %d %d %d %d\n", r.StartByte, r.EndByte, r.StartPoint.Row, r.StartPoint.Column, r.EndPoint.Row, r.EndPoint.Column)
				}
				want, err = proc.CommandContext(ctx, exe, append([]string{"changed-ranges"}, args...)...).Output()
				if err != nil {
					t.Fatal(err)
				}
				if got.String() != string(want) {
					t.Fatalf("changed ranges %q, native %q", got.String(), want)
				}
			}
		})
	}
}
