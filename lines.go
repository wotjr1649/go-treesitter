package treesitter

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// LineIndex maps byte offsets of a source to lines and columns in bytes, UTF-16 code units and runes, and back (Pos,
// Offset). Lines end at LF only (like tree-sitter rows). A byte that is not valid UTF-8 counts as one rune and one
// UTF-16 unit. It is immutable and safe for concurrent use.
type LineIndex struct {
	src          string
	starts       []uint32 // starts[k] = byte offset of line k; starts[0] = 0
	nonASCIIOnce sync.Once
	nonASCII     []uint64 // bit k set: line k contains a byte >= 0x80; only Pos needs this
}

// Position is a 0-based line and the columns of one offset.
type Position struct {
	Line                       int
	ByteCol, UTF16Col, RuneCol uint32
}

// NewLineIndex indexes src (at most 4 GiB; offsets are uint32).
func NewLineIndex(src string) *LineIndex {
	x := &LineIndex{src: src, starts: make([]uint32, 1, strings.Count(src, "\n")+1)}
	for off := 0; ; {
		j := strings.IndexByte(src[off:], '\n')
		if j < 0 {
			break
		}
		off += j + 1
		x.starts = append(x.starts, uint32(off))
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

// point needs only byte columns; Pos would also decode a non-ASCII line for UTF-16/rune columns.
func (x *LineIndex) point(off uint32) Point {
	off = min(off, uint32(len(x.src)))
	line := x.Line(off)
	return Point{uint32(line), off - x.starts[line]}
}

// ColumnKind says what a column counts. The names follow the Position fields.
type ColumnKind uint8

const (
	ByteColumn  ColumnKind = iota + 1 // bytes: tree-sitter's Point.Column; LSP positionEncoding "utf-8"
	UTF16Column                       // UTF-16 code units: LSP positionEncoding "utf-16", the LSP default
	RuneColumn                        // runes (Unicode code points): LSP positionEncoding "utf-32"
)

func (k ColumnKind) String() string {
	switch k {
	case ByteColumn:
		return "ByteColumn"
	case UTF16Column:
		return "UTF16Column"
	case RuneColumn:
		return "RuneColumn"
	}
	return "ColumnKind(" + strconv.Itoa(int(k)) + ")"
}

// indexNonASCII builds, once, the bit set of lines that hold a byte >= 0x80.
func (x *LineIndex) indexNonASCII() {
	x.nonASCIIOnce.Do(func() {
		x.nonASCII = make([]uint64, (len(x.starts)+63)/64)
		for k, start := range x.starts {
			end := len(x.src)
			if k+1 < len(x.starts) {
				end = int(x.starts[k+1])
			}
			for i := int(start); i < end; i++ {
				if x.src[i] >= 0x80 {
					x.nonASCII[k>>6] |= 1 << (k & 63)
					break
				}
			}
		}
	})
}

// Pos maps a byte offset to its line and columns. Offsets past the end are clamped to the end of the source. An offset
// inside a multi-byte character gives the columns of that character's start (so UTF16Col and RuneCol never decrease as
// the offset grows); ByteCol is always the offset from the start of the line, as in a tree-sitter Point.
func (x *LineIndex) Pos(off uint32) Position {
	x.indexNonASCII()
	if int64(off) > int64(len(x.src)) {
		off = uint32(len(x.src))
	}
	line := x.Line(off)
	p := Position{Line: line, ByteCol: off - x.starts[line]}
	if x.nonASCII[line>>6]>>(line&63)&1 == 0 {
		p.UTF16Col, p.RuneCol = p.ByteCol, p.ByteCol
		return p
	}
	rest, limit := x.src[x.starts[line]:], int(p.ByteCol)
	for i := 0; i < limit; {
		r, w := utf8.DecodeRuneInString(rest[i:])
		if i+w > limit {
			break // inside a character: it is not counted
		}
		i += w
		p.RuneCol++
		if r >= 0x10000 {
			p.UTF16Col += 2
		} else {
			p.UTF16Col++
		}
	}
	return p
}

// Offset is the inverse of Pos: the byte offset of column col, counted in kind units, on line (0-based). The bool is
// false, and the offset 0, when line is outside [0, NumLines()) or kind is not one of the three constants; a line is
// never clamped, so a wrong line is not silently moved. A column is clamped, as LSP specifies:
//   - a column past the line's content gives the end of the content, which is before the line's LF, and before the CR
//     of a CRLF. A CR that is not followed by LF is content (lines end at LF only, unlike LSP);
//   - a column inside a character, that is between the two UTF-16 units of a pair or inside the bytes of a multi-byte
//     character, gives the start of that character.
//
// So Offset(Pos(off)) is off for an off at a character boundary in a line's content, the start of the character for an
// off inside one, and the end of the content for an off inside the line terminator. Invalid UTF-8 bytes count one
// column unit each (as in Pos).
func (x *LineIndex) Offset(line int, col uint32, kind ColumnKind) (uint32, bool) {
	if line < 0 || line >= len(x.starts) || kind < ByteColumn || kind > RuneColumn {
		return 0, false
	}
	x.indexNonASCII()
	start, end := int(x.starts[line]), len(x.src)
	if line+1 < len(x.starts) {
		end = int(x.starts[line+1]) - 1 // the LF
		if end > start && x.src[end-1] == '\r' {
			end--
		}
	}
	ascii := x.nonASCII[line>>6]>>(line&63)&1 == 0
	if ascii || kind == ByteColumn {
		off := start + int(min(uint64(col), uint64(end-start)))
		if !ascii {
			off = x.floorRune(off, start)
		}
		return uint32(off), true
	}
	off, units := start, uint64(0)
	for off < end {
		r, w := utf8.DecodeRuneInString(x.src[off:end])
		add := uint64(1)
		if kind == UTF16Column && r >= 0x10000 {
			add = 2
		}
		if units+add > uint64(col) {
			break
		}
		off, units = off+w, units+add
	}
	return uint32(off), true
}

// floorRune moves off back to the start of the valid multi-byte character that contains it, if it is inside one.
func (x *LineIndex) floorRune(off, lineStart int) int {
	if off >= len(x.src) || utf8.RuneStart(x.src[off]) {
		return off
	}
	for j := 1; j < utf8.UTFMax && off-j >= lineStart; j++ {
		if utf8.RuneStart(x.src[off-j]) {
			if _, w := utf8.DecodeRuneInString(x.src[off-j:]); w > j {
				return off - j
			}
			break
		}
	}
	return off
}
