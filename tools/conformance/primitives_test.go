package conformance

import (
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/experimental/match"
)

// Real-grammar checks of the v0.0.1 primitives (api-v2 section 7) on the fresh corpus, through the public API only.

// canon is the full canonical serialization of a subtree, built exactly from what Hash/ShapeHash claim to cover:
// symbol, field, named/extra/missing/error flags, leaf text and children in order (extras skipped when shape).
func canon(n treesitter.Node, shape bool, dst []byte) []byte {
	var f uint64
	for i, b := range []bool{n.IsNamed(), n.IsExtra() && !shape, n.IsMissing(), n.IsError()} {
		if b {
			f |= 1 << i
		}
	}
	dst = append(dst, '(')
	dst = strconv.AppendUint(dst, uint64(n.Symbol()), 10)
	dst = append(dst, '@')
	dst = strconv.AppendUint(dst, uint64(n.FieldID()), 10)
	dst = append(dst, ':')
	dst = strconv.AppendUint(dst, f, 10)
	if n.ChildCount() == 0 {
		t := n.Text()
		dst = append(dst, ' ')
		dst = strconv.AppendInt(dst, int64(len(t)), 10)
		dst = append(dst, ':')
		dst = append(dst, t...)
	} else {
		for c := range n.Children() {
			if shape && c.IsExtra() {
				continue
			}
			dst = canon(c, shape, dst)
		}
	}
	return append(dst, ')')
}

func naiveCols(src string, off int) (line int, byteCol, u16, runes uint32) {
	last := 0
	for i := 0; i < off; i++ {
		if src[i] == '\n' {
			line++
			last = i + 1
		}
	}
	byteCol = uint32(off - last)
	for t := src[last:off]; len(t) > 0; {
		r, w := utf8.DecodeRuneInString(t)
		t = t[w:]
		runes++
		u16++
		if r >= 0x10000 {
			u16++
		}
	}
	return
}

