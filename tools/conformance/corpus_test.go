package conformance

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	treesitter "github.com/wotjr1649/go-treesitter"
)

// corpusFile finds corpus/<lang>/<name>.<ext>: the extension depends on the language.
func corpusFile(t *testing.T, root, lang, name string) string {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(root, "corpus", lang, name+".*"))
	if len(m) != 1 {
		t.Fatalf("corpus file %s/%s: %d matches", lang, name, len(m))
	}
	return m[0]
}

// TestCorpus: every fresh-corpus input, parsed with the real grammar through the public API, must render to the same
// bytes as the recorded native C dump; Outcome must agree with the dump's root_has_error and HasInvalidUTF8 with
// utf8.Valid. Every 10th input additionally goes through ParseSnapshot.
func TestCorpus(t *testing.T) {
	root := dataRoot(t)
	rows := readCSV(t, filepath.Join(root, "corpus", "manifest.csv"))
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			p := newParser(t, lang)
			var files, nodes, mism, snapChecked, invalid int
			var bytesIn int64
			start := time.Now()
			for _, r := range rows {
				if r[1] != lang {
					continue
				}
				id, name := r[0], filepath.Base(r[0])
				src := readFile(t, corpusFile(t, root, lang, name))
				if sha(src) != r[4] {
					t.Fatalf("%s: input sha256 differs from the manifest (identity drift)", id)
				}
				want := readFile(t, filepath.Join(root, "native", "dumps", lang, name+".dump"))
				files++
				bytesIn += int64(len(src))
				bad := func(format string, a ...any) {
					mism++
					if mism <= 10 {
						t.Errorf("%s: "+format, append([]any{id}, a...)...)
					}
				}
				tree, err := p.Parse(bg, src, nil)
				if err != nil {
					bad("Parse: %v", err)
					continue
				}
				snap, err := tree.Snapshot(bg)
				if err != nil {
					bad("Snapshot: %v", err)
					tree.Close()
					continue
				}
				got := Dump(snap)
				nodes += snap.Len()
				if string(got) != string(want) {
					bad("dump differs from native: %s", firstDiff(got, want))
					preserve("corpus/"+lang+"/"+name+".go.dump", got)
				}
				wantErr := strings.HasPrefix(string(want), "#tsg-dump/1 root_has_error=1")
				if o := tree.Outcome(); (o != treesitter.AcceptedWithErrors && o != treesitter.AcceptedClean) || (o == treesitter.AcceptedWithErrors) != wantErr {
					bad("Outcome %v vs native root_has_error=%v", o, wantErr)
				}
				if tree.HasInvalidUTF8() != !utf8.Valid(src) {
					bad("HasInvalidUTF8 %v, utf8.Valid %v", tree.HasInvalidUTF8(), utf8.Valid(src))
				}
				if tree.HasInvalidUTF8() {
					invalid++
				}
				if tree.InputBytes() != len(src) || snap.Root().EndByte() != uint32(len(src)) {
					bad("root end %d, input %d", snap.Root().EndByte(), len(src))
				}
				tree.Close()
				if files%10 == 1 {
					snapChecked++
					s2, err := p.ParseSnapshot(bg, src)
					if err != nil {
						bad("ParseSnapshot: %v", err)
					} else if g2 := Dump(s2); string(g2) != string(want) {
						bad("ParseSnapshot dump differs: %s", firstDiff(g2, want))
					}
				}
			}
			if files == 0 {
				t.Skip("no corpus rows for " + lang)
			}
			summary(t, map[string]any{"lang": lang, "files": files, "nodes": nodes, "bytes": bytesIn,
				"mismatches": mism, "parseSnapshotChecked": snapChecked, "invalidUTF8Inputs": invalid,
				"seconds": time.Since(start).Seconds()})
		})
	}
}
