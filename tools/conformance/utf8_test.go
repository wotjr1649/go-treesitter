package conformance

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"unicode/utf8"

	treesitter "github.com/wotjr1649/go-treesitter"
)

// nativeDump runs the native reference dumper (native/exe2/<lang>.exe dump <file>).
func nativeDump(t *testing.T, root, lang string, src []byte) []byte {
	t.Helper()
	f := filepath.Join(t.TempDir(), "in.bin")
	if err := os.WriteFile(f, src, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(filepath.Join(root, "native", "exe2", lang+".exe"), "dump", f).Output()
	if err != nil {
		t.Fatalf("native dump of a %d-byte input: %v", len(src), err)
	}
	return out
}

// TestInvalidUTF8VsNative: the corpus holds no invalid UTF-8, so this derives inputs with every kind of invalid or
// unusual byte sequence from a corpus file. The tree must equal the native C tree byte for byte (C decodes invalid
// bytes into error nodes; a leading BOM is skipped), HasInvalidUTF8 is compared with utf8.Valid and WithStrictUTF8
// must reject exactly the inputs Go calls invalid.
func TestInvalidUTF8VsNative(t *testing.T) {
	root := dataRoot(t)
	cases := []struct {
		name string
		ins  string
	}{
		{"ff", "\xff"}, {"truncated-3byte", "\xe2\x82"}, {"overlong-c0", "\xc0\x80"}, {"surrogate", "\xed\xa0\x80"},
		{"above-10ffff", "\xf4\x90\x80\x80"}, {"lone-continuation", "\x80"}, {"bom", "\xef\xbb\xbf"}, {"nul", "\x00"},
		{"noncharacter-valid", "\xef\xbf\xbe"},
	}
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			base, _ := smallReference(t, root, lang)
			p := newParser(t, lang)
			strict := newParser(t, lang, treesitter.WithStrictUTF8())
			mid := 0
			for i, c := range base {
				if c == '\n' {
					mid = i + 1
					break
				}
			}
			var run, mism, flagDiff int
			for _, c := range cases {
				for _, where := range []string{"end", "start", "after-first-line"} {
					var src []byte
					switch where {
					case "end":
						src = append(append([]byte{}, base...), c.ins...)
					case "start":
						src = append([]byte(c.ins), base...)
					default:
						src = append(append(append([]byte{}, base[:mid]...), c.ins...), base[mid:]...)
					}
					run++
					id := c.name + "/" + where
					tree, err := p.Parse(bg, src, nil)
					if err != nil {
						mism++
						t.Errorf("%s: Parse: %v", id, err)
						continue
					}
					snap, err := tree.Snapshot(bg)
					if err != nil {
						mism++
						t.Errorf("%s: Snapshot: %v", id, err)
						tree.Close()
						continue
					}
					got, want := Dump(snap), nativeDump(t, root, lang, src)
					if string(got) != string(want) {
						mism++
						t.Errorf("%s: dump differs from native C: %s", id, firstDiff(got, want))
						preserve("utf8/"+lang+"/"+c.name+"-"+where+".go.dump", got)
					}
					valid := utf8.Valid(src)
					if tree.HasInvalidUTF8() == valid {
						flagDiff++
						t.Errorf("%s: HasInvalidUTF8=%v but utf8.Valid=%v", id, tree.HasInvalidUTF8(), valid)
					}
					tree.Close()
					_, serr := strict.Parse(bg, src, nil)
					if valid != (serr == nil) || (serr != nil && !errors.Is(serr, treesitter.ErrInvalidInput)) {
						mism++
						t.Errorf("%s: strict mode result %v for utf8.Valid=%v", id, serr, valid)
					}
				}
			}
			summary(t, map[string]any{"lang": lang, "inputs": run, "mismatches": mism, "flagDisagreements": flagDiff})
		})
	}
}
