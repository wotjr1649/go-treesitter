package treesitter

import (
	"context"
	"errors"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

// randSource builds a source in the fake language from a seeded generator.
func randSource(r *rand.Rand, tokens int) string {
	parts := []string{"a", "bb", "ccc", "dd", "+", "(", ")", "(", " ", " ", "\n", "!", "é", "€"}
	var b strings.Builder
	for i := 0; i < tokens; i++ {
		b.WriteString(parts[r.IntN(len(parts))])
	}
	return b.String()
}

func snapOf(t *testing.T, p *Parser, src string) *Snapshot {
	t.Helper()
	s, err := p.ParseSnapshot(bg, []byte(src))
	if err != nil {
		t.Fatalf("ParseSnapshot(%q): %v", src, err)
	}
	return s
}

// ---- Source -----------------------------------------------------------------------------------------------------

func utf16Bytes(s string, bigEndian, bom bool) []byte {
	units := utf16.Encode([]rune(s))
	if bom {
		units = append([]uint16{0xFEFF}, units...)
	}
	out := make([]byte, 0, 2*len(units))
	for _, u := range units {
		if bigEndian {
			out = append(out, byte(u>>8), byte(u))
		} else {
			out = append(out, byte(u), byte(u>>8))
		}
	}
	return out
}

func TestSourceEncodings(t *testing.T) {
	const text = "aé€\U0001F600\nb"
	le, err := NewSource(utf16Bytes(text, false, true))
	if err != nil || le.Encoding() != UTF16LE || string(le.Bytes()) != text {
		t.Fatalf("LE BOM: %v %v %q", err, le.Encoding(), le.Bytes())
	}
	be, err := NewSource(utf16Bytes(text, true, true))
	if err != nil || be.Encoding() != UTF16BE || string(be.Bytes()) != text {
		t.Fatalf("BE BOM: %v %v %q", err, be.Encoding(), be.Bytes())
	}
	nobom, err := NewSourceUTF16(utf16Bytes(text, true, false), true)
	if err != nil || string(nobom.Bytes()) != text {
		t.Fatalf("BE without BOM: %v", err)
	}
	plain := []byte("\xef\xbb\xbfabc")
	u8, _ := NewSource(plain)
	if u8.Encoding() != UTF8 || &u8.Bytes()[0] != &plain[0] || u8.OriginalOffset(3) != 3 || u8.OriginalOffset(99) != len(plain) {
		t.Fatal("UTF-8 (with BOM) must be used unchanged")
	}
	if _, err := NewSourceUTF16([]byte{1, 2, 3}, false); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("odd length: %v", err)
	}
	lone, _ := NewSourceUTF16([]byte{0x61, 0, 0x00, 0xD8, 0x62, 0}, false) // a, lone high surrogate, b
	if string(lone.Bytes()) != "a�b" || lone.OriginalOffset(4) != 4 {
		t.Fatalf("lone surrogate: %q %d", lone.Bytes(), lone.OriginalOffset(4))
	}
	empty, _ := NewSource(nil)
	if len(empty.Bytes()) != 0 || empty.OriginalOffset(5) != 0 {
		t.Fatal("empty")
	}
	if be0, _ := NewSource([]byte{0xFE, 0xFF}); len(be0.Bytes()) != 0 || be0.OriginalOffset(0) != 2 {
		t.Fatal("BOM only")
	}
}

func TestSourceOriginalOffset(t *testing.T) { // crosses many 1 KiB checkpoints, every rune boundary
	r := rand.New(rand.NewPCG(1, 2))
	var b strings.Builder
	pool := []rune{'a', 'z', ' ', '\n', 'é', '€', '\U0001F600', '\U00010348'}
	for i := 0; i < 6000; i++ {
		b.WriteRune(pool[r.IntN(len(pool))])
	}
	text := b.String()
	for _, big := range []bool{false, true} {
		src, err := NewSourceUTF16(utf16Bytes(text, big, true), big)
		if err != nil || string(src.Bytes()) != text {
			t.Fatal("conversion", err)
		}
		units := 0
		for off, ru := range text {
			want := 2 + 2*units // BOM + units before the rune
			if got := src.OriginalOffset(uint32(off)); got != want {
				t.Fatalf("big=%v off %d: got %d want %d", big, off, got, want)
			}
			if w := utf8.RuneLen(ru); w > 1 { // inside the rune: its start
				if got := src.OriginalOffset(uint32(off + 1)); got != want {
					t.Fatalf("mid-rune off %d: got %d want %d", off+1, got, want)
				}
			}
			units += utf16.RuneLen(ru)
		}
		if got := src.OriginalOffset(uint32(len(text))); got != len(src.Original()) {
			t.Fatalf("end: %d", got)
		}
	}
}

