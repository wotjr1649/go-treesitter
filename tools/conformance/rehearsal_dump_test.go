package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/tools/conformance/rehearsal"
)

// TestRuntimeRehearsalDump is the measuring half of the runtime-only rehearsal (tools/conformance/runtime-rehearsal.sh;
// TEST ONLY, never a gate). With TSG_REHEARSAL_OUT set it parses every input of the ciconf data (TSG_DATA_DIR) with the
// linked grammar modules and writes one table row per input (package rehearsal): dump and Fields() hashes of the fresh
// parse and, for edit inputs, the incremental reparse, ChangedRanges and whether incremental equals fresh. It compares
// nothing and needs no native executable; the script runs it once against the committed modules and once against modules
// generated for another runtime, and cmd/rehearsal-compare classifies the two tables.
// TSG_REHEARSAL_LARGE optionally names a directory with large-<pins key>/<pins key>/* inputs (the tools/bench layout).
func TestRuntimeRehearsalDump(t *testing.T) {
	out := os.Getenv("TSG_REHEARSAL_OUT")
	if out == "" {
		t.Skip("TSG_REHEARSAL_OUT unset")
	}
	root := dataRoot(t)
	var rows []rehearsal.Row
	row := func(lang, id, kind string, src []byte, p *treesitter.Parser) rehearsal.Row {
		r := rehearsal.Row{Lang: lang, ID: id, Kind: kind, Src: sha(src)}
		r.Inc, r.IncEq, r.Ranges, r.RangesN = rehearsal.NoEdit()
		snap, err := p.ParseSnapshot(bg, src)
		if err != nil {
			r.Tree, r.Nodes, r.Err, r.Fields = "ERR", "0", "ERR", rehearsal.Clean(err.Error())
			return r
		}
		r.Tree, r.Nodes, r.Err, r.Fields = sha(Dump(snap)), strconv.Itoa(snap.Len()), strconv.FormatBool(snap.Root().HasError()), sha(Fields(snap))
		return r
	}
	for _, lang := range langs(t) {
		p := newParser(t, lang)
		for _, r := range readCSV(t, filepath.Join(root, "corpus", "manifest.csv")) {
			if r[1] == lang {
				rows = append(rows, row(lang, r[0], "corpus", readFile(t, corpusFile(t, root, lang, filepath.Base(r[0]))), p))
			}
		}
		for _, r := range readCSV(t, filepath.Join(root, "edits", "manifest.csv")) {
			if r[1] != lang {
				continue
			}
			name := filepath.Base(r[0])
			oldSrc := readFile(t, filepath.Join(root, "edits", lang, name+".old"))
			newSrc := readFile(t, filepath.Join(root, "edits", lang, name+".new"))
			if sha(oldSrc) != r[6] || sha(newSrc) != r[7] {
				t.Fatalf("%s: input sha256 differs from the manifest (identity drift)", r[0])
			}
			x := row(lang, "edit/"+name+".new", "edit", newSrc, p)
			x.Inc, x.IncEq, x.Ranges, x.RangesN = rehearsalEdit(p, oldSrc, newSrc, r, x.Tree)
			rows = append(rows, x)
		}
		key := lang
		if lang == "go" {
			key = "golang" // pins.json key
		}
		if dir := os.Getenv("TSG_REHEARSAL_LARGE"); dir != "" {
			large, _ := filepath.Glob(filepath.Join(dir, "large-"+key, key, "*"))
			sort.Strings(large)
			for _, f := range large {
				if !strings.HasSuffix(f, ".LICENSE") {
					rows = append(rows, row(lang, "large/"+filepath.Base(f), "large", readFile(t, f), p))
				}
			}
		}
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := rehearsal.Write(f, rows); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d rows -> %s", len(rows), out)
}

// rehearsalEdit parses old, applies the edit, reparses new incrementally and returns the incremental dump hash, whether it
// equals the fresh parse (freshTree is the fresh dump hash; "ERR" when that failed), and the ChangedRanges hash and count.
// A failing step yields "ERR" cells (with the reason in the hash cell) rather than failing the run.
func rehearsalEdit(p *treesitter.Parser, oldSrc, newSrc []byte, r []string, freshTree string) (inc, incEq, ranges, rangesN string) {
	fail := func(step string, err error) (string, string, string, string) {
		return "ERR:" + step + ":" + rehearsal.Clean(err.Error()), "ERR", "ERR", "0"
	}
	oldTree, err := p.Parse(bg, oldSrc, nil)
	if err != nil {
		return fail("parse-old", err)
	}
	defer oldTree.Close()
	sb, oe, ne := atoiMust(r[3]), atoiMust(r[4]), atoiMust(r[5])
	if err := oldTree.Edit(treesitter.EditFor(oldSrc, newSrc, sb, oe, ne)); err != nil {
		return fail("edit", err)
	}
	incTree, err := p.Parse(bg, newSrc, oldTree)
	if err != nil {
		return fail("parse-inc", err)
	}
	defer incTree.Close()
	snap, err := incTree.Snapshot(bg)
	if err != nil {
		return fail("snapshot-inc", err)
	}
	inc = sha(Dump(snap))
	rg, err := incTree.ChangedRanges(oldTree)
	if err != nil {
		return fail("changed-ranges", err)
	}
	var sb2 strings.Builder
	for _, x := range rg {
		fmt.Fprintf(&sb2, "%d %d %d %d %d %d\n", x.StartByte, x.EndByte, x.StartPoint.Row, x.StartPoint.Column, x.EndPoint.Row, x.EndPoint.Column)
	}
	return inc, strconv.FormatBool(inc == freshTree), sha([]byte(sb2.String())), strconv.Itoa(len(rg))
}

func atoiMust(s string) uint32 {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		panic(err)
	}
	return uint32(n)
}
