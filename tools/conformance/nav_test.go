package conformance

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	proc "github.com/wotjr1649/go-treesitter/tools/bench/proc"
)

// Column names of the two "#tsg-nav/1" line kinds (tools/native-ref/c/native_ref.c, nav_tree).
var (
	navNodeCols  = []string{"parent", "next", "prev", "nextNamed", "prevNamed"}
	navRangeCols = []string{"span", "spanNamed", "start", "startNamed", "end", "endNamed"}
)

// navDiff lists every differing cell of two "#tsg-nav/1" dumps as "<node>\t<column>\t<C>\t<Go>" (a missing line is
// "<missing>"), and counts the differing cells per column.
func navDiff(want, got []byte, cols map[string]int) []string {
	w, g := strings.Split(string(want), "\n"), strings.Split(string(got), "\n")
	var rows []string
	for i := 1; i < max(len(w), len(g)); i++ { // line 0 is the header, compared by the caller
		var wl, gl []string
		if i < len(w) && w[i] != "" {
			wl = strings.Split(w[i], "\t")
		}
		if i < len(g) && g[i] != "" {
			gl = strings.Split(g[i], "\t")
		}
		if len(wl) == 0 && len(gl) == 0 {
			continue
		}
		if len(wl) == 0 || len(gl) == 0 || len(wl) != len(gl) || wl[0] != gl[0] || wl[1] != gl[1] {
			rows = append(rows, "line "+strconv.Itoa(i)+"\t-\t"+strings.Join(wl, " ")+"\t"+strings.Join(gl, " "))
			cols["structure"]++
			continue
		}
		names := navNodeCols
		if wl[0] == "r" {
			names = navRangeCols
		}
		for c := 2; c < len(wl) && c-2 < len(names); c++ {
			if wl[c] != gl[c] {
				rows = append(rows, wl[1]+"\t"+names[c-2]+"\t"+wl[c]+"\t"+gl[c])
				cols[names[c-2]]++
			}
		}
	}
	return rows
}

// TestNavigation compares Node.Parent/NextSibling/PrevSibling/NextNamedSibling/PrevNamedSibling and
// Snapshot.DescendantForByteRange with C's ts_node_* (`<exe> nav`, tools/native-ref/c/native_ref.c) for every node of the
// corpus inputs of TestCorpus. TestCorpus compares the cursor walk only; these functions are separate C algorithms
// (node.c: sibling search, descendant_for_range) that a runtime move can change without changing the dump.
// Needs TSG_NATIVE_EXE_DIR built from the same runtime as the grammar module (ADR-0020 decision 3).
//
// Every byte difference is a finding. It is written to $TSG_CONF_OUT/fail/nav/<lang>.tsv (input, node, column, C result,
// Go result; "-" = none; at most navRowCap rows per input) and counted per column in the SUMMARY line. By default a
// difference fails the test. With TSG_NAV_REPORT=1 it is logged and kept as data but does not fail: the comparison is
// the same, only the exit status differs, for the first run against C of a function the repository has never compared.
// Missing prerequisites, header or input-identity mismatches and native failures fail in both modes.
func TestNavigation(t *testing.T) {
	const navRowCap = 200
	report := os.Getenv("TSG_NAV_REPORT") == "1"
	root := dataRoot(t)
	rows := readCSV(t, filepath.Join(root, "corpus", "manifest.csv"))
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			exe, ok := nativeExe(lang)
			if !ok {
				skipConformance(t, "no native executable for "+lang+" (TSG_NATIVE_EXE_DIR)")
			}
			p := newParser(t, lang)
			var files, inputsDiff, cells int
			cols := map[string]int{}
			var data []string
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
				want, err := proc.Command(exe, "nav", path).Output()
				if err != nil {
					t.Fatalf("%s: native nav: %v", id, err)
				}
				got := Nav(snap)
				wh, _, _ := strings.Cut(string(want), "\n")
				gh, _, _ := strings.Cut(string(got), "\n")
				if !strings.HasPrefix(wh, "#tsg-nav/1 nodes=") || wh != gh {
					t.Fatalf("%s: header native %q, Go %q", id, wh, gh)
				}
				if bytes.Equal(got, want) {
					continue
				}
				inputsDiff++
				diffs := navDiff(want, got, cols)
				cells += len(diffs)
				if len(diffs) > navRowCap {
					diffs = append(diffs[:navRowCap], "...\t-\t-\t-")
				}
				for _, d := range diffs {
					data = append(data, id+"\t"+d)
				}
				if !report {
					t.Errorf("%s: navigation differs from C: %s", id, firstDiff(got, want))
				}
				if inputsDiff <= 5 { // whole dumps only for the first few; the .tsv has every cell
					preserve("nav/"+lang+"/"+name+".native", want)
					preserve("nav/"+lang+"/"+name+".go", got)
				}
			}
			if files == 0 {
				skipConformance(t, "no corpus rows for "+lang)
			}
			sort.Strings(data)
			preserve("nav/"+lang+".tsv", []byte(strings.Join(data, "\n")+"\n"))
			summary(t, map[string]any{"lang": lang, "inputs": files, "inputsDiffer": inputsDiff, "cellsDiffer": cells, "columns": cols, "reportOnly": report})
			if inputsDiff > 0 {
				msg := strconv.Itoa(inputsDiff) + " of " + strconv.Itoa(files) + " inputs differ from C navigation"
				if report {
					t.Log("NAV-MISMATCH (report only, TSG_NAV_REPORT=1): " + msg)
				} else {
					t.Error(msg)
				}
			}
		})
	}
}