// TestPrimitivesOnCorpus: LineIndex, Merkle hashes, atoms and Index on every corpus input of every linked language.
func TestPrimitivesOnCorpus(t *testing.T) {
	root := dataRoot(t)
	rows := readCSV(t, filepath.Join(root, "corpus", "manifest.csv"))
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			p := newParser(t, lang)
			byHash, byCanon := [2]map[uint64]string{{}, {}}, [2]map[string]uint64{{}, {}}
			var files, nodes, posChecked, groupsMulti, atomNodes, idxQueries int
			var nonASCIIFiles int
			rng := rand.New(rand.NewPCG(42, 43))
			start := time.Now()
			multi := [2]map[uint64]int{{}, {}}
			for _, r := range rows {
				if r[1] != lang {
					continue
				}
				name := filepath.Base(r[0])
				src := readFile(t, corpusFile(t, root, lang, name))
				snap, err := p.ParseSnapshot(bg, src)
				if err != nil {
					t.Fatalf("%s: %v", r[0], err)
				}
				files++
				nodes += snap.Len()
				text := snap.Source()
				if !utf8.ValidString(text) || strings.ContainsFunc(text, func(r rune) bool { return r >= 0x80 }) {
					nonASCIIFiles++
				}

				// LineIndex: every node start/end equals the snapshot's row and byte column; UTF-16/rune columns equal a
				// naive scan at random offsets.
				lx := snap.Lines()
				for n := range snap.All() {
					a, b := lx.Pos(n.StartByte()), lx.Pos(n.EndByte())
					if uint32(a.Line) != n.StartPoint().Row || a.ByteCol != n.StartPoint().Column ||
						uint32(b.Line) != n.EndPoint().Row || b.ByteCol != n.EndPoint().Column {
						t.Fatalf("%s node %d: LineIndex %+v %+v vs snapshot %v %v", r[0], n.Index(), a, b, n.StartPoint(), n.EndPoint())
					}
					posChecked += 2
				}
				for k := 0; k < 100; k++ {
					off := rng.IntN(len(text) + 1)
					l, bc, u16, rc := naiveCols(text, off)
					if got := lx.Pos(uint32(off)); got.Line != l || got.ByteCol != bc || got.UTF16Col != u16 || got.RuneCol != rc {
						t.Fatalf("%s offset %d: LineIndex %+v, naive %d/%d/%d/%d", r[0], off, got, l, bc, u16, rc)
					}
					posChecked++
				}

				// Merkle: equal hash <=> equal full serialization, across all files of the language.
				for n := range snap.All() {
					for v, h := range [2]uint64{n.Hash(), n.ShapeHash()} {
						c := string(canon(n, v == 1, nil))
						if prev, ok := byHash[v][h]; ok && prev != c {
							t.Fatalf("%s: %s equal at hash %x but serializations differ:\n%.200s\n%.200s", r[0], [2]string{"Hash", "ShapeHash"}[v], h, prev, c)
						}
						if prev, ok := byCanon[v][c]; ok && prev != h {
							t.Fatalf("%s: equal serialization, different %s %x vs %x: %.200s", r[0], [2]string{"Hash", "ShapeHash"}[v], prev, h, c)
						}
						if multi[v][h]++; multi[v][h] == 2 {
							groupsMulti++
						}
						byHash[v][h], byCanon[v][c] = c, h
					}
				}

				// Atoms: AtomText round-trips; equal atom <=> equal text within the snapshot.
				txt, at := map[string]treesitter.Atom{}, map[treesitter.Atom]string{}
				for n := range snap.All() {
					a := n.Atom()
					if a == 0 {
						continue
					}
					atomNodes++
					if snap.AtomText(a) != n.Text() || !n.IsNamed() || n.ChildCount() != 0 || n.Text() == "" {
						t.Fatalf("%s node %d: bad atom %d", r[0], n.Index(), a)
					}
					if pa, ok := txt[n.Text()]; ok && pa != a {
						t.Fatalf("%s: equal text %q has atoms %d and %d", r[0], n.Text(), pa, a)
					}
					if pt, ok := at[a]; ok && pt != n.Text() {
						t.Fatalf("%s: atom %d has texts %q and %q", r[0], a, pt, n.Text())
					}
					txt[n.Text()], at[a] = a, n.Text()
				}
				if snap.NumAtoms() != len(at) {
					t.Fatalf("%s: NumAtoms %d, distinct %d", r[0], snap.NumAtoms(), len(at))
				}

				// Index: OfSymbol/OfType equal a scan; NodeAt equals a brute-force deepest-containing search.
				x := snap.Index()
				seen := map[treesitter.Symbol]bool{}
				for n := range snap.All() {
					if seen[n.Symbol()] {
						continue
					}
					seen[n.Symbol()] = true
					var want, got, gotKind, wantKind []int
					for m := range snap.All() {
						if m.Symbol() == n.Symbol() {
							want = append(want, m.Index())
						}
						if m.Kind() == n.Kind() {
							wantKind = append(wantKind, m.Index())
						}
					}
					for m := range x.OfSymbol(n.Symbol()) {
						got = append(got, m.Index())
					}
					for m := range x.OfType(n.Kind()) {
						gotKind = append(gotKind, m.Index())
					}
					if !slices.Equal(want, got) || !slices.Equal(wantKind, gotKind) {
						t.Fatalf("%s: OfSymbol/OfType(%s) differs from a scan", r[0], n.Kind())
					}
					idxQueries += 2
					if snap.Len() > 3000 { // the scan above is quadratic in distinct symbols x nodes; bound it
						break
					}
				}
				if snap.Len() <= 3000 {
					depth := make([]int, snap.Len())
					for n := range snap.All() {
						if q, ok := n.Parent(); ok {
							depth[n.Index()] = depth[q.Index()] + 1
						}
					}
					for k := 0; k < 60; k++ {
						off := uint32(rng.IntN(len(text) + 1))
						want, wd := -1, -1
						for n := range snap.All() {
							if n.StartByte() <= off && off < n.EndByte() && depth[n.Index()] > wd {
								want, wd = n.Index(), depth[n.Index()]
							}
						}
						got, ok := x.NodeAt(off)
						if (want >= 0) != ok || (ok && got.Index() != want) {
							t.Fatalf("%s: NodeAt(%d) = %d,%v want %d", r[0], off, got.Index(), ok, want)
						}
						idxQueries++
					}
				}
			}
			if files == 0 {
				t.Skip("no corpus rows for " + lang)
			}
			summary(t, map[string]any{"lang": lang, "files": files, "nodes": nodes, "positionsChecked": posChecked,
				"distinctSubtrees": len(byHash[0]), "distinctShapeSubtrees": len(byHash[1]), "equalHashGroupsWithManyMembers": groupsMulti,
				"atomNodes": atomNodes, "indexQueries": idxQueries, "nonASCIIFiles": nonASCIIFiles, "seconds": time.Since(start).Seconds()})
		})
	}
}

