package conformance

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	proc "github.com/wotjr1649/go-treesitter/tools/bench/proc"
)

// These edge bytes supplement the upstream corpus, including missing/zero-width nodes at EOF.
func TestSnapshotPointsNative(t *testing.T) {
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			exe, ok := nativeExe(lang)
			if !ok {
				skipConformance(t, "no native executable for "+lang+" (TSG_NATIVE_EXE_DIR)")
			}
			p := newParser(t, lang)
			nodes, missingEOF := 0, 0
			for i, src := range []string{"", "\r\n", "[1,\r\n2]\r\n", "[\"한글😀\",\n\"é\"]\n", "[\"\xff\xc3\",\n1]", "[1", "{\n", "a\rb\n"} {
				path := filepath.Join(t.TempDir(), fmt.Sprintf("edge-%d.in", i))
				if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
					t.Fatal(err)
				}
				want, err := proc.Command(exe, "dump", path).Output()
				if err != nil {
					t.Fatal(err)
				}
				s, err := p.ParseSnapshot(bg, []byte(src))
				if err != nil {
					t.Fatal(err)
				}
				if got := Dump(s); !bytes.Equal(got, want) {
					t.Errorf("edge-%d: %s", i, firstDiff(got, want))
					preserve(fmt.Sprintf("points/%s/edge-%d.native", lang, i), want)
					preserve(fmt.Sprintf("points/%s/edge-%d.go", lang, i), got)
				}
				nodes += s.Len()
				for n := range s.All() {
					if n.IsMissing() && n.StartByte() == uint32(len(src)) && n.EndByte() == uint32(len(src)) {
						missingEOF++
					}
				}
			}
			if lang == "json" && missingEOF == 0 {
				t.Fatal("JSON edge inputs did not exercise missing nodes at EOF")
			}
			summary(t, map[string]any{"lang": lang, "inputs": 8, "nodes": nodes, "missingEOF": missingEOF})
		})
	}
}
