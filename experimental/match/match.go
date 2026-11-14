// Package match pairs the nodes of two snapshots of one language across an edit. EXPERIMENTAL: it lives outside the
// stable API of package treesitter, has no compatibility promise, and may change or be removed. It is built only on the
// public treesitter API (Node.Hash, Symbol, FieldID, Parent, ChildCount) plus an internal language-identity check.
package match

import (
	"github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/internal/langid"
)

// Mapping pairs the nodes of two snapshots of one language across an edit. EXPERIMENTAL: the algorithm and its
// accuracy are still being worked on (T2 measured 100 % on large single-token edits, 77-97 % matched with 0.1-0.3 %
// wrong on small edits, and weak results when the edit produces ERROR recovery); it may change or be removed.
type Mapping struct {
	old, new *treesitter.Snapshot
	o2n, n2o []int32 // -1 = unmatched
	matched  int
}

// NewOf returns the node of the new snapshot matched with n, a node of the old snapshot.
func (m *Mapping) NewOf(n treesitter.Node) (treesitter.Node, bool) {
	if n.IsZero() || n.Snapshot() != m.old {
		return treesitter.Node{}, false
	}
	j := m.o2n[n.Index()]
	if j < 0 {
		return treesitter.Node{}, false
	}
	return m.new.Node(int(j)), true
}

// OldOf returns the node of the old snapshot matched with n, a node of the new snapshot.
func (m *Mapping) OldOf(n treesitter.Node) (treesitter.Node, bool) {
	if n.IsZero() || n.Snapshot() != m.new {
		return treesitter.Node{}, false
	}
	i := m.n2o[n.Index()]
	if i < 0 {
		return treesitter.Node{}, false
	}
	return m.old.Node(int(i)), true
}

// Len is the number of matched pairs.
func (m *Mapping) Len() int { return m.matched }

// view is one snapshot unpacked into the columns the algorithm reads, built from the public Node API.
type view struct {
	n      int
	sym    []treesitter.Symbol
	fld    []treesitter.FieldID
	parent []int32 // -1 for the root
	end    []int32 // exclusive end of the subtree in preorder
	kids   []int32 // child count
	hash   []uint64
}

func newView(s *treesitter.Snapshot) *view {
	n := s.Len()
	v := &view{n: n, sym: make([]treesitter.Symbol, n), fld: make([]treesitter.FieldID, n), parent: make([]int32, n),
		end: make([]int32, n), kids: make([]int32, n), hash: make([]uint64, n)}
	for nd := range s.All() {
		i := nd.Index()
		v.sym[i], v.fld[i], v.hash[i], v.kids[i] = nd.Symbol(), nd.FieldID(), nd.Hash(), int32(nd.ChildCount())
		v.parent[i], v.end[i] = -1, int32(i+1)
		if p, ok := nd.Parent(); ok {
			v.parent[i] = int32(p.Index())
		}
	}
	for i := n - 1; i > 0; i-- { // children have larger indexes: a reverse pass settles every subtree end
		if p := v.parent[i]; v.end[i] > v.end[p] {
			v.end[p] = v.end[i]
		}
	}
	return v
}

func (v *view) childList(i int32) []int32 {
	var out []int32
	if v.kids[i] == 0 {
		return out
	}
	for c := i + 1; c < v.end[i]; c = v.end[c] {
		out = append(out, c)
	}
	return out
}

// Match pairs nodes of old and new, the snapshots before and after an edit. Top-down it links identical subtrees
// (equal Hash) and containers with equal (symbol, field) using longest common subsequences; bottom-up an unmatched
// inner node of old takes the same-symbol ancestor in new that holds most of its matched descendants (dice >= 0.5).
// Snapshots of different languages produce an empty Mapping. Experimental, see Mapping.
func Match(old, new *treesitter.Snapshot) *Mapping {
	g := &matcher{o: newView(old), n: newView(new)}
	g.mo, g.mn = make([]int32, g.o.n), make([]int32, g.n.n)
	for i := range g.mo {
		g.mo[i] = -1
	}
	for i := range g.mn {
		g.mn[i] = -1
	}
	m := &Mapping{old: old, new: new, o2n: g.mo, n2o: g.mn}
	if !langid.Same(old.Language(), new.Language()) || g.o.sym[0] != g.n.sym[0] {
		return m
	}
	if g.o.hash[0] == g.n.hash[0] {
		g.subtree(0, 0)
	} else {
		g.link(0, 0)
		g.align(0, 0)
		g.bottomUp()
	}
	for _, j := range g.mo {
		if j >= 0 {
			m.matched++
		}
	}
	return m
}