func TestSourceParses(t *testing.T) {
	p := newP(t, newFakeLang(nil))
	src, _ := NewSource(utf16Bytes("(a b)", false, true))
	s := snapOf(t, p, string(src.Bytes()))
	b := s.Node(3) // root, group, a, b
	if b.Text() != "b" || src.OriginalOffset(b.StartByte()) != 2+2*3 {
		t.Fatalf("%q -> %d", b.Text(), src.OriginalOffset(b.StartByte()))
	}
}

// ---- LineIndex --------------------------------------------------------------------------------------------------

func naivePos(src string, off int) Position {
	p := Position{}
	last := 0
	for i := 0; i < off; i++ {
		if src[i] == '\n' {
			p.Line++
			last = i + 1
		}
	}
	p.ByteCol = uint32(off - last)
	for t := src[last:off]; len(t) > 0; {
		r, w := utf8.DecodeRuneInString(t)
		t = t[w:]
		p.RuneCol++
		p.UTF16Col++
		if r >= 0x10000 {
			p.UTF16Col++
		}
	}
	return p
}

func TestLineIndex(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for k := 0; k < 40; k++ {
		src := randSource(r, 5+r.IntN(80)) + string([]byte{0xff, 'x', '\n', 0xc3}) + "\U0001F600z"
		x := NewLineIndex(src)
		if x.NumLines() != strings.Count(src, "\n")+1 || x.LineStart(0) != 0 {
			t.Fatal("NumLines")
		}
		for off := 0; off <= len(src); off++ {
			if got, want := x.Pos(uint32(off)), naivePos(src, off); got != want {
				t.Fatalf("%q off %d: got %+v want %+v", src, off, got, want)
			}
		}
		if got, want := x.Pos(uint32(len(src)+50)), naivePos(src, len(src)); got != want {
			t.Fatalf("clamp: %+v %+v", got, want)
		}
	}
	if NewLineIndex("").NumLines() != 1 {
		t.Fatal("empty source has one line")
	}
}

func TestLineIndexMatchesSnapshotPoints(t *testing.T) {
	p := newP(t, newFakeLang(nil))
	r := rand.New(rand.NewPCG(5, 6))
	for k := 0; k < 30; k++ {
		s := snapOf(t, p, randSource(r, 60))
		lx := s.Lines()
		if s.Lines() != lx {
			t.Fatal("Lines must be cached")
		}
		for n := range s.All() {
			a, b := lx.Pos(n.StartByte()), lx.Pos(n.EndByte())
			if uint32(a.Line) != n.StartPoint().Row || a.ByteCol != n.StartPoint().Column ||
				uint32(b.Line) != n.EndPoint().Row || b.ByteCol != n.EndPoint().Column {
				t.Fatalf("node %d: index %+v %+v, snapshot %v %v", n.Index(), a, b, n.StartPoint(), n.EndPoint())
			}
		}
	}
}

// ---- Merkle hashes ----------------------------------------------------------------------------------------------

func canon(s *Snapshot, i int32, shape bool, dst []byte) []byte {
	f := s.flags[i] & (flagNamed | flagExtra | flagMissing | flagError)
	if shape {
		f &^= flagExtra
	}
	dst = append(dst, '(')
	dst = strconv.AppendUint(dst, uint64(s.symbol[i]), 10)
	dst = append(dst, '@')
	dst = strconv.AppendUint(dst, uint64(s.field[i]), 10)
	dst = append(dst, ':')
	dst = strconv.AppendUint(dst, uint64(f), 10)
	if c, ok := s.firstChild(i); !ok {
		text := s.src[s.startByte[i]:s.endByte[i]]
		dst = append(dst, ' ')
		dst = strconv.AppendInt(dst, int64(len(text)), 10)
		dst = append(dst, ':')
		dst = append(dst, text...)
	} else {
		for ; ok; c, ok = s.nextSibling(c) {
			if shape && s.flags[c]&flagExtra != 0 {
				continue
			}
			dst = canon(s, c, shape, dst)
		}
	}
	return append(dst, ')')
}

