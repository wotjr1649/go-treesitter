package conformance

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
