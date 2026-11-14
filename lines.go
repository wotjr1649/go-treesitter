package treesitter

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// LineIndex maps byte offsets of a source to lines and columns in bytes, UTF-16 code units and runes. Lines end at LF
// only (like tree-sitter rows). A byte that is not valid UTF-8 counts as one rune and one UTF-16 unit. It is immutable
// and safe for concurrent use.
type LineIndex struct {
	src      string
	starts   []uint32 // starts[k] = byte offset of line k; starts[0] = 0
	nonASCII []uint64 // bit k set: line k contains a byte >= 0x80
}

// Position is a 0-based line and the columns of one offset.
type Position struct {
	Line                       int
	ByteCol, UTF16Col, RuneCol uint32
}

// NewLineIndex indexes src (at most 4 GiB; offsets are uint32).
func NewLineIndex(src string) *LineIndex {
	x := &LineIndex{src: src, starts: make([]uint32, 1, len(src)/32+2)}
	for off := 0; ; {
		j := strings.IndexByte(src[off:], '\n')
		if j < 0 {
			break
		}
		off += j + 1
		x.starts = append(x.starts, uint32(off))
	}
	x.nonASCII = make([]uint64, (len(x.starts)+63)/64)
	for k := range x.starts {
		a, e := int(x.starts[k]), len(src)
		if k+1 < len(x.starts) {
			e = int(x.starts[k+1])
		}
		for i := a; i < e; i++ {
			if src[i] >= 0x80 {
				x.nonASCII[k>>6] |= 1 << (k & 63)
				break
			}
		}
	}
	return x
}

// Lines is the snapshot's line index over its owned source, built once on first use.
func (s *Snapshot) Lines() *LineIndex {
	s.linesOnce.Do(func() { s.lines = NewLineIndex(s.src) })
	return s.lines
}

// NumLines is the number of lines (a final line without LF counts; an empty source has one line).
func (x *LineIndex) NumLines() int { return len(x.starts) }

// LineStart is the byte offset where line k starts; it panics when k is out of range.
func (x *LineIndex) LineStart(k int) uint32 { return x.starts[k] }

// Line is the 0-based line containing byte offset off (off may be len(source)).
func (x *LineIndex) Line(off uint32) int {
	return sort.Search(len(x.starts), func(i int) bool { return x.starts[i] > off }) - 1
}

// Pos maps a byte offset to its line and columns. Offsets past the end are clamped to the end of the source.
func (x *LineIndex) Pos(off uint32) Position {
	if int64(off) > int64(len(x.src)) {
		off = uint32(len(x.src))
	}
	line := x.Line(off)
	p := Position{Line: line, ByteCol: off - x.starts[line]}
	if x.nonASCII[line>>6]>>(line&63)&1 == 0 {
		p.UTF16Col, p.RuneCol = p.ByteCol, p.ByteCol
		return p
	}
	for t := x.src[x.starts[line]:off]; len(t) > 0; {
		r, w := utf8.DecodeRuneInString(t)
		t = t[w:]
		p.RuneCol++
		if r >= 0x10000 {
			p.UTF16Col += 2
		} else {
			p.UTF16Col++
		}
	}
	return p
}