func TestHashMixHasNoFixedPointAtZero(t *testing.T) {
	for h := uint64(0); h < 5000; h++ {
		if hmix(h, h) == 0 {
			t.Fatalf("hmix(%d,%d) == 0: equal operands must not collapse (T2 regression)", h, h)
		}
	}
}

// Equal-hash groups are verified by full canonical serialization, both ways, across many snapshots of one language.
func TestMerkleGroupsMatchCanonicalSerialization(t *testing.T) {
	fl := newFakeLang(func(_ *fakeLang, f *fakeModule) { // make some nodes extra so ShapeHash is exercised
		f.corrupt = func(n []fnode) {
			for i := range n {
				if i > 2 && i%7 == 0 {
					n[i].flags |= flagExtra
				}
			}
		}
	})
	p := newP(t, fl)
	r := rand.New(rand.NewPCG(7, 8))
	for _, shape := range []bool{false, true} {
		byHash, byCanon := map[uint64]string{}, map[string]uint64{}
		nodes := 0
		for k := 0; k < 150; k++ {
			s := snapOf(t, p, randSource(r, 3+r.IntN(40)))
			for n := range s.All() {
				h := n.Hash()
				if shape {
					h = n.ShapeHash()
				}
				c := string(canon(s, int32(n.Index()), shape, nil))
				if prev, ok := byHash[h]; ok && prev != c {
					t.Fatalf("shape=%v: equal hash %x, different serialization:\n%s\n%s", shape, h, prev, c)
				}
				if prev, ok := byCanon[c]; ok && prev != h {
					t.Fatalf("shape=%v: equal serialization, different hash %x %x: %s", shape, prev, h, c)
				}
				byHash[h], byCanon[c] = c, h
				nodes++
			}
		}
		if len(byHash) < 100 || nodes < 1000 {
			t.Fatalf("too little coverage: %d distinct subtrees in %d nodes", len(byHash), nodes)
		}
		t.Logf("shape=%v: %d nodes, %d distinct subtrees, no collisions", shape, nodes, len(byHash))
	}
}

func TestShapeHashIgnoresExtras(t *testing.T) {
	fl := newFakeLang(func(fl *fakeLang, f *fakeModule) {
		if len(fl.mods) == 1 {
			f.corrupt = func(n []fnode) { n[3].flags |= flagExtra } // "(a x b)": x (node 3) is a comment
		}
	})
	p := newP(t, fl)
	withExtra := snapOf(t, p, "(a x b)")
	p2 := newP(t, newFakeLang(nil))
	plain := snapOf(t, p2, "(a b)")
	g1, g2 := withExtra.Node(1), plain.Node(1)
	if g1.ShapeHash() != g2.ShapeHash() {
		t.Fatal("ShapeHash must ignore the extra child")
	}
	if g1.Hash() == g2.Hash() {
		t.Fatal("Hash must see the extra child")
	}
	other := snapOf(t, p2, "(a c)")
	if other.Node(1).ShapeHash() == g2.ShapeHash() {
		t.Fatal("ShapeHash must still see a changed token")
	}
}

// ---- Atoms ------------------------------------------------------------------------------------------------------

func TestAtoms(t *testing.T) {
	p := newP(t, newFakeLang(nil))
	s := snapOf(t, p, "foo bar foo + baz bar")
	var atoms []Atom
	for c := range s.Root().Children() {
		atoms = append(atoms, c.Atom())
	}
	// foo bar foo + baz bar
	if atoms[0] == 0 || atoms[0] != atoms[2] || atoms[1] != atoms[5] || atoms[3] != 0 || atoms[0] == atoms[1] || atoms[4] == atoms[0] {
		t.Fatalf("atoms %v", atoms)
	}
	if s.Root().Atom() != 0 || s.NumAtoms() != 3 {
		t.Fatalf("root atom %d, %d atoms", s.Root().Atom(), s.NumAtoms())
	}
	for c := range s.Root().NamedChildren() {
		if s.AtomText(c.Atom()) != c.Text() {
			t.Fatal("AtomText")
		}
	}
	if a := testing.AllocsPerRun(1000, func() { _ = s.AtomText(atoms[0]) }); a != 0 {
		t.Fatalf("AtomText allocates %v", a)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("AtomText(0) must panic")
		}
	}()
	s.AtomText(0)
}

