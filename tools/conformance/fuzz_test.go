package conformance

import (
	"context"
	"errors"
	"testing"
	"time"

	treesitter "github.com/wotjr1649/go-treesitter"
)

// CGO-free fuzz targets, one per grammar. They need no TSG_S15_DIR: the seeds are small built-in samples and the
// properties are self-contained (no native reference). Run one with, for example:
//
//	go test -run '^$' -fuzz '^FuzzParseJSON$' -fuzztime 60s .
//
// A crasher is kept by the Go tool under testdata/fuzz/<target>/ and must be reported, not fixed here.
//
// Properties, for every input: Parse either returns a *ParseError (never another error type or a panic) or an accepted
// tree whose Outcome agrees with Root().HasError() and whose root ends at len(src); the snapshot's Len equals the
// number of nodes reachable from the root; every node's Text() is the source slice [StartByte, EndByte);
// ParseSnapshot gives the same tree as Tree.Snapshot; an EditFor edit + incremental reparse never panics, returns an
// accepted tree with the same invariants (the new tree is NOT required to equal a fresh parse), and ChangedRanges on
// the pair succeeds with in-bounds, ordered ranges.

const fuzzMaxInput = 1 << 16

func FuzzParseJSON(f *testing.F) {
	fuzzParse(f, "json", `{"a": [1, 2.5, true, null], "b": {"c": "d\n"}}`, `[`, `{"k":`, "\"\xff\"", ``)
}

func FuzzParseGo(f *testing.F) {
	fuzzParse(f, "go", "package p\n\nfunc f(a int) int {\n\treturn a + 1\n}\n", "package", "func (", "var x = \"é\"\n", "")
}

func FuzzParsePython(f *testing.F) {
	fuzzParse(f, "python", "def f(a):\n    if a:\n        return [x for x in a]\n    return None\n", "def (", "class A:\n  x = '''\n", "\tif 1:\n  pass\n", "")
}

func FuzzParseTypeScript(f *testing.F) {
	fuzzParse(f, "typescript", "interface I { a: number }\nfunction f<T>(x: T): T { return x }\n", "let x: = ;", "class A<", "enum E { A = 1 }", "")
}

func FuzzParseTSX(f *testing.F) {
	fuzzParse(f, "tsx", "const A = (p: {n: number}) => <div id=\"a\">{p.n}</div>;\n", "<div", "const x = <T,>(a: T) => a", "<>{}</>", "")
}

func FuzzParseCSharp(f *testing.F) {
	fuzzParse(f, "csharp", "class A { int F(int x) => x + 1; }\n", "namespace N {", "var s = $\"{x}\";", "class { ;", "")
}

func FuzzParseTSQL(f *testing.F) {
	fuzzParse(f, "tsql", "SELECT a, b FROM t WHERE a > 1;\n", "CREATE TABLE", "SELECT (", "DECLARE @x INT = 1;\nGO\n", "")
}

func FuzzParseJavaScript(f *testing.F) {
	fuzzParse(f, "javascript", "function f(x) { return x + 1; }\n", "const x = <div/>;\n", "function (", "")
}

func FuzzParseHTML(f *testing.F) {
	fuzzParse(f, "html", "<main><p>Hello</p></main>\n", "<div", "<!--", "")
}

func FuzzParseCSS(f *testing.F) {
	fuzzParse(f, "css", "a { color: red; }\n", "@media screen {", "a { color:", "")
}

func FuzzParseJava(f *testing.F) {
	fuzzParse(f, "java", "class A { int f() { return 1; } }\n", "interface A {", "class {", "")
}

func FuzzParseC(f *testing.F) {
	fuzzParse(f, "c", "int f(void) { return 1; }\n", "struct A {", "int f(", "")
}

func FuzzParseRust(f *testing.F) {
	fuzzParse(f, "rust", "fn f() -> i32 { 1 }\n", "struct A {", "fn f(", "")
}

func FuzzParseBash(f *testing.F) {
	fuzzParse(f, "bash", "echo \"$HOME\"\n", "f() { echo a; }\n", "if true; then", "")
}

func FuzzParseCPP(f *testing.F) {
	fuzzParse(f, "cpp", "int f() { return 1; }\n", "template <typename T> struct A {};\n", "class A {", "")
}

