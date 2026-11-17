package conformance

import (
	"bytes"
	"context"
	"fmt"
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
			cases := map[string]string{}
			topLevel := map[string]int{} // cases whose heredoc must swallow every body line
			switch lang {
			case "bash", "ruby":
				// An ASCII delimiter closes only on ASCII: the body line shares the low bytes of the delimiter.
				if lang == "bash" {
					topLevel["ascii-delimiter-alias-line"] = 2 // redirected_statement, echo
					cases["ascii-delimiter-alias-line"] = "cat <<EOF\nline1\nŅŏņ\nline3 still body\nEOF\necho after\n"
				} else {
					topLevel["ascii-delimiter-alias-line"] = 3 // assignment, heredoc_body, puts
					cases["ascii-delimiter-alias-line"] = "x = <<~EOS\n  line1\n  Ņŏœ\n  line3 still body\nEOS\nputs 1\n"
				}
				for _, delimiter := range []string{"EOF", "é", "€", "\xba"} {
					if lang == "bash" {
						cases[fmt.Sprintf("delimiter-%x", delimiter)] = "cat <<'" + delimiter + "'\nhello\n" + delimiter + "\nls\n"
					} else {
						cases[fmt.Sprintf("delimiter-%x", delimiter)] = "x = <<~'" + delimiter + "'\n  hello\n" + delimiter + "\ny = 1\n"
					}
				}
			case "yaml":
				for _, depth := range []int{253, 254, 300, 600} {
					cases[fmt.Sprint(depth)] = strings.Repeat("- ", depth) + "x\n"
				}
			case "python":
				for _, depth := range []int{510, 511, 520, 600} {
					var prefix strings.Builder
					for i := 0; i < depth; i++ {
						prefix.WriteString(strings.Repeat(" ", i) + "if True:\n")
					}
					for _, literal := range []string{"'a'", `"a"`, `b"a"`, `f"a"`, `"""a"""`, "1"} {
						cases[fmt.Sprintf("%d-%s", depth, literal)] = prefix.String() + strings.Repeat(" ", depth) + "y = " + literal + "\n"
					}
				}
			}
			p := newParser(t, lang)
			for name, src := range cases {
				t.Run(name, func(t *testing.T) {
					snap, err := p.ParseSnapshot(bg, []byte(src))
					if err != nil {
						t.Fatal(err)
					}
					if snap.Root().EndByte() != uint32(len(src)) {
						t.Fatal("truncated root")
					}
					if (lang == "bash" || lang == "ruby") && snap.Root().HasError() {
						t.Fatal("heredoc did not close cleanly")
					}
					if n, ok := topLevel[name]; ok && snap.Root().NamedChildCount() != n {
						t.Fatalf("heredoc closed early: %d top-level nodes, want %d", snap.Root().NamedChildCount(), n)
					}
					file := filepath.Join(t.TempDir(), "input")
					if err := os.WriteFile(file, []byte(src), 0600); err != nil {
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
			add := func(name, opening, body, closing, word string) {
				prefix, suffix := "cat ", "echo after\n"
				if lang == "ruby" {
					prefix, suffix = "x = ", "puts 1\n"
				}
				cases = append(cases, heredocCase{name, prefix + opening + "\n" + body + closing + "\n" + suffix, []string{word}})
			}
			for _, word := range []string{"끝", "종료", "한글끝", "END한글9", "가", "걁", "é", "Ā", "😀"} {
				add(fmt.Sprintf("quoted-%x", word), "<<'"+word+"'", "A\n굁\nbody\n", word, word)
			}
			add("unquoted", "<<끝", "body\n", "끝", "끝")
			add("double-quoted", `<<"종료"`, "body\n", "종료", "종료")
			add("dash", "<<-'한글끝'", "\tbody\n", "\t한글끝", "한글끝")
			add("ascii-alias", "<<'A'", "걁\nbody\n", "A", "A")
			add("invalid-alias", "<<'\xff'", "�\nbody\n", "\xfe", "\xfe")
			add("replacement-is-valid", "<<'�'", "\xff\nbody\n", "�", "�")
			add("invalid-error-counts-short-line", "<<'\xff\xfe'", "\xfd\nbody\n", "\xfe\xff", "\xfe\xff")
			prefixName := "prefix-line-exact"
			if lang == "bash" {
				prefixName = "prefix-line-known-bash-close"
			}
			add(prefixName, "<<'끝'", "끝났다\nbody\n", "끝", "끝")
			if lang == "bash" {
				// Existing upstream bug: Bash closes at a prefix; this patch does not repair it.
				expectedError[prefixName] = true
				expectedEnds[prefixName] = []uint32{uint32(strings.Index(cases[len(cases)-1].src, "\n끝났다") + 1 + len("끝"))}
			}
			add("dash-wide-space", "<<-'끝'", "　끝\nbody\n", "끝", "끝")
			if lang == "bash" {
				// Existing upstream bug: <<- strips iswspace, although Bash permits only tabs.
				expectedEnds["dash-wide-space"] = []uint32{uint32(strings.Index(cases[len(cases)-1].src, "　끝") + len("　끝"))}
			}
			lastFit := 337
			if lang == "ruby" {
				lastFit = 338
			}
			word := strings.Repeat("끝", lastFit)
			add("serialization-last-fitting-word", "<<'"+word+"'", "body\n", word, word)
			word += "끝"
			add("serialization-overflow-known-state-loss", "<<'"+word+"'", "body\n", word, word)
			// Overflow also loses state on fresh parses; Ruby can still report HasError=false.
			expectedEnds["serialization-overflow-known-state-loss"] = nil
			if lang == "bash" {
				expectedError["serialization-overflow-known-state-loss"] = true
				expectedEnds["serialization-overflow-known-state-loss"] = []uint32{uint32(len(cases[len(cases)-1].src))} // missing end at EOF
			}
			add("crlf", "<<'끝'", "body\n", "끝", "끝")
			// The complete input uses CRLF, including the opening and following statement.
			cases[len(cases)-1].src = strings.ReplaceAll(cases[len(cases)-1].src, "\n", "\r\n")
			if lang == "ruby" {
				add("squiggly", "<<~'끝'", "  body\n", "  끝", "끝")
				add("repeated-word-known-ruby-close", "<<'끝'", "끝끝\nbody\n", "끝", "끝")
				// Existing upstream bug: Ruby accepts repeated complete end words on one line.
				expectedEnds["repeated-word-known-ruby-close"] = []uint32{uint32(strings.Index(cases[len(cases)-1].src, "\n끝끝") + 1 + len("끝끝"))}
				cases = append(cases, heredocCase{"multiple", "x = [<<'끝', <<~'종료']\nfirst\n끝\n  second\n  종료\nputs 1\n", []string{"끝", "종료"}})
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
					snap.Walk(func(n treesitter.Node, _ int) treesitter.WalkAction {
						if n.Kind() == "heredoc_end" {
							got = append(got, n.EndByte())
						}
						return treesitter.Continue
					})
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
			old := "cat <<한글끝\nfirst $value\nsecond\n한글끝\necho after\n"
			if lang == "ruby" {
				old = "x = <<~한글끝\nfirst #{1}\nsecond\n한글끝\nputs 1\n"
			}
			next := strings.Replace(old, "second", "둘째", 1)
			start := uint32(strings.Index(old, "second"))
			oldEnd, newEnd := start+uint32(len("second")), start+uint32(len("둘째"))
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