func TestLazyPrimitivesAreConcurrencySafe(t *testing.T) {
	p := newP(t, newFakeLang(nil))
	s := snapOf(t, p, "(a (b c)) d (e f) g")
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range s.All() {
				_ = n.Hash() + n.ShapeHash() + uint64(n.Atom())
			}
			_ = s.Lines().Pos(3)
			_ = s.Index().OfType("word")
		}()
	}
	wg.Wait()
}

// ---- Index ------------------------------------------------------------------------------------------------------

func TestIndexAgainstBruteForce(t *testing.T) {
	fl := newFakeLang(func(_ *fakeLang, f *fakeModule) {
		f.corrupt = func(n []fnode) { // an anonymous symbol that shares the name "word" with named symbol 2
			for i := range n {
				if i > 1 && i%5 == 0 && n[i].sym == fWord {
					n[i].sym, n[i].flags = fAnon, 0
				}
			}
		}
	})
	p := newP(t, fl)
	r := rand.New(rand.NewPCG(11, 12))
	for k := 0; k < 60; k++ {
		src := randSource(r, 5+r.IntN(60))
		s := snapOf(t, p, src)
		x := s.Index()
		if s.Index() != x {
			t.Fatal("Index must be cached")
		}
		for _, kind := range []string{"word", "+", "group", "source_file", "ERROR", "nope"} {
			var want, got []int
			for n := range s.All() {
				if n.Kind() == kind {
					want = append(want, n.Index())
				}
			}
			for n := range x.OfType(kind) {
				got = append(got, n.Index())
			}
			if !slices.Equal(want, got) {
				t.Fatalf("OfType(%q): got %v want %v", kind, got, want)
			}
		}
		for _, sym := range []Symbol{fWord, fAnon, fGroup, ErrorSymbol, 99} {
			var want, got []int
			for n := range s.All() {
				if n.Symbol() == sym {
					want = append(want, n.Index())
				}
			}
			for n := range x.OfSymbol(sym) {
				got = append(got, n.Index())
			}
			if !slices.Equal(want, got) {
				t.Fatalf("OfSymbol(%d): got %v want %v", sym, got, want)
			}
		}
		for off := uint32(0); off <= uint32(len(src)); off++ { // deepest containing half-open range, ties by preorder
			want, wantDepth := -1, -1
			for n := range s.All() {
				if n.StartByte() <= off && off < n.EndByte() {
					d := 0
					for q, ok := n.Parent(); ok; q, ok = q.Parent() {
						d++
					}
					if d > wantDepth {
						want, wantDepth = n.Index(), d
					}
				}
			}
			got, ok := x.NodeAt(off)
			if (want >= 0) != ok || (ok && got.Index() != want) {
				t.Fatalf("NodeAt(%d) in %q: got %d,%v want %d", off, src, got.Index(), ok, want)
			}
		}
	}
}

// ---- ParserPool -------------------------------------------------------------------------------------------------