// TestNavDiff pins the cell listing of navDiff (no native executable needed).
func TestNavDiff(t *testing.T) {
	want := []byte("#tsg-nav/1 nodes=2\nn\t0\t-\t-\t-\t-\t-\nr\t0\t0\t0\t0\t0\t0\t0\nn\t1\t0\t-\t-\t-\t-\n")
	got := []byte("#tsg-nav/1 nodes=2\nn\t0\t-\t-\t-\t-\t-\nr\t0\t0\t1\t0\t0\t0\t0\nn\t1\t0\t-\t0\t-\t-\n")
	cols := map[string]int{}
	rows := navDiff(want, got, cols)
	if strings.Join(rows, "|") != "0\tspanNamed\t0\t1|1\tprev\t-\t0" || cols["spanNamed"] != 1 || cols["prev"] != 1 {
		t.Fatalf("rows %q cols %v", rows, cols)
	}
	cols = map[string]int{}
	if rows := navDiff(want, want[:len(want)-8], cols); len(rows) != 1 || cols["structure"] != 1 {
		t.Fatalf("truncated dump: rows %q cols %v", rows, cols)
	}
}

// TestNavigationSelfConsistent needs no native executable: parent and sibling links must be mutual inverses, the
// named variants must skip only unnamed nodes, and a node's own span must resolve to a node that contains it.
func TestNavigationSelfConsistent(t *testing.T) {
	root := dataRoot(t)
	rows := readCSV(t, filepath.Join(root, "corpus", "manifest.csv"))
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			p := newParser(t, lang)
			for _, r := range rows {
				if r[1] != lang {
					continue
				}
				snap, err := p.ParseSnapshot(bg, readFile(t, corpusFile(t, root, lang, filepath.Base(r[0]))))
				if err != nil {
					t.Fatal(err)
				}
				for n := range snap.All() {
					if nx, ok := n.NextSibling(); ok {
						if back, ok := nx.PrevSibling(); !ok || back.Index() != n.Index() {
							t.Fatalf("%s: node %d: next.prev != self", r[0], n.Index())
						}
						pn, _ := n.Parent()
						px, _ := nx.Parent()
						if pn.Index() != px.Index() {
							t.Fatalf("%s: node %d: siblings with different parents", r[0], n.Index())
						}
					}
					if nn, ok := n.NextNamedSibling(); ok && !nn.IsNamed() {
						t.Fatalf("%s: node %d: next named sibling %d is unnamed", r[0], n.Index(), nn.Index())
					}
					d := snap.DescendantForByteRange(n.StartByte(), n.EndByte(), false)
					if d.IsZero() || d.StartByte() > n.StartByte() || d.EndByte() < n.EndByte() {
						t.Fatalf("%s: node %d: span resolves to %s", r[0], n.Index(), strconv.Itoa(d.Index()))
					}
				}
			}
		})
	}
}
