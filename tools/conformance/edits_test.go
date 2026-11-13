package conformance

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	treesitter "github.com/wotjr1649/go-treesitter"
)

// knownIncrementalDiffs are the five tsql cases whose incremental reparse differs from a fresh parse in native C
// tree-sitter v0.27.2 and in the W2 engine alike (edits/REPORT-S4.md section 4). They are a documented ratchet: they
// must keep differing, and no other case may.
var knownIncrementalDiffs = map[string]bool{
	"tsql/E02170": true, "tsql/E02888": true, "tsql/E02901": true, "tsql/E03510": true, "tsql/E03520": true,
}

type nativeResult struct{ editExit, freshExit, editSHA, freshSHA string }

func loadNative(t *testing.T, root string) map[string]nativeResult {
	t.Helper()
	m := map[string]nativeResult{}
	for _, r := range readCSV(t, filepath.Join(root, "edits", "results-native.csv")) {
		m[r[0]] = nativeResult{editExit: r[1], editSHA: r[2], freshExit: r[3], freshSHA: r[4]}
	}
	return m
}

// editSample returns how many cases per language to run: 0 = all (TSG_S15_EDITS_SAMPLE unset or 0), -short = 25.
func editSample() int {
	if v := os.Getenv("TSG_S15_EDITS_SAMPLE"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n >= 0 {
			return n
		}
	}
	if testing.Short() {
		return 25
	}
	return 0
}