func fuzzParse(f *testing.F, lang string, seeds ...string) {
	mk, ok := languages[lang]
	if !ok {
		f.Skipf("grammar %q is not linked into this binary", lang)
	}
	p, err := treesitter.NewParser(mk())
	if err != nil {
		f.Fatalf("NewParser(%s): %v", lang, err)
	}
	f.Cleanup(func() { p.Close() })
	for _, s := range seeds {
		f.Add([]byte(s), uint16(0), uint16(0), []byte("x"))
		f.Add([]byte(s), uint16(len(s)/2), uint16(2), []byte(" 1 "))
		f.Add([]byte(s), uint16(len(s)), uint16(0), []byte("\n"))
	}
	f.Fuzz(func(t *testing.T, src []byte, a, b uint16, repl []byte) {
		if len(src) > fuzzMaxInput || len(repl) > fuzzMaxInput {
			t.Skip()
		}
		ctx, cancel := context.WithTimeout(bg, 30*time.Second)
		defer cancel()
		tree, err := p.Parse(ctx, src, nil)
		if err != nil {
			checkParseError(t, "Parse", err)
			return
		}
		defer tree.Close()
		snap, err := tree.Snapshot(ctx)
		if err != nil {
			checkParseError(t, "Snapshot", err)
			return
		}
		checkTree(t, "fresh", tree, snap, src)

		if s2, err := p.ParseSnapshot(ctx, src); err != nil {
			checkParseError(t, "ParseSnapshot", err)
		} else if s2.Root().String() != snap.Root().String() || s2.Len() != snap.Len() {
			t.Fatalf("ParseSnapshot differs from Tree.Snapshot: %d vs %d nodes", s2.Len(), snap.Len())
		}

		// One edit derived from the fuzz input: replace src[start:oldEnd] with repl.
		start := int(a) % (len(src) + 1)
		oldEnd := start + int(b)%(len(src)-start+1)
		next := append(append(append([]byte{}, src[:start]...), repl...), src[oldEnd:]...)
		newEnd := start + len(repl)
		if err := tree.Edit(treesitter.EditFor(src, next, uint32(start), uint32(oldEnd), uint32(newEnd))); err != nil {
			t.Fatalf("Edit(%d,%d,%d) of a %d-byte tree: %v", start, oldEnd, newEnd, len(src), err)
		}
		inc, err := p.Parse(ctx, next, tree)
		if err != nil {
			checkParseError(t, "Parse(incremental)", err)
			return
		}
		defer inc.Close()
		isnap, err := inc.Snapshot(ctx)
		if err != nil {
			checkParseError(t, "Snapshot(incremental)", err)
			return
		}
		checkTree(t, "incremental", inc, isnap, next)
		ranges, err := inc.ChangedRanges(tree)
		if err != nil {
			checkParseError(t, "ChangedRanges", err)
			return
		}
		var prev uint32
		for i, r := range ranges {
			if r.StartByte > r.EndByte || int(r.EndByte) > len(next) || (i > 0 && r.StartByte < prev) {
				t.Fatalf("ChangedRanges[%d] = %+v out of order or beyond the %d-byte input (previous end %d)", i, r, len(next), prev)
			}
			prev = r.EndByte
		}
	})
}

// checkParseError fails unless err is a *ParseError of a kind an arbitrary (possibly invalid) input may legitimately
// produce. EngineFault means a trap or a broken invariant inside the engine: always a finding. Timeout/Cancelled here
// means a 30 s hang on a small input: also a finding.
func checkParseError(t *testing.T, what string, err error) {
	t.Helper()
	var pe *treesitter.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("%s: error is %T, not *ParseError: %v", what, err, err)
	}
	switch pe.Outcome {
	case treesitter.EngineFault, treesitter.Timeout, treesitter.Cancelled, treesitter.IncompatibleLanguage:
		t.Fatalf("%s: unexpected %v", what, pe)
	}
}

func checkTree(t *testing.T, what string, tree *treesitter.Tree, snap *treesitter.Snapshot, src []byte) {
	t.Helper()
	root := snap.Root()
	if int(root.EndByte()) != len(src) {
		t.Fatalf("%s: root ends at %d, input has %d bytes", what, root.EndByte(), len(src))
	}
	if o := tree.Outcome(); o != treesitter.AcceptedClean && o != treesitter.AcceptedWithErrors {
		t.Fatalf("%s: tree has outcome %v", what, o)
	} else if (o == treesitter.AcceptedWithErrors) != root.HasError() {
		t.Fatalf("%s: Outcome %v but Root().HasError() = %v", what, o, root.HasError())
	}
	count := 0
	var walk func(n treesitter.Node)
	walk = func(n treesitter.Node) {
		count++
		s, e := n.StartByte(), n.EndByte()
		if s > e || int(e) > len(src) {
			t.Fatalf("%s: node %s has range [%d,%d) in a %d-byte input", what, n.Kind(), s, e, len(src))
		}
		if n.Text() != string(src[s:e]) {
			t.Fatalf("%s: node %s Text() %q != source slice %q", what, n.Kind(), n.Text(), src[s:e])
		}
		for c := range n.Children() {
			walk(c)
		}
	}
	walk(root)
	if count != snap.Len() {
		t.Fatalf("%s: %d nodes reachable from the root, Snapshot.Len() = %d", what, count, snap.Len())
	}
}
