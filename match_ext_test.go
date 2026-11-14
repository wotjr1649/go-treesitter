package treesitter_test

import (
	"context"
	"math/rand/v2"
	"testing"

	"github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/experimental/match"
)

func snapOf(t *testing.T, p *treesitter.Parser, src string) *treesitter.Snapshot {
	t.Helper()
	s, err := p.ParseSnapshot(context.Background(), []byte(src))
	if err != nil {
		t.Fatalf("ParseSnapshot(%q): %v", src, err)
	}
	return s
}

func TestMatch(t *testing.T) {
	p, same, other := treesitter.FakeMatchParsers(t)
	old, nw := snapOf(t, p, "(aa bb) cc"), snapOf(t, p, "(aa bb dd) cc")
	m := match.Match(old, nw)
	pair := func(i int) (int, bool) {
		n, ok := m.NewOf(old.Node(i))
		return n.Index(), ok
	}
	// old: 0 root, 1 group, 2 aa, 3 bb, 4 cc; new: 0 root, 1 group, 2 aa, 3 bb, 4 dd, 5 cc
	for i, want := range map[int]int{0: 0, 1: 1, 2: 2, 3: 3, 4: 5} {
		if got, ok := pair(i); !ok || got != want {
			t.Errorf("old %d -> %d,%v want %d", i, got, ok, want)
		}
	}
	if _, ok := m.OldOf(nw.Node(4)); ok || m.Len() != 5 {
		t.Errorf("inserted node must stay unmatched; Len %d", m.Len())
	}
	if o, ok := m.OldOf(nw.Node(5)); !ok || o.Index() != 4 {
		t.Error("OldOf")
	}
	if _, ok := m.NewOf(nw.Node(1)); ok { // a node of the wrong snapshot
		t.Error("NewOf must reject nodes of the new snapshot")
	}
	if _, ok := m.NewOf(treesitter.Node{}); ok { // the zero Node
		t.Error("NewOf must reject the zero Node")
	}
	if _, ok := m.OldOf(treesitter.Node{}); ok {
		t.Error("OldOf must reject the zero Node")
	}
	if s := match.Match(old, old); s.Len() != old.Len() {
		t.Errorf("identical snapshots: %d of %d", s.Len(), old.Len())
	}
	// A grammar module's Language() returns a new value per call: the same grammar must still match.
	if match.Match(old, snapOf(t, same, "(aa bb) cc")).Len() != old.Len() {
		t.Error("the same grammar behind two Language values must match")
	}
	// A different grammar never matches, even with identical symbol ids.
	if match.Match(old, snapOf(t, other, "(aa bb) cc")).Len() != 0 {
		t.Error("different grammars must give an empty Mapping")
	}
}

func TestMatchInvariantsOnRandomEdits(t *testing.T) {
	p, _, _ := treesitter.FakeMatchParsers(t)
	r := rand.New(rand.NewPCG(9, 10))
	pairs, wrongSym := 0, 0
	for k := 0; k < 120; k++ {
		src := treesitter.RandFakeSource(r, 5+r.IntN(40))
		cut := r.IntN(len(src) + 1)
		edited := src[:cut] + treesitter.RandFakeSource(r, 1+r.IntN(3)) + src[min(len(src), cut+r.IntN(4)):]
		old, nw := snapOf(t, p, src), snapOf(t, p, edited)
		m := match.Match(old, nw)
		seen := map[int]bool{}
		for n := range old.All() {
			j, ok := m.NewOf(n)
			if !ok {
				continue
			}
			pairs++
			if seen[j.Index()] {
				t.Fatalf("new node %d matched twice", j.Index())
			}
			seen[j.Index()] = true
			if j.Symbol() != n.Symbol() {
				wrongSym++
			}
			if back, ok := m.OldOf(j); !ok || back.Index() != n.Index() {
				t.Fatal("NewOf/OldOf must be inverse")
			}
		}
		if len(seen) != m.Len() {
			t.Fatalf("Len %d, counted %d", m.Len(), len(seen))
		}
	}
	if wrongSym != 0 || pairs == 0 {
		t.Fatalf("%d pairs, %d with different symbols", pairs, wrongSym)
	}
}