// TestEdits: for every edit case, parse old, Tree.Edit(EditFor(..)), reparse new with the edited old tree, and compare
// the incremental result with the recorded native C incremental dump (sha256) and the fresh reparse with the recorded
// native fresh dump. incremental==fresh must agree with the native outcome for every case (the 5 known tsql cases
// differ in both). ChangedRanges must succeed and satisfy structural sanity; when a native reference exe is available
// (TSG_CONF_NATIVECR, see nativeRanges) it must equal the native ts_tree_get_changed_ranges output exactly.
func TestEdits(t *testing.T) {
	root := dataRoot(t)
	native := loadNative(t, root)
	all := readCSV(t, filepath.Join(root, "edits", "manifest.csv"))
	sample := editSample()
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			var rows [][]string
			for _, r := range all {
				if r[1] == lang {
					rows = append(rows, r)
				}
			}
			if len(rows) == 0 {
				t.Skip("no edit cases for " + lang)
			}
			if sample > 0 && sample < len(rows) {
				var pick [][]string
				for i, r := range rows {
					if knownIncrementalDiffs[r[0]] || i*sample/len(rows) != (i+1)*sample/len(rows) {
						pick = append(pick, r)
					}
				}
				rows = pick
			}
			p := newParser(t, lang)
			var cases, nodes, mism, incMatch, freshMatch, incEqFresh, incDiffFresh, treeChanged, rangesEmptyOnReplace, rangesEmptyOnPureDeletion, rangesTotal, knownSeen, crCompared int
			start := time.Now()
			for _, r := range rows {
				id := r[0]
				name := filepath.Base(id)
				oldSrc := readFile(t, filepath.Join(root, "edits", lang, name+".old"))
				newSrc := readFile(t, filepath.Join(root, "edits", lang, name+".new"))
				if sha(oldSrc) != r[6] || sha(newSrc) != r[7] {
					t.Fatalf("%s: input sha256 differs from the manifest (identity drift)", id)
				}
				nr, ok := native[id]
				if !ok || nr.editExit != "0" || nr.freshExit != "0" {
					t.Fatalf("%s: no usable recorded native result (%+v)", id, nr)
				}
				cases++
				bad := func(format string, a ...any) {
					mism++
					if mism <= 10 {
						t.Errorf("%s: "+format, append([]any{id}, a...)...)
					}
				}
				sb, oe, ne := atou(t, r[3]), atou(t, r[4]), atou(t, r[5])

				oldTree, err := p.Parse(bg, oldSrc, nil)
				if err != nil {
					bad("Parse(old): %v", err)
					continue
				}
				oldSnap, err := oldTree.Snapshot(bg) // before Edit: an edited tree cannot be snapshotted
				if err != nil {
					bad("Snapshot(old): %v", err)
					oldTree.Close()
					continue
				}
				oldDump := Dump(oldSnap)
				if err := oldTree.Edit(treesitter.EditFor(oldSrc, newSrc, sb, oe, ne)); err != nil {
					bad("Edit: %v", err)
					oldTree.Close()
					continue
				}
				incTree, err := p.Parse(bg, newSrc, oldTree)
				if err != nil {
					bad("Parse(new, old): %v", err)
					oldTree.Close()
					continue
				}
				incSnap, err := incTree.Snapshot(bg)
				if err != nil {
					bad("Snapshot(inc): %v", err)
					incTree.Close()
					oldTree.Close()
					continue
				}
				incDump := Dump(incSnap)
				nodes += incSnap.Len()
				ranges, err := incTree.ChangedRanges(oldTree)
				if err != nil {
					bad("ChangedRanges: %v", err)
				}
				rangesTotal += len(ranges)
				incTree.Close()
				oldTree.Close()

				freshSnap, err := p.ParseSnapshot(bg, newSrc)
				if err != nil {
					bad("ParseSnapshot(new): %v", err)
					continue
				}
				freshDump := Dump(freshSnap)

				incSHA, freshSHA := sha(incDump), sha(freshDump)
				if incSHA == nr.editSHA {
					incMatch++
				} else {
					bad("incremental dump sha %s != native %s", incSHA[:12], nr.editSHA[:12])
					preserve("edits/"+id+".go-inc.dump", incDump)
					if out, err := nativeEdit(root, lang, name, r); err == nil {
						preserve("edits/"+id+".native-inc.dump", out)
					}
				}
				if freshSHA == nr.freshSHA {
					freshMatch++
				} else {
					bad("fresh dump sha %s != native %s", freshSHA[:12], nr.freshSHA[:12])
					preserve("edits/"+id+".go-fresh.dump", freshDump)
				}
				eq, nativeEq := incSHA == freshSHA, nr.editSHA == nr.freshSHA
				if eq {
					incEqFresh++
				} else {
					incDiffFresh++
				}
				if eq != nativeEq {
					bad("incremental==fresh is %v, native says %v", eq, nativeEq)
				}
				if knownIncrementalDiffs[id] {
					knownSeen++
					if eq {
						bad("known upstream incremental difference no longer differs (ratchet)")
					}
				} else if !eq {
					bad("incremental != fresh and the case is not in the known list")
				}
				if string(oldDump) != string(incDump) {
					treeChanged++
					if len(ranges) == 0 {
						// C reports no changed range for some edits that move nodes without changing the syntax (e.g. a
						// pure deletion, or one more character inside a token): measured, not asserted (see nativeRanges).
						if ne == sb {
							rangesEmptyOnPureDeletion++
						} else {
							rangesEmptyOnReplace++
						}
					}
				}
				// Sanity (always): ranges are ordered, non-overlapping, inside the new input, with start <= end.
				prevEnd := uint32(0)
				for i, rg := range ranges {
					if rg.StartByte > rg.EndByte || rg.EndByte > uint32(len(newSrc)) || (i > 0 && rg.StartByte < prevEnd) {
						bad("ChangedRanges[%d] %+v is not ordered/inside the %d-byte input", i, rg, len(newSrc))
					}
					prevEnd = rg.EndByte
				}
				// Ground truth when a native reference exe exists: ranges must equal ts_tree_get_changed_ranges exactly.
				if want, ok := nativeRanges(lang, root, name, r); ok {
					crCompared++
					got := ""
					for _, rg := range ranges {
						got += fmt.Sprintf("%d %d %d %d %d %d\n", rg.StartByte, rg.EndByte, rg.StartPoint.Row, rg.StartPoint.Column, rg.EndPoint.Row, rg.EndPoint.Column)
					}
					if got != want {
						bad("ChangedRanges differ from native C: got %q want %q", got, want)
					}
				}
			}
			summary(t, map[string]any{"lang": lang, "cases": cases, "nodes": nodes, "mismatches": mism,
				"incrementalMatchesNative": incMatch, "freshMatchesNative": freshMatch,
				"incrementalEqualsFresh": incEqFresh, "incrementalDiffersFromFresh": incDiffFresh,
				"knownRatchetCasesRun": knownSeen, "treeChanged": treeChanged,
				"changedRangesEmptyOnReplaceOrInsert": rangesEmptyOnReplace, "changedRangesEmptyOnPureDeletion": rangesEmptyOnPureDeletion, "changedRangesComparedToNative": crCompared, "changedRangesTotal": rangesTotal,
				"sample": sample, "seconds": time.Since(start).Seconds()})
		})
	}
}

// nativeEdit runs the native reference exe for one case (preserving a failing case's native dump).
func nativeEdit(root, lang, name string, r []string) ([]byte, error) {
	exe := filepath.Join(root, "native", "exe2", lang+".exe")
	d := filepath.Join(root, "edits", lang)
	return exec.Command(exe, "edit", filepath.Join(d, name+".old"), filepath.Join(d, name+".new"), r[3], r[4], r[5]).Output()
}

// nativeRanges runs the native ChangedRanges reference (cr.exe built from .scratch/s15/p2-api/conf/nativecr/cr.c) for one
// case; ok is false when TSG_CONF_NATIVECR is unset or has no exe for the language.
func nativeRanges(lang, root, name string, r []string) (string, bool) {
	dir := os.Getenv("TSG_CONF_NATIVECR")
	if dir == "" {
		return "", false
	}
	exe := filepath.Join(dir, lang+".exe")
	if _, err := os.Stat(exe); err != nil {
		return "", false
	}
	d := filepath.Join(root, "edits", lang)
	out, err := exec.Command(exe, filepath.Join(d, name+".old"), filepath.Join(d, name+".new"), r[3], r[4], r[5]).Output()
	if err != nil {
		return "", false
	}
	return strings.ReplaceAll(string(out), "\r\n", "\n"), true // the exe prints in Windows text mode
}