// TestParserPoolRealEngine: Put frees live C trees in the real engine and resets the C parser; the reused parser then
// parses another input to its own native dump; concurrent borrowers (bound 2) all get native-identical trees.
func TestParserPoolRealEngine(t *testing.T) {
	root := dataRoot(t)
	rows := readCSV(t, filepath.Join(root, "corpus", "manifest.csv"))
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			var names []string
			for _, r := range rows {
				if r[1] == lang && len(names) < 24 {
					names = append(names, filepath.Base(r[0]))
				}
			}
			if len(names) < 3 {
				t.Skip("too few corpus rows")
			}
			load := func(name string) (src, want []byte) {
				return readFile(t, corpusFile(t, root, lang, name)), readFile(t, filepath.Join(root, "native", "dumps", lang, name+".dump"))
			}
			pl, err := treesitter.NewParserPool(languages[lang](), 2)
			if err != nil {
				t.Fatal(err)
			}
			defer pl.Close()
			p, err := pl.Get(bg)
			if err != nil {
				t.Fatal(err)
			}
			srcA, wantA := load(names[0])
			trA, err := p.Parse(bg, srcA, nil)
			if err != nil {
				t.Fatal(err)
			}
			snapA, _ := trA.Snapshot(bg)
			trB, _ := p.Parse(bg, srcA, nil) // two live trees, never closed
			_ = trB
			pl.Put(p)
			if _, err := trA.Snapshot(bg); err == nil {
				t.Fatal("a tree must be invalid after Put")
			}
			if string(Dump(snapA)) != string(wantA) {
				t.Fatal("a snapshot must stay valid after Put")
			}
			p2, _ := pl.Get(bg)
			if p2 != p {
				t.Fatal("the idle parser must be reused")
			}
			srcB, wantB := load(names[1])
			tr, err := p2.Parse(bg, srcB, nil)
			if err != nil {
				t.Fatal(err)
			}
			snapB, _ := tr.Snapshot(bg)
			if string(Dump(snapB)) != string(wantB) {
				t.Fatalf("reused parser: dump differs from native: %s", firstDiff(Dump(snapB), wantB))
			}
			pl.Put(p2)

			var wg sync.WaitGroup
			errs := make(chan error, 8)
			checked := 0
			var mu sync.Mutex
			for g := 0; g < 4; g++ {
				wg.Add(1)
				go func(g int) {
					defer wg.Done()
					for i := g; i < len(names); i += 4 {
						src, want := load(names[i])
						q, err := pl.Get(bg)
						if err != nil {
							errs <- err
							return
						}
						s, err := q.ParseSnapshot(bg, src)
						if err == nil && string(Dump(s)) != string(want) {
							err = fmt.Errorf("%s: dump differs from native", names[i])
						}
						pl.Put(q)
						if err != nil {
							errs <- err
							return
						}
						mu.Lock()
						checked++
						mu.Unlock()
					}
				}(g)
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				t.Error(err)
			}
			summary(t, map[string]any{"lang": lang, "concurrentParsesChecked": checked})
		})
	}
}

// TestSourceUTF16RealEngine: a corpus input converted to UTF-16 (LE and BE, with BOM) goes through NewSource and parses
// to the same native dump as the UTF-8 original; OriginalOffset maps node starts back to the UTF-16 byte offsets.
func TestSourceUTF16RealEngine(t *testing.T) {
	root := dataRoot(t)
	rows := readCSV(t, filepath.Join(root, "corpus", "manifest.csv"))
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			p := newParser(t, lang)
			n, nodesChecked := 0, 0
			for _, r := range rows {
				if r[1] != lang || n >= 12 {
					continue
				}
				name := filepath.Base(r[0])
				src := readFile(t, corpusFile(t, root, lang, name))
				if !utf8.Valid(src) || (len(src) >= 3 && src[0] == 0xEF && src[1] == 0xBB) {
					continue
				}
				want := readFile(t, filepath.Join(root, "native", "dumps", lang, name+".dump"))
				n++
				for _, big := range []bool{false, true} {
					units := append([]uint16{0xFEFF}, utf16.Encode([]rune(string(src)))...)
					raw := make([]byte, 2*len(units))
					for i, u := range units {
						if big {
							binary.BigEndian.PutUint16(raw[2*i:], u)
						} else {
							binary.LittleEndian.PutUint16(raw[2*i:], u)
						}
					}
					s, err := treesitter.NewSource(raw)
					if err != nil || string(s.Bytes()) != string(src) {
						t.Fatalf("%s: NewSource: %v (bytes equal: %v)", name, err, err == nil && string(s.Bytes()) == string(src))
					}
					snap, err := p.ParseSnapshot(bg, s.Bytes())
					if err != nil {
						t.Fatal(err)
					}
					if string(Dump(snap)) != string(want) {
						t.Fatalf("%s: dump of the converted source differs from native", name)
					}
					// OriginalOffset of node starts == 2 (BOM) + 2 * UTF-16 units before the node
					for node := range snap.All() {
						if node.Index()%7 != 0 {
							continue
						}
						pre := utf16.Encode([]rune(string(src[:node.StartByte()])))
						if got := s.OriginalOffset(node.StartByte()); got != 2+2*len(pre) {
							t.Fatalf("%s node %d: OriginalOffset %d want %d", name, node.Index(), got, 2+2*len(pre))
						}
						nodesChecked++
					}
				}
			}
			summary(t, map[string]any{"lang": lang, "filesConverted": n, "nodeOffsetsChecked": nodesChecked})
		})
	}
}