func TestParserPoolReuseAndInvalidation(t *testing.T) {
	fl := newFakeLang(nil)
	pl, err := NewParserPool(fl.lang, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer pl.Close()
	p1, err := pl.Get(bg)
	if err != nil {
		t.Fatal(err)
	}
	tr1, tr2 := mustParse(t, p1, "a b"), mustParse(t, p1, "c d")
	snap, _ := tr1.Snapshot(bg)
	pl.Put(p1)
	f := fl.mods[0]
	if f.count("delete") != 2 || len(f.trees) != 0 || len(fl.mods) != 1 {
		t.Fatalf("Put must free both live trees and keep the instance: deletes %d trees %d instances %d", f.count("delete"), len(f.trees), len(fl.mods))
	}
	for _, tr := range []*Tree{tr1, tr2} {
		if _, err := tr.Snapshot(bg); !errors.Is(err, ErrTreeInvalidated) {
			t.Fatalf("tree after Put: %v", err)
		}
	}
	if snap.Root().Text() != "a b" {
		t.Fatal("snapshots survive Put")
	}
	p2, _ := pl.Get(bg)
	if p2 != p1 || len(fl.mods) != 1 {
		t.Fatal("an idle parser must be reused")
	}
	if tr := mustParse(t, p2, "x"); tr.Outcome() != AcceptedClean {
		t.Fatal("reused parser must parse")
	}
	pl.Put(p2)
	pl.Put(p2) // double Put and foreign parsers are ignored
	pl.Put(nil)
	pl.Put(newP(t, newFakeLang(nil)))
	a, _ := pl.Get(bg)
	b, _ := pl.Get(bg)
	if a == b {
		t.Fatal("a parser must not be lent twice")
	}
}

func TestParserPoolBoundAndContext(t *testing.T) {
	pl, _ := NewParserPool(newFakeLang(nil).lang, 1)
	p, _ := pl.Get(bg)
	ctx, cancel := context.WithTimeout(bg, 30*time.Millisecond)
	defer cancel()
	if _, err := pl.Get(ctx); !errors.Is(err, ErrTimeout) {
		t.Fatalf("a bounded pool must wait for the context: %v", err)
	}
	got := make(chan *Parser, 1)
	go func() { q, _ := pl.Get(bg); got <- q }()
	time.Sleep(20 * time.Millisecond)
	pl.Put(p)
	if q := <-got; q != p {
		t.Fatal("the waiting Get must receive the returned parser")
	}
	done, cancel2 := context.WithCancel(bg)
	cancel2()
	if _, err := pl.Get(done); !errors.Is(err, ErrCancelled) {
		t.Fatalf("done context: %v", err)
	}
	//lint:ignore SA1012 deliberately nil
	if _, err := pl.Get(nil); !errors.Is(err, ErrInvalidInput) { //nolint
		t.Fatalf("nil context: %v", err)
	}
	if _, err := NewParserPool(nil, 0); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("nil language")
	}
	if _, err := NewParserPool(newFakeLang(nil).lang, -1); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("negative bound")
	}
	if _, err := NewParserPool(newFakeLang(nil).lang, 0, WithMaxInputBytes(-1)); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("options are validated at construction")
	}
}

func TestParserPoolClosedParserAndPoolClose(t *testing.T) {
	fl := newFakeLang(nil)
	pl, _ := NewParserPool(fl.lang, 0)
	p, _ := pl.Get(bg)
	p.Close()
	pl.Put(p)
	q, _ := pl.Get(bg)
	if q == p || len(fl.mods) != 2 {
		t.Fatal("a closed parser must not be reused")
	}
	pl.Put(q)
	pl.Close()
	if !fl.mods[1].released.Load() {
		t.Fatal("Close must close the idle parsers")
	}
	if _, err := pl.Get(bg); !errors.Is(err, ErrParserClosed) {
		t.Fatalf("Get after Close: %v", err)
	}
	r, _ := func() (*Parser, error) {
		pl2, _ := NewParserPool(fl.lang, 0)
		x, e := pl2.Get(bg)
		pl2.Close()
		pl2.Put(x)
		return x, e
	}()
	if _, err := r.Parse(bg, []byte("a"), nil); !errors.Is(err, ErrParserClosed) {
		t.Fatalf("a lent parser is closed when Put into a closed pool: %v", err)
	}
}

func TestParserPoolConcurrent(t *testing.T) {
	fl := newFakeLang(nil)
	pl, _ := NewParserPool(fl.lang, 3)
	defer pl.Close()
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for g := 0; g < 12; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				p, err := pl.Get(bg)
				if err != nil {
					errs <- err
					return
				}
				tr, err := p.Parse(bg, []byte("(a b) "+strconv.Itoa(g*100+i)), nil)
				if err != nil {
					errs <- err
					return
				}
				if s, err := tr.Snapshot(bg); err != nil || s.Root().EndByte() != uint32(tr.InputBytes()) {
					errs <- errors.New("bad snapshot")
					return
				}
				pl.Put(p) // the tree is deliberately left open: Put frees it
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if len(fl.mods) > 3 {
		t.Fatalf("a pool bounded to 3 created %d instances", len(fl.mods))
	}
	for _, f := range fl.mods {
		if len(f.trees) != 0 {
			t.Fatalf("%d trees leaked in an instance", len(f.trees))
		}
	}
}
