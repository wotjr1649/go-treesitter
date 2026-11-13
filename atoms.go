package treesitter

// Atom identifies the text of a named leaf node within one Snapshot: equal atoms of one snapshot mean equal text.
// 0 means "no atom" (the node is not a named, non-empty leaf). Atoms of different snapshots are not comparable; use
// AtomText to compare across snapshots.
type Atom uint32

type atomTable struct {
	col []Atom   // per node
	off []uint32 // (start, len) pairs; entry 0 unused
}

func (s *Snapshot) buildAtoms() {
	t := atomTable{col: make([]Atom, s.n), off: make([]uint32, 2, 64)}
	seen := make(map[string]Atom) // transient: dropped after the build
	for i := 0; i < s.n; i++ {
		if s.flags[i]&flagNamed == 0 || s.childCount[i] != 0 || s.endByte[i] == s.startByte[i] {
			continue
		}
		a, e := s.startByte[i], s.endByte[i]
		text := s.src[a:e]
		id, ok := seen[text]
		if !ok {
			id = Atom(len(t.off) / 2)
			t.off = append(t.off, a, e-a)
			seen[text] = id
		}
		t.col[i] = id
	}
	s.atoms = t
}

// Atom is the interned text of a named leaf node (0 for every other node). The table is built once, on first use.
func (n Node) Atom() Atom {
	n.s.atomOnce.Do(n.s.buildAtoms)
	return n.s.atoms.col[n.i]
}

// AtomText is the text of atom a: a substring of the owned source, no allocation. It panics for 0 and for atoms that
// do not belong to this snapshot.
func (s *Snapshot) AtomText(a Atom) string {
	s.atomOnce.Do(s.buildAtoms)
	if a == 0 || int(a) >= len(s.atoms.off)/2 {
		panic("treesitter: atom does not belong to this snapshot")
	}
	o, l := s.atoms.off[2*a], s.atoms.off[2*a+1]
	return s.src[o : o+l]
}

// NumAtoms is the number of distinct atoms.
func (s *Snapshot) NumAtoms() int {
	s.atomOnce.Do(s.buildAtoms)
	return len(s.atoms.off)/2 - 1
}