// TestMatchOnEdits: Match is EXPERIMENTAL; this records its accuracy on real edit cases (ground truth: nodes entirely
// before the edit keep their range, nodes entirely after it shift by the length delta) and asserts only the
// invariants (injective, symbol-preserving, NewOf/OldOf inverse, identical snapshots match completely).
// TSG_S15_MATCH_SAMPLE=N edits per language (default 40; 0 = all).
func TestMatchOnEdits(t *testing.T) {
	root := dataRoot(t)
	rows := readCSV(t, filepath.Join(root, "edits", "manifest.csv"))
	limit := 40
	if v := os.Getenv("TSG_S15_MATCH_SAMPLE"); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			p := newParser(t, lang)
			var cases, gtNodes, right, wrong, unmatched int
			for _, r := range rows {
				if r[1] != lang {
					continue
				}
				if limit > 0 && cases >= limit {
					break
				}
				id := filepath.Base(r[0])
				dir := filepath.Join(root, "edits", lang)
				oldSrc, newSrc := readFile(t, filepath.Join(dir, id+".old")), readFile(t, filepath.Join(dir, id+".new"))
				start, oldEnd, newEnd := atou(t, r[3]), atou(t, r[4]), atou(t, r[5])
				so, err1 := p.ParseSnapshot(bg, oldSrc)
				sn, err2 := p.ParseSnapshot(bg, newSrc)
				if err1 != nil || err2 != nil {
					t.Fatalf("%s: %v %v", id, err1, err2)
				}
				cases++
				m := match.Match(so, sn)
				if same := match.Match(so, so); same.Len() != so.Len() {
					t.Fatalf("%s: identical snapshots matched %d of %d", id, same.Len(), so.Len())
				}
				used := map[int]bool{}
				for n := range so.All() {
					j, ok := m.NewOf(n)
					if ok {
						if used[j.Index()] || j.Symbol() != n.Symbol() {
							t.Fatalf("%s: node %d matched twice or across symbols", id, n.Index())
						}
						used[j.Index()] = true
						if back, ok := m.OldOf(j); !ok || back.Index() != n.Index() {
							t.Fatalf("%s: NewOf/OldOf are not inverse", id)
						}
					}
					var wantStart, wantEnd int64
					switch {
					case n.EndByte() <= start:
						wantStart, wantEnd = int64(n.StartByte()), int64(n.EndByte())
					case n.StartByte() >= oldEnd:
						d := int64(newEnd) - int64(oldEnd)
						wantStart, wantEnd = int64(n.StartByte())+d, int64(n.EndByte())+d
					default:
						continue // overlaps the edit: no ground truth
					}
					gtNodes++
					switch {
					case !ok:
						unmatched++
					case int64(j.StartByte()) == wantStart && int64(j.EndByte()) == wantEnd:
						right++
					default:
						wrong++
					}
				}
			}
			if cases == 0 {
				t.Skip("no edit rows for " + lang)
			}
			pct := func(a int) float64 {
				if gtNodes == 0 {
					return 0
				}
				return 100 * float64(a) / float64(gtNodes)
			}
			summary(t, map[string]any{"lang": lang, "cases": cases, "groundTruthNodes": gtNodes, "rightPct": pct(right),
				"wrongPct": pct(wrong), "unmatchedPct": pct(unmatched)})
		})
	}
}
