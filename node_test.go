package treesitter

import (
	"slices"
	"testing"
)

// "(a (b c)) d" parses (in the fake) to
//
//	0 source_file [0,11)  1 group [0,9)  2 word a [1,2)  3 group [3,8)  4 word b [4,5)  5 word c [6,7)
//	6 word d [10,11) field "left"
const sample = "(a (b c)) d"

func sampleSnapshot(t *testing.T) *Snapshot {
	t.Helper()
	p := newP(t, newFakeLang(nil))
	s, err := p.ParseSnapshot(bg, []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestWalkAllAndStructure(t *testing.T) {
	s := sampleSnapshot(t)
	if s.Len() != 7 || s.Root().Index() != 0 || s.Language().Name() != "fake" {
		t.Fatalf("len %d", s.Len())
	}
	var kinds []string
	var depths []int
	s.Walk(func(n Node, d int) WalkAction {
		kinds = append(kinds, n.Kind())
		depths = append(depths, d)
		return Continue
	})
	if !slices.Equal(depths, []int{0, 1, 2, 2, 3, 3, 1}) || len(kinds) != 7 || kinds[1] != "group" {
		t.Fatalf("depths %v kinds %v", depths, kinds)
	}
	depths = nil
	s.Walk(func(n Node, d int) WalkAction {
		depths = append(depths, d)
		if n.Kind() == "group" {
			return SkipChildren
		}
		return Continue
	})
	if !slices.Equal(depths, []int{0, 1, 1}) { // root, group (skipped), d
		t.Fatalf("skip: %v", depths)
	}
	k := 0
	s.Walk(func(Node, int) WalkAction { k++; return map[bool]WalkAction{true: Stop, false: Continue}[k == 3] })
	if k != 3 {
		t.Fatalf("stop: %d", k)
	}
	var idx []int
	for n := range s.All() {
		idx = append(idx, n.Index())
	}
	if !slices.Equal(idx, []int{0, 1, 2, 3, 4, 5, 6}) {
		t.Fatalf("All: %v", idx)
	}
	for n := range s.All() { // early break
		if n.Index() == 2 {
			break
		}
	}
}

func TestNavigation(t *testing.T) {
	s := sampleSnapshot(t)
	root, g1 := s.Root(), s.Node(1)
	if root.ChildCount() != 2 || g1.ChildCount() != 2 || s.Node(2).ChildCount() != 0 {
		t.Fatal("child counts")
	}
	if c, ok := root.Child(1); !ok || c.Index() != 6 {
		t.Fatal("Child(1)")
	}
	if _, ok := root.Child(2); ok {
		t.Fatal("Child out of range")
	}
	if _, ok := root.Child(-1); ok {
		t.Fatal("Child negative")
	}
	if p, ok := s.Node(4).Parent(); !ok || p.Index() != 3 {
		t.Fatal("Parent")
	}
	if _, ok := root.Parent(); ok {
		t.Fatal("root has no parent")
	}
	if n, ok := s.Node(2).NextSibling(); !ok || n.Index() != 3 {
		t.Fatal("NextSibling")
	}
	if _, ok := s.Node(5).NextSibling(); ok {
		t.Fatal("last child of group 3 has no next sibling")
	}
	if n, ok := s.Node(6).PrevSibling(); !ok || n.Index() != 1 {
		t.Fatal("PrevSibling")
	}
	if _, ok := s.Node(2).PrevSibling(); ok {
		t.Fatal("first child has no prev sibling")
	}
	if _, ok := root.NextSibling(); ok {
		t.Fatal("root sibling")
	}
	if n, ok := s.Node(2).NextNamedSibling(); !ok || n.Index() != 3 {
		t.Fatal("NextNamedSibling")
	}
	if n, ok := s.Node(3).PrevNamedSibling(); !ok || n.Index() != 2 {
		t.Fatal("PrevNamedSibling")
	}
	if g1.NamedChildCount() != 2 {
		t.Fatal("NamedChildCount")
	}
	if c, ok := g1.NamedChild(1); !ok || c.Index() != 3 {
		t.Fatal("NamedChild")
	}
	if _, ok := g1.NamedChild(2); ok {
		t.Fatal("NamedChild range")
	}
	var got []int
	for c := range g1.Children() {
		got = append(got, c.Index())
	}
	if !slices.Equal(got, []int{2, 3}) {
		t.Fatalf("Children %v", got)
	}
}

func TestAnonymousAndNamedNavigation(t *testing.T) {
	p := newP(t, newFakeLang(nil))
	s, _ := p.ParseSnapshot(bg, []byte("a + b"))
	r := s.Root()
	if r.ChildCount() != 3 || r.NamedChildCount() != 2 {
		t.Fatalf("%d %d", r.ChildCount(), r.NamedChildCount())
	}
	plus, _ := r.Child(1)
	if plus.IsNamed() || plus.Kind() != "+" {
		t.Fatal("anonymous child")
	}
	if n, ok := plus.NextNamedSibling(); !ok || n.Text() != "b" {
		t.Fatal("NextNamedSibling skips nothing here")
	}
	if n, ok := r.NamedChild(1); !ok || n.Text() != "b" {
		t.Fatal("NamedChild skips anonymous nodes")
	}
}

func TestFields(t *testing.T) {
	s := sampleSnapshot(t)
	root := s.Root()
	d, ok := root.ChildByFieldName("left")
	if !ok || d.Index() != 6 || d.Field() != "left" || d.FieldID() != 1 {
		t.Fatal("ChildByFieldName")
	}
	if _, ok := root.ChildByFieldName("right"); ok {
		t.Fatal("absent field")
	}
	if _, ok := root.ChildByFieldName("nope"); ok {
		t.Fatal("unknown field")
	}
	if n := 0; true {
		for range root.ChildrenByFieldName("left") {
			n++
		}
		if n != 1 {
			t.Fatalf("ChildrenByFieldName %d", n)
		}
	}
	if s.Node(2).Field() != "" || s.Node(2).FieldID() != 0 {
		t.Fatal("no field")
	}
	l := s.Language()
	if id, ok := l.FieldForName("right"); !ok || id != 2 || l.FieldName(2) != "right" || l.FieldName(99) != "" {
		t.Fatal("field lookup")
	}
}

func TestSymbolForName(t *testing.T) {
	l := newFakeLang(nil).lang
	cases := []struct {
		name  string
		named bool
		want  Symbol
		ok    bool
	}{
		{"word", true, 2, true}, {"word", false, 6, true}, {"+", false, 3, true}, {"+", true, 0, false},
		{"source_file", true, 1, true}, {"_hidden", true, 0, false}, {"end", false, 0, false},
		{"ERROR", true, ErrorSymbol, true}, {"ERROR", false, 0, false}, {"missing", true, 0, false},
	}
	for _, c := range cases {
		if got, ok := l.SymbolForName(c.name, c.named); got != c.want || ok != c.ok {
			t.Errorf("SymbolForName(%q,%v) = %d,%v want %d,%v", c.name, c.named, got, ok, c.want, c.ok)
		}
	}
	if l.SymbolName(ErrorSymbol) != "ERROR" || l.SymbolName(2) != "word" || l.SymbolName(300) != "" || l.SymbolCount() != 7 {
		t.Fatal("SymbolName")
	}
}

func TestDescendantForRange(t *testing.T) {
	s := sampleSnapshot(t)
	idx := func(n Node) int { return n.Index() }
	for _, c := range []struct {
		start, end uint32
		named      bool
		want       int
	}{
		{4, 5, true, 4}, {4, 7, true, 3}, {0, 0, true, 1}, {9, 9, true, 0}, {10, 11, false, 6},
		{4, 5, false, 4}, {0, 11, true, 0}, {0, 99, true, 0},
	} {
		if got := idx(s.DescendantForByteRange(c.start, c.end, c.named)); got != c.want {
			t.Errorf("DescendantForByteRange(%d,%d,%v) = %d want %d", c.start, c.end, c.named, got, c.want)
		}
	}
	p := newP(t, newFakeLang(nil))
	m, _ := p.ParseSnapshot(bg, []byte("a\n+ b"))
	if n := m.DescendantForByteRange(2, 3, false); n.Kind() != "+" {
		t.Fatalf("anonymous node: %s", n.Kind())
	}
	if n := m.DescendantForByteRange(2, 3, true); n.Index() != 0 {
		t.Fatalf("named=true skips the anonymous node: %s", n.Kind())
	}
	if n := m.DescendantForPointRange(Point{1, 0}, Point{1, 1}, false); n.Kind() != "+" {
		t.Fatalf("point range: %s", n.Kind())
	}
	if n := m.DescendantForPointRange(Point{1, 2}, Point{1, 3}, true); n.Text() != "b" {
		t.Fatalf("point range word: %s", n.Text())
	}
}

func TestStringSexp(t *testing.T) {
	s := sampleSnapshot(t)
	want := "(source_file (group (word) (group (word) (word))) left: (word))"
	if got := s.Root().String(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	p := newP(t, newFakeLang(nil))
	e, _ := p.ParseSnapshot(bg, []byte("a + !"))
	if got, want := e.Root().String(), "(source_file left: (word) (ERROR))"; got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if k := e.Node(3).Kind(); k != "ERROR" || !e.Node(3).IsError() || !e.Root().HasError() || e.Node(1).HasError() {
		t.Fatalf("error flags: %s", k)
	}
	empty, _ := p.ParseSnapshot(bg, nil)
	if empty.Len() != 1 || empty.Root().EndByte() != 0 || empty.Root().Text() != "" {
		t.Fatal("empty input")
	}
}

func TestNodeAccessors(t *testing.T) {
	p := newP(t, newFakeLang(nil))
	s, _ := p.ParseSnapshot(bg, []byte("  ab\n cd"))
	root := s.Root()
	if root.StartByte() != 2 || root.EndByte() != 8 { // root starts after the leading padding, ends at the input length
		t.Fatalf("root [%d,%d)", root.StartByte(), root.EndByte())
	}
	cd := s.Node(2)
	if cd.StartPoint() != (Point{1, 1}) || cd.EndPoint() != (Point{1, 3}) || cd.Text() != "cd" || cd.Symbol() != 2 || cd.Kind() != "word" {
		t.Fatalf("cd %v %v %q", cd.StartPoint(), cd.EndPoint(), cd.Text())
	}
	if !cd.IsNamed() || cd.IsExtra() || cd.IsMissing() || cd.Snapshot() != s {
		t.Fatal("flags")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("Snapshot.Node must panic out of range")
		}
	}()
	s.Node(3)
}

func TestTextAllocations(t *testing.T) {
	s := sampleSnapshot(t)
	n := s.Node(4)
	var sink string
	if a := testing.AllocsPerRun(1000, func() { sink = n.Text() }); a != 0 {
		t.Fatalf("Text allocates %v", a)
	}
	buf := make([]byte, 0, 16)
	if a := testing.AllocsPerRun(1000, func() { buf = n.AppendText(buf[:0]) }); a != 0 {
		t.Fatalf("AppendText allocates %v", a)
	}
	if sink != "b" || string(buf) != "b" {
		t.Fatal("text")
	}
}

func TestInvalidUTF8Flag(t *testing.T) {
	p := newP(t, newFakeLang(nil))
	s, err := p.ParseSnapshot(bg, []byte("a\xff"))
	if err != nil || !s.HasInvalidUTF8() {
		t.Fatalf("%v %v", s, err)
	}
}
