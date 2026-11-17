package conformance

import (
	"bytes"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	proc "github.com/wotjr1649/go-treesitter/tools/bench/proc"
)

// TestFieldLookup compares Node.ChildByFieldName with C's ts_node_child_by_field_name (`<exe> fields`, see
// tools/native-ref/c/native_ref.c) for every node and every field name of the language, over the corpus inputs of
// TestCorpus. The dump comparison of TestCorpus cannot see this: it compares cursor field ids, while C's function walks
// the production's field map (hidden-field inheritance, aliases, ERROR nodes, extras), so the two can disagree.
//
// Every byte difference fails the gate. Field differences are also written to
// $TSG_CONF_OUT/fail/fields/<lang>.tsv (input, node index, field, C result, Go result; "-" = none).
// Needs TSG_NATIVE_EXE_DIR.
// CI sets TSG_REQUIRE_FIELD_LOOKUP=1 so missing prerequisites and zero inputs fail instead of skipping.
func TestFieldLookup(t *testing.T) {
	root := dataRoot(t)
	rows := readCSV(t, filepath.Join(root, "corpus", "manifest.csv"))
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			exe, ok := nativeExe(lang)
			if !ok {
				skipConformance(t, "no native executable for "+lang+" (TSG_NATIVE_EXE_DIR)")
			}
			p := newParser(t, lang)
			var files, inputsDiff, cLines, goLines, onlyC, onlyGo, differ int
			var report []string
			start := time.Now()
			for _, r := range rows {
				if r[1] != lang {
					continue
				}
				id, name := r[0], filepath.Base(r[0])
				path := corpusFile(t, root, lang, name)
				src := readFile(t, path)
				if sha(src) != r[4] {
					t.Fatalf("%s: input sha256 differs from the manifest (identity drift)", id)
				}
				files++
				snap, err := p.ParseSnapshot(bg, src)
				if err != nil {
					t.Fatalf("%s: ParseSnapshot: %v", id, err)
				}
				want, err := proc.Command(exe, "fields", path).Output()
				if err != nil {
					t.Fatalf("%s: native fields: %v", id, err)
				}
				got := Fields(snap)
				wh, _, _ := strings.Cut(string(want), "\n")
				gh, _, _ := strings.Cut(string(got), "\n")
				if !strings.HasPrefix(wh, "#tsg-fields/1 nodes=") || wh != gh {
					t.Fatalf("%s: header native %q, Go %q", id, wh, gh)
				}
				cLines += strings.Count(string(want), "\n") - 1
				goLines += strings.Count(string(got), "\n") - 1
				if bytes.Equal(want, got) {
					continue
				}
				inputsDiff++
				t.Errorf("%s: field lookup bytes differ from C", id)
				preserve("fields/"+lang+"/"+name+".native", want)
				preserve("fields/"+lang+"/"+name+".go", got)
				c, g := fieldMap(want), fieldMap(got)
				var diffs []string
				for k, cv := range c {
					if gv, ok := g[k]; !ok {
						onlyC++
						diffs = append(diffs, id+"\t"+k+"\t"+cv+"\t-")
					} else if gv != cv {
						differ++
						diffs = append(diffs, id+"\t"+k+"\t"+cv+"\t"+gv)
					}
				}
				for k, gv := range g {
					if _, ok := c[k]; !ok {
						onlyGo++
						diffs = append(diffs, id+"\t"+k+"\t-\t"+gv)
					}
				}
				if len(diffs) > 0 {
					sort.Strings(diffs)
					report = append(report, diffs...)
				}
			}
			if files == 0 {
				skipConformance(t, "no corpus rows for "+lang)
			}
			if d := outDir(); d != "" && len(report) > 0 {
				preserve("fields/"+lang+".tsv", []byte("input\tnode\tfield\tC\tGo\n"+strings.Join(report, "\n")+"\n"))
			}
			summary(t, map[string]any{"lang": lang, "files": files, "inputsWithDifference": inputsDiff,
				"nativeResults": cLines, "goResults": goLines, "onlyNative": onlyC, "onlyGo": onlyGo, "differentResult": differ,
				"strict": true, "seconds": time.Since(start).Seconds()})
			if inputsDiff > 0 {
				t.Errorf("%d inputs differ from C ts_node_child_by_field_name (%d only C, %d only Go, %d different result)",
					inputsDiff, onlyC, onlyGo, differ)
			}
		})
	}
}

// fieldMap maps "<node>\t<field>" to the result of the "#tsg-fields/1" lines of a dump.
func fieldMap(b []byte) map[string]string {
	m := map[string]string{}
	lines := strings.Split(string(b), "\n")
	for _, l := range lines[1:] {
		if f := strings.Split(l, "\t"); len(f) == 3 {
			m[f[0]+"\t"+f[1]] = f[2]
		}
	}
	return m
}