type matcher struct {
	o, n   *view
	mo, mn []int32
}

func (g *matcher) link(a, b int32) { g.mo[a], g.mn[b] = b, a }

// subtree links two subtrees with equal hashes node by node (equal hashes mean equal shape; a symbol or size
// mismatch, i.e. a hash collision, stops the linking instead of pairing unrelated nodes).
func (g *matcher) subtree(a, b int32) {
	sz := g.o.end[a] - a
	if g.n.end[b]-b != sz {
		return
	}
	for k := int32(0); k < sz; k++ {
		if g.o.sym[a+k] != g.n.sym[b+k] {
			return
		}
		g.link(a+k, b+k)
	}
}

// lcs returns index pairs of a longest common subsequence of equal keys; it gives up (nil) above about 16M cells.
func lcs(x, y []uint64) [][2]int {
	if len(x) == 0 || len(y) == 0 || len(x)*len(y) > 1<<24 {
		return nil
	}
	w := len(y) + 1
	t := make([]int32, (len(x)+1)*w)
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				t[i*w+j] = t[(i+1)*w+j+1] + 1
			} else if a, b := t[(i+1)*w+j], t[i*w+j+1]; a >= b {
				t[i*w+j] = a
			} else {
				t[i*w+j] = b
			}
		}
	}
	var out [][2]int
	for i, j := 0, 0; i < len(x) && j < len(y); {
		switch {
		case x[i] == y[j]:
			out = append(out, [2]int{i, j})
			i++
			j++
		case t[(i+1)*w+j] >= t[i*w+j+1]:
			i++
		default:
			j++
		}
	}
	return out
}

// align matches the children of an already matched pair: identical subtrees first (common prefix and suffix, then an
// LCS on hashes), the rest by (symbol, field) with recursion into the paired containers.
func (g *matcher) align(a, b int32) {
	co, cn := g.o.childList(a), g.n.childList(b)
	lo, ln := 0, 0
	for lo < len(co) && ln < len(cn) && g.o.hash[co[lo]] == g.n.hash[cn[ln]] {
		g.subtree(co[lo], cn[ln])
		lo, ln = lo+1, ln+1
	}
	eo, en := len(co), len(cn)
	for eo > lo && en > ln && g.o.hash[co[eo-1]] == g.n.hash[cn[en-1]] {
		g.subtree(co[eo-1], cn[en-1])
		eo, en = eo-1, en-1
	}
	mo, mn := co[lo:eo], cn[ln:en]
	kx, ky := make([]uint64, len(mo)), make([]uint64, len(mn))
	for i, c := range mo {
		kx[i] = g.o.hash[c]
	}
	for i, c := range mn {
		ky[i] = g.n.hash[c]
	}
	for _, p := range lcs(kx, ky) {
		g.subtree(mo[p[0]], mn[p[1]])
	}
	var uo, un []int32
	for _, c := range mo {
		if g.mo[c] < 0 {
			uo = append(uo, c)
		}
	}
	for _, c := range mn {
		if g.mn[c] < 0 {
			un = append(un, c)
		}
	}
	key := func(v *view, c int32) uint64 { return uint64(v.sym[c])<<16 | uint64(v.fld[c]) }
	kx, ky = kx[:0], ky[:0]
	for _, c := range uo {
		kx = append(kx, key(g.o, c))
	}
	for _, c := range un {
		ky = append(ky, key(g.n, c))
	}
	for _, p := range lcs(kx, ky) {
		x, y := uo[p[0]], un[p[1]]
		g.link(x, y)
		g.align(x, y)
	}
}

func (g *matcher) bottomUp() {
	votes := map[int32]int32{}
	for i := int32(g.o.n - 1); i > 0; i-- {
		if g.mo[i] >= 0 || g.o.kids[i] == 0 {
			continue
		}
		clear(votes)
		zi := g.o.end[i] - i
		for d := i + 1; d < g.o.end[i]; d++ {
			if j := g.mo[d]; j >= 0 {
				for a := g.n.parent[j]; a >= 0; a = g.n.parent[a] {
					if g.mn[a] < 0 && g.n.sym[a] == g.o.sym[i] {
						votes[a]++
					}
				}
			}
		}
		best, bd := int32(-1), 0.0
		for a, v := range votes {
			za := g.n.end[a] - a
			if d := 2 * float64(v) / float64(zi+za-2); d > bd || (d == bd && a < best) {
				best, bd = a, d
			}
		}
		if best >= 0 && bd >= 0.5 {
			g.link(i, best)
			g.align(i, best)
		}
	}
}
