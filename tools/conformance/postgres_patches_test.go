package conformance

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	treesitter "github.com/wotjr1649/go-treesitter"
	proc "github.com/wotjr1649/go-treesitter/tools/bench/proc"
)

// The TestScannerPatches prefix keeps these grammar and scanner regressions in CI.
func TestScannerPatchesPostgresUTF8(t *testing.T) {
	if !slices.Contains(langs(t), "postgres") {
		t.Skip("postgres is not in this shard")
	}
	p := newParser(t, "postgres")
	cases := []struct {
		name, kind, text string
		hasError         bool
	}{
		{"hangul", "identifier", "값", false},
		{"keyword-adjacent", "identifier", "SELECT값", false},
		{"ascii", "identifier", "_value9$", false},
		{"latin1", "identifier", "café", false},
		{"nfd", "identifier", "가", false},
		{"combining", "identifier", "a\u0301", false},
		{"cjk", "identifier", "名称", false},
		{"cyrillic", "identifier", "имя", false},
		{"supplementary", "identifier", "😀", false},
		{"digit-hangul-alias", "identifier", "값", false},
		{"digit-ascii-alias", "BareColLabel", "name", false},
		{"digit-hangul-dollar-alias", "identifier", "끝$body$9끝$", false},
		{"digit-ascii-dollar-alias", "identifier", "END$body$9END$", false},
		{"wide-space-only-name", "identifier", "　", false},
		{"wide-space-start-name", "identifier", "　값", false},
		{"wide-space-standalone-name", "identifier", "SELECT　값", true},
		{"wide-space-before-from", "columnref", "a　FROM", false},
		{"wide-space-before-from-alias", "BareColLabel", "t", false},
		{"wide-space-inside-name", "identifier", "a　b", false},
		{"wide-space-after-keyword", "identifier", "SELECT　값", false},
		{"quoted", "quoted_identifier", "\"한 글\"", false},
		{"dollar-empty", "dollar_quoted_string", "$$한글$$", false},
		{"dollar-ascii", "dollar_quoted_string", "$END$body$END$", false},
		{"dollar-hangul", "dollar_quoted_string", "$끝$본문$끝$", false},
		{"dollar-mixed", "dollar_quoted_string", "$END한글9$body$END한글9$", false},
		{"dollar-nfd", "dollar_quoted_string", "$가$body$가$", false},
		{"dollar-supplementary", "dollar_quoted_string", "$😀$body$😀$", false},
		{"dollar-hangul-ascii-alias", "dollar_quoted_string", "$걁$body$A$still$걁$", false},
		{"dollar-ascii-hangul-alias", "dollar_quoted_string", "$A$body$걁$still$A$", false},
		{"dollar-hangul-hangul-alias", "dollar_quoted_string", "$끝$body$굝$still$끝$", false},
		{"dollar-mismatch", "dollar_quoted_string", "", true},
		{"dollar-low-byte-mismatch", "dollar_quoted_string", "", true},
		{"dollar-case-mismatch", "dollar_quoted_string", "", true},
		{"dollar-63-code-points", "dollar_quoted_string", "$" + strings.Repeat("끝", 63) + "$body$" + strings.Repeat("끝", 63) + "$", false},
		{"dollar-64-known-upstream-limit", "dollar_quoted_string", "", true},
	}
	inputs := scannerPatchInputs(t, "postgres")
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := scannerPatchInput(t, inputs, c.name)
			snap, err := p.ParseSnapshot(bg, src)
			if err != nil {
				t.Fatal(err)
			}
			if snap.Root().HasError() != c.hasError || snap.Root().EndByte() != uint32(len(src)) {
				t.Fatalf("error=%v end=%d, want error=%v end=%d", snap.Root().HasError(), snap.Root().EndByte(), c.hasError, len(src))
			}
			var got []string
			snap.Walk(func(n treesitter.Node, _ int) treesitter.WalkAction {
				if n.Kind() == c.kind {
					got = append(got, n.Text())
				}
				return treesitter.Continue
			})
			var want []string
			if c.text != "" {
				want = []string{c.text}
			}
			if !slices.Equal(got, want) {
				t.Fatalf("%s text=%q, want %q", c.kind, got, want)
			}
			if exe, ok := nativeExe("postgres"); ok {
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
				if !bytes.Equal(Dump(snap), want) {
					t.Fatal(firstDiff(Dump(snap), want))
				}
			}
		})
	}
}

func TestScannerPatchesPostgresUTF8Incremental(t *testing.T) {
	if !slices.Contains(langs(t), "postgres") {
		t.Skip("postgres is not in this shard")
	}
	old := "SELECT $끝$first\nsecond$끝$; SELECT 값;\n"
	next := strings.Replace(old, "second", "둘째$A$", 1)
	start := uint32(strings.Index(old, "second"))
	oldEnd, newEnd := start+uint32(len("second")), start+uint32(len("둘째$A$"))
	p := newParser(t, "postgres")
	tree, err := p.Parse(bg, []byte(old), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()
	if err := tree.Edit(treesitter.EditFor([]byte(old), []byte(next), start, oldEnd, newEnd)); err != nil {
		t.Fatal(err)
	}
	inc, err := p.Parse(bg, []byte(next), tree)
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
		t.Fatal("incremental PostgreSQL differs from clean fresh tree")
	}
	if exe, ok := nativeExe("postgres"); ok {
		dir := t.TempDir()
		for name, src := range map[string]string{"old": old, "new": next} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0600); err != nil {
				t.Fatal(err)
			}
		}
		ctx, cancel := context.WithTimeout(bg, 30*time.Second)
		defer cancel()
		args := []string{"edit", filepath.Join(dir, "old"), filepath.Join(dir, "new"), fmt.Sprint(start), fmt.Sprint(oldEnd), fmt.Sprint(newEnd)}
		want, err := proc.CommandContext(ctx, exe, args...).Output()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(Dump(snap), want) {
			t.Fatal(firstDiff(Dump(snap), want))
		}
	}
}
