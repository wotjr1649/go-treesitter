package harness

import (
	"fmt"
	"strings"
)

// SNode is one visible node of a ts_node_string-style S-expression.
type SNode struct {
	Field      string
	Kind       string
	Missing    bool
	Err        bool   // ERROR or UNEXPECTED
	Unexpected bool   // printed as (UNEXPECTED 'c'): a leaf error; Text keeps the quoted token
	Text       string // the UNEXPECTED token text
	Kids       []*SNode
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
		n.Err, n.Unexpected, n.Kind = true, true, "ERROR"
		n.Text = p.atom()
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

// Levels of agreement between a candidate and its native reference, strongest first. Every level above "mismatch"
// compares the tree structure (parent/child relations in order); the levels differ only in what else must match.
const (
	LevelExact    = "sexp-exact" // the public strings are byte-identical
	LevelTree     = "tree-equal" // the same tree (kinds, fields, MISSING/ERROR marks, UNEXPECTED text); only formatting differs
	LevelLossy    = "tree-lossy" // the same tree once the token text of UNEXPECTED leaves is discarded (a lossy normalization)
	LevelShape    = "shape-only" // same parent/child shape and kinds, but field labels or marks differ
	LevelMismatch = "mismatch"   // different trees
	LevelUnparsed = "unparsed"   // a side is not a recognizable S-expression
)

// Diff is the outcome of Compare.
type Diff struct {
	Level     string `json:"level"`
	FirstDiff string `json:"first_diff,omitempty"` // path of the first differing node, or the parse error
	Missing   int    `json:"missing"`              // reference nodes the candidate lacks (by kind multiset)
	Extra     int    `json:"extra"`                // candidate nodes the reference lacks
}

type cmpMode int

const (
	cmpFull cmpMode = iota
	cmpLossy
	cmpShape
)

// same reports whether a and b are the same tree under the mode, comparing children in order.
func same(a, b *SNode, m cmpMode) bool {
	if a.Kind != b.Kind || len(a.Kids) != len(b.Kids) {
		return false
	}
	if m != cmpShape && (a.Field != b.Field || a.Missing != b.Missing || a.Err != b.Err) {
		return false
	}
	if m == cmpFull && (a.Unexpected != b.Unexpected || a.Text != b.Text) {
		return false
	}
	for i := range a.Kids {
		if !same(a.Kids[i], b.Kids[i], m) {
			return false
		}
	}
	return true
}

// firstDiff walks both trees in step and names the first node that differs, as a path of kinds with child indices.
func firstDiff(a, b *SNode, path string) (string, bool) {
	path += "/" + a.Kind
	if a.Kind != b.Kind {
		return path + " vs " + b.Kind, true
	}
	if a.Field != b.Field || a.Missing != b.Missing || a.Err != b.Err {
		return path + " (field or mark)", true
	}
	for i := 0; i < len(a.Kids) || i < len(b.Kids); i++ {
		switch {
		case i >= len(b.Kids):
			return fmt.Sprintf("%s[%d] (candidate ends)", path, i), true
		case i >= len(a.Kids):
			return fmt.Sprintf("%s[%d] (reference ends)", path, i), true
		}
		if p, ok := firstDiff(a.Kids[i], b.Kids[i], fmt.Sprintf("%s[%d]", path, i)); ok {
			return p, true
		}
	}
	return "", false
}

func countKinds(n *SNode, into map[string]int, delta int) {
	into[n.Kind] += delta
	for _, k := range n.Kids {
		countKinds(k, into, delta)
	}
}

// Compare grades cand against ref. A level other than mismatch/unparsed means the parent/child structure is equal.
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
	switch {
	case same(a, b, cmpFull):
		return Diff{Level: LevelTree}
	case same(a, b, cmpLossy):
		return Diff{Level: LevelLossy, FirstDiff: "UNEXPECTED token text discarded"}
	}
	d := Diff{Level: LevelShape}
	if !same(a, b, cmpShape) {
		d.Level = LevelMismatch
	}
	d.FirstDiff, _ = firstDiff(a, b, "")
	count := map[string]int{}
	countKinds(a, count, 1)
	countKinds(b, count, -1)
	for _, c := range count {
		if c > 0 {
			d.Missing += c
		} else {
			d.Extra -= c
		}
	}
	return d
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
