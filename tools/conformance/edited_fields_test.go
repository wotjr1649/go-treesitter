package conformance

import (
	"bytes"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	treesitter "github.com/wotjr1649/go-treesitter"
	proc "github.com/wotjr1649/go-treesitter/tools/bench/proc"
)

// TestFieldLookupEdits compares all seeded edits with C's incremental tree, including
// knownIncrementalDiffs: a fresh parse is never the reference for this test.
func TestFieldLookupEdits(t *testing.T) {
	root := dataRoot(t)
	rows := readCSV(t, filepath.Join(root, "edits", "manifest.csv"))
	native := map[string][]string{}
	for _, r := range readCSV(t, filepath.Join(root, "edits", "results-native.csv")) {
		native[r[0]] = r
	}
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			p := newParser(t, lang)
			var cases, results, firstChecks, knownSeen int
			start := time.Now()
			for _, r := range rows {
				if r[1] != lang {
					continue
				}
				id, name := r[0], filepath.Base(r[0])
				d := filepath.Join(root, "edits", lang)
				oldPath, newPath := filepath.Join(d, name+".old"), filepath.Join(d, name+".new")
				oldSrc, newSrc := readFile(t, oldPath), readFile(t, newPath)
				if sha(oldSrc) != r[6] || sha(newSrc) != r[7] {
					t.Fatalf("%s: input sha256 differs from the manifest (identity drift)", id)
				}
				var want []byte
				nr := native[id]
				if len(nr) >= 8 {
					if nr[6] != "0" {
						t.Fatalf("%s: no usable recorded native edit-fields result (exit %s)", id, nr[6])
					}
					want = readFile(t, filepath.Join(d, name+".fields"))
					if sha(want) != nr[7] {
						t.Fatalf("%s: native edit-fields sha256 differs from the manifest (identity drift)", id)
					}
				} else {
					exe, ok := nativeExe(lang)
					if !ok {
						skipConformance(t, "no native edit-fields result or executable for "+lang)
					}
					var err error
					want, err = proc.Command(exe, "edit-fields", oldPath, newPath, r[3], r[4], r[5]).Output()
					if err != nil {
						t.Fatalf("%s: native edit-fields: %v", id, err)
					}
				}
				oldTree, err := p.Parse(bg, oldSrc, nil)
				if err != nil {
					t.Fatalf("%s: Parse(old): %v", id, err)
				}
				err = oldTree.Edit(treesitter.EditFor(oldSrc, newSrc, atou(t, r[3]), atou(t, r[4]), atou(t, r[5])))
				if err != nil {
					oldTree.Close()
					t.Fatalf("%s: Edit: %v", id, err)
				}
				incTree, err := p.Parse(bg, newSrc, oldTree)
				oldTree.Close()
				if err != nil {
					t.Fatalf("%s: Parse(new, old): %v", id, err)
				}
				snap, err := incTree.Snapshot(bg)
				incTree.Close()
				if err != nil {
					t.Fatalf("%s: Snapshot(inc): %v", id, err)
				}
				got := Fields(snap)
				if !bytes.HasPrefix(want, []byte("#tsg-fields/1 nodes=")) || !bytes.Equal(want, got) {
					preserve("edited-fields/"+id+".native", want)
					preserve("edited-fields/"+id+".go", got)
					c, g := fieldMap(want), fieldMap(got)
					var diff []string
					for k, cv := range c {
						if gv, ok := g[k]; !ok {
							diff = append(diff, id+"\t"+k+"\t"+cv+"\t-")
						} else if gv != cv {
							diff = append(diff, id+"\t"+k+"\t"+cv+"\t"+gv)
						}
					}
					for k, gv := range g {
						if _, ok := c[k]; !ok {
							diff = append(diff, id+"\t"+k+"\t-\t"+gv)
						}
					}
					sort.Strings(diff)
					preserve("edited-fields/"+lang+".tsv", []byte("input\tnode\tfield\tC\tGo\n"+strings.Join(diff, "\n")+"\n"))
					t.Fatalf("%s: incremental field lookup differs from C: %s", id, firstDiff(got, want))
				}
				for n := range snap.All() {
					for f := 1; f < snap.Language().FieldCount(); f++ {
						field := snap.Language().FieldName(treesitter.FieldID(f))
						child, ok := n.ChildByFieldName(field)
						var first treesitter.Node
						var found bool
						for c := range n.ChildrenByFieldName(field) {
							first, found = c, true
							break
						}
						if ok != found || ok && child.Index() != first.Index() {
							preserve("edited-fields/"+id+".native", want)
							preserve("edited-fields/"+id+".go", got)
							t.Fatalf("%s: node %d field %s: ChildByFieldName=(%d,%v), ChildrenByFieldName first=(%d,%v)",
								id, n.Index(), field, child.Index(), ok, first.Index(), found)
						}
						firstChecks++
					}
				}
				cases++
				results += bytes.Count(got, []byte("\n")) - 1
				if knownIncrementalDiffs[id] {
					knownSeen++
				}
			}
			if cases == 0 {
				skipConformance(t, "no edit rows for "+lang)
			}
			summary(t, map[string]any{"lang": lang, "cases": cases, "nativeResults": results, "goResults": results,
				"inputsWithDifference": 0, "childrenFirstChecks": firstChecks, "knownRatchetCasesRun": knownSeen,
				"strict": true, "seconds": time.Since(start).Seconds()})
		})
		if t.Failed() {
			t.FailNow()
		}
	}
}
