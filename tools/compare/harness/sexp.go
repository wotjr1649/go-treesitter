package harness

import (
	"fmt"
	"strings"
)

// SNode is one visible node of a ts_node_string-style S-expression.
type SNode struct {
	Field   string
	Kind    string
	Missing bool
	Err     bool // ERROR or UNEXPECTED
	Kids    []*SNode
}

// ParseSexp reads the format shared by ts_node_string and the candidates' public S-expression calls:
// (kind field: (child) ...), (MISSING kind), (MISSING "x"), (ERROR ...), (UNEXPECTED 'c'). Text it cannot place is an error.
func ParseSexp(s string) (*SNode, error) {
	p := &sexpParser{s: s}
	n, err := p.node("")
	if err != nil {
		return nil, err
	}
	p.skip()
	if p.i != len(p.s) {
		return nil, fmt.Errorf("trailing text at byte %d", p.i)
	}
	return n, nil
}

type sexpParser struct {
	s string
	i int
}

func (p *sexpParser) skip() {
	for p.i < len(p.s) && strings.IndexByte(" \t\r\n", p.s[p.i]) >= 0 {
		p.i++
	}
}

// atom reads up to whitespace or a parenthesis; a quoted run ("..." or '...') is one atom with backslash escapes.
func (p *sexpParser) atom() string {
	p.skip()
	start := p.i
	if p.i < len(p.s) && (p.s[p.i] == '"' || p.s[p.i] == '\'') {
		q := p.s[p.i]
		p.i++
		for p.i < len(p.s) && p.s[p.i] != q {
			if p.s[p.i] == '\\' {
				p.i++
			}
			p.i++
		}
		p.i++
		return p.s[start:min(p.i, len(p.s))]
	}
	for p.i < len(p.s) && strings.IndexByte(" \t\r\n()", p.s[p.i]) < 0 {
		p.i++
	}
	return p.s[start:p.i]
}

func (p *sexpParser) node(field string) (*SNode, error) {
	p.skip()
	if p.i >= len(p.s) || p.s[p.i] != '(' {
		return nil, fmt.Errorf("expected '(' at byte %d", p.i)
	}
	p.i++
	n := &SNode{Field: field}
	switch kind := p.atom(); kind {
	case "":
		return nil, fmt.Errorf("empty node kind at byte %d", p.i)
	case "MISSING":
		n.Missing, n.Kind = true, p.atom()
	case "UNEXPECTED":
		n.Err, n.Kind = true, "ERROR"
		p.atom()
	case "ERROR":
		n.Err, n.Kind = true, kind
	default:
		n.Kind = kind
	}
	for {
		p.skip()
		if p.i >= len(p.s) {
			return nil, fmt.Errorf("unterminated node %q", n.Kind)
		}
		if p.s[p.i] == ')' {
			p.i++
			return n, nil
		}
		f := ""
		if p.s[p.i] != '(' {
			f = p.atom()
			if !strings.HasSuffix(f, ":") {
				return nil, fmt.Errorf("unexpected %q at byte %d", f, p.i)
			}
			f = strings.TrimSuffix(f, ":")
		}
		c, err := p.node(f)
		if err != nil {
			return nil, err
		}
		n.Kids = append(n.Kids, c)
	}
}

// walk visits the tree in preorder with each node's path.
func (n *SNode) walk(path string, f func(path string, n *SNode)) {
	path += "/" + n.Kind
	f(path, n)
	for i, k := range n.Kids {
		k.walk(fmt.Sprintf("%s[%d]", path, i), f)
	}
}

// Levels of agreement between a candidate and its native reference, strongest first.
const (
	LevelExact    = "sexp-exact"    // the public strings are byte-identical
	LevelMarks    = "profile-marks" // kinds, field labels and ERROR/MISSING marks equal; only formatting differs
	LevelKinds    = "profile-kinds" // the ordered visible node kinds equal; fields or marks differ
	LevelMismatch = "mismatch"      // different node sequences
	LevelUnparsed = "unparsed"      // a side is not a recognizable S-expression
)

// Diff is the outcome of Compare.
type Diff struct {
	Level     string `json:"level"`
	FirstDiff string `json:"first_diff,omitempty"` // path of the first differing node, or the parse error
	Missing   int    `json:"missing"`              // reference nodes the candidate lacks (by kind multiset)
	Extra     int    `json:"extra"`                // candidate nodes the reference lacks
}

// Compare grades cand against ref. Only a raw mismatch that still matches a normalized profile may be called
// formatting-only; the report keeps the level name so no such claim is implied by Level alone.
func Compare(ref, cand string) Diff {
	if ref == cand {
		return Diff{Level: LevelExact}
	}
	a, errA := ParseSexp(strings.TrimSpace(ref))
	b, errB := ParseSexp(strings.TrimSpace(cand))
	if errA != nil || errB != nil {
		msg := "reference: "
		if errA == nil {
			msg = "candidate: " + errB.Error()
		} else {
			msg += errA.Error()
		}
		return Diff{Level: LevelUnparsed, FirstDiff: msg}
	}
	type sig struct {
		Field, Kind  string
		Missing, Err bool
	}
	type row struct {
		path string
		n    sig
	}
	flat := func(r *SNode) (out []row) {
		r.walk("", func(p string, n *SNode) { out = append(out, row{p, sig{n.Field, n.Kind, n.Missing, n.Err}}) })
		return out
	}
	fa, fb := flat(a), flat(b)
	d := Diff{Level: LevelMarks}
	kinds := true
	for i := 0; i < max(len(fa), len(fb)); i++ {
		switch {
		case i >= len(fa) || i >= len(fb):
			d.FirstDiff, kinds = firstNonEmpty(d.FirstDiff, pathOf(fa, fb, i, func(r row) string { return r.path })), false
		case fa[i].n.Kind != fb[i].n.Kind:
			d.FirstDiff, kinds = firstNonEmpty(d.FirstDiff, fa[i].path+" vs "+fb[i].path), false
		case fa[i].n != fb[i].n:
			d.FirstDiff = firstNonEmpty(d.FirstDiff, fa[i].path+" (field/mark)")
			d.Level = LevelKinds
		}
		if !kinds {
			break
		}
	}
	if !kinds {
		d.Level = LevelMismatch
	}
	count := map[string]int{}
	for _, r := range fa {
		count[r.n.Kind]++
	}
	for _, r := range fb {
		count[r.n.Kind]--
	}
	for _, c := range count {
		if c > 0 {
			d.Missing += c
		} else {
			d.Extra -= c
		}
	}
	return d
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func pathOf[T any](a, b []T, i int, f func(T) string) string {
	if i < len(a) {
		return f(a[i]) + " (candidate ends)"
	}
	return f(b[i]) + " (reference ends)"
}

// FormatSexp renders a tree the way ts_node_string does (visible nodes only: the caller leaves anonymous nodes out),
// for a library whose public API has no S-expression call. Its result is a documented harness serialization,
// not the library's own output.
func FormatSexp(n *SNode) string {
	var b strings.Builder
	var w func(n *SNode)
	w = func(n *SNode) {
		b.WriteByte('(')
		if n.Missing {
			b.WriteString("MISSING ")
		}
		b.WriteString(n.Kind)
		for _, k := range n.Kids {
			b.WriteByte(' ')
			if k.Field != "" {
				b.WriteString(k.Field + ": ")
			}
			w(k)
		}
		b.WriteByte(')')
	}
	w(n)
	return b.String()
}
