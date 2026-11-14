package treesitter

import (
	"encoding/binary"
	"math/bits"
	"unsafe"
)

const (
	hk1 = 0x9E3779B97F4A7C15
	hk2 = 0xC2B2AE3D27D4EB4F
	hk3 = 0x165667B19E3779F9
)

// hmix must not be (h^v)*k: that maps h==v to 0, a structural collision found by the T2 canonical-serialization check.
func hmix(h, v uint64) uint64 {
	h = (h + v*hk2 + hk3) * hk1
	h = bits.RotateLeft64(h, 29) * hk2
	return h ^ h>>32
}

// hashText is a deterministic 64-bit word-at-a-time hash (not cryptographic).
func hashText(s string) uint64 {
	b := unsafe.Slice(unsafe.StringData(s), len(s)) // read-only view
	h := uint64(len(b)) * hk3
	for len(b) >= 8 {
		h = hmix(h, binary.LittleEndian.Uint64(b))
		b = b[8:]
	}
	if len(b) > 0 {
		var t uint64
		for i, c := range b {
			t |= uint64(c) << (8 * i)
		}
		h = hmix(h, t^hk3)
	}
	return h
}

// merkle computes one hash per node bottom-up in a single reverse-preorder pass (children have larger indexes).
// shape=true skips extra children (comments and the like) and ignores the extra flag.
func (s *Snapshot) merkle(shape bool) []uint64 {
	h := make([]uint64, s.n)
	for i := int32(s.n - 1); i >= 0; i-- {
		f := s.flags[i] & (flagNamed | flagExtra | flagMissing | flagError)
		if shape {
			f &^= flagExtra
		}
		x := hmix(uint64(s.symbol[i])<<16|uint64(s.field[i]), uint64(f))
		if c, ok := s.firstChild(i); !ok {
			x = hmix(x, hashText(s.src[s.startByte[i]:s.endByte[i]]))
		} else {
			for ; ok; c, ok = s.nextSibling(c) {
				if shape && s.flags[c]&flagExtra != 0 {
					continue
				}
				x = hmix(x, h[c])
			}
		}
		h[i] = x
	}
	return h
}

// Hash is a Merkle hash of the subtree: symbol, field, named/extra/missing/error flags, the text of leaves and the
// hashes of the children in order. Equal subtrees (same language, same text) have equal hashes; unequal subtrees
// collide only with ordinary 64-bit probability, so treat equality as a strong hint and compare structure when a
// collision would matter. The hash array (8 bytes per node) is computed once, on first use of Hash.
func (n Node) Hash() uint64 {
	if n.s == nil {
		return 0
	}
	return n.s.hashes()[n.i]
}

// ShapeHash is Hash that ignores extra nodes (comments and other trivia) among the descendants and the extra flag, so
// a subtree keeps its ShapeHash when only comments change. Computed once, on first use of ShapeHash.
func (n Node) ShapeHash() uint64 {
	if n.s == nil {
		return 0
	}
	n.s.shapeOnce.Do(func() { n.s.shape = n.s.merkle(true) })
	return n.s.shape[n.i]
}

func (s *Snapshot) hashes() []uint64 {
	s.hashOnce.Do(func() { s.hash = s.merkle(false) })
	return s.hash
}
