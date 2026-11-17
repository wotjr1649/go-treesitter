package treesitter

import (
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf8"
)

// refOffset is Offset written from its documented rules over strings.Split: a line's content ends before its LF (and
// before the CR of a CRLF); characters are runes, an invalid byte is one rune; a column inside a character gives the
// character's start; a column past the content gives the end of the content.
func refOffset(src string, line int, col uint32, kind ColumnKind) (uint32, bool) {
	lines := strings.Split(src, "\n")
	if line < 0 || line >= len(lines) || kind < ByteColumn || kind > RuneColumn {
		return 0, false
	}
	start := 0
	for _, l := range lines[:line] {
		start += len(l) + 1
	}
	content := lines[line]
	if line < len(lines)-1 {
		content = strings.TrimSuffix(content, "\r")
	}
	off, units := 0, uint32(0)
	for off < len(content) {
		r, w := utf8.DecodeRuneInString(content[off:])
		u := uint32(1)
		switch {
		case kind == ByteColumn:
			u = uint32(w)
		case kind == UTF16Column && r >= 0x10000:
			u = 2
		}
		if units+u > col {
			break
		}
		off, units = off+w, units+u
	}
	return uint32(start + off), true
}

func colOf(p Position, kind ColumnKind) uint32 {
	switch kind {
	case ByteColumn:
		return p.ByteCol
	case UTF16Column:
		return p.UTF16Col
	}
	return p.RuneCol
}

const utf8BOM = "\xef\xbb\xbf"

var lineSamples = []string{
	"",
	"가x😀y",
	"한글\n두 번째 줄\n",
	"한글 abc\n한",                      // NFD jamo: one syllable is three runes
	"a😀b😀😀c\nz",                            // astral characters: two UTF-16 units each
	"ab\r\ncd\r\n\r\nef\r",                 // CRLF, an empty CRLF line, a final lone CR
	"a\rb\r\r\nc",                          // lone CR is content
	"x\xff가\xe2\x82y\xc3\n\xed\xa0\x80z\n", // invalid bytes, a truncated sequence, an encoded surrogate
	utf8BOM + "BOM 한글\n두번째",                // a UTF-8 BOM stays in the text
	"\n\n\n",
	"\xea\xb0", // the first two bytes of 가, at the end
}

func TestLineIndexOffsetRoundTrip(t *testing.T) {
	for _, src := range lineSamples {
		x := NewLineIndex(src)
		for off := 0; off <= len(src); off++ {
			p := x.Pos(uint32(off))
			for _, kind := range []ColumnKind{ByteColumn, UTF16Column, RuneColumn} {
				got, ok := x.Offset(p.Line, colOf(p, kind), kind)
				if !ok {
					t.Fatalf("%q off %d %v: not ok", src, off, kind)
				}
				want, _ := refOffset(src, p.Line, colOf(p, kind), kind)
				if got != want {
					t.Fatalf("%q off %d %v: Offset %d, reference %d", src, off, kind, got, want)
				}
				// Offset(Pos(off)) is off at a character boundary inside the line content; it never moves forward
				// past off, and only the line terminator or the inside of a character moves it back.
				if got > uint32(off) {
					t.Fatalf("%q off %d %v: Offset %d is past the offset", src, off, kind, got)
				}
				if utf8.RuneStart(srcByte(src, off)) && !inTerminator(src, off) && got != uint32(off) {
					t.Fatalf("%q off %d %v: boundary not round-tripped: %d", src, off, kind, got)
				}
			}
		}
	}
}

// srcByte is the byte at off, or a start byte at the end.
func srcByte(src string, off int) byte {
	if off >= len(src) {
		return 'a'
	}
	return src[off]
}

// inTerminator: off is at an LF, or at the CR of a CRLF, or between them: Offset clamps those to the content end. The
// first of them (the CR of a CRLF, or the LF of an LF-only line) is the content end itself and round-trips.
func inTerminator(src string, off int) bool {
	return off < len(src) && off > 0 && src[off] == '\n' && src[off-1] == '\r'
}

func TestLineIndexOffsetRandom(t *testing.T) {
	r := rand.New(rand.NewPCG(11, 12))
	pool := []string{"a", "bc", "가", "한", "ᄒ", "ᅡ", "😀", "\r\n", "\n", "\r", "\xff", "\xe2\x82", " "}
	for k := 0; k < 300; k++ {
		var b strings.Builder
		for i, n := 0, 1+r.IntN(40); i < n; i++ {
			b.WriteString(pool[r.IntN(len(pool))])
		}
		src := b.String()
		x := NewLineIndex(src)
		for line := -1; line <= x.NumLines(); line++ {
			for col := uint32(0); col < 30; col++ {
				for _, kind := range []ColumnKind{ByteColumn, UTF16Column, RuneColumn, 0, 4} {
					got, gotOK := x.Offset(line, col, kind)
					want, wantOK := refOffset(src, line, col, kind)
					if got != want || gotOK != wantOK {
						t.Fatalf("%q Offset(%d,%d,%v) = %d,%v; reference %d,%v", src, line, col, kind, got, gotOK, want, wantOK)
					}
				}
			}
		}
	}
}

// TestLineIndexPosInsideCharacter is the regression test for the monotonic Pos: an offset inside a character used to
// count each of its bytes as a rune, so the columns went down again at the next boundary.
func TestLineIndexPosInsideCharacter(t *testing.T) {
	src := "가x😀y"
	x := NewLineIndex(src)
	want := []Position{
		{0, 0, 0, 0}, {0, 1, 0, 0}, {0, 2, 0, 0}, // 가
		{0, 3, 1, 1},                                           // x
		{0, 4, 2, 2}, {0, 5, 2, 2}, {0, 6, 2, 2}, {0, 7, 2, 2}, // 😀 (two UTF-16 units)
		{0, 8, 4, 3}, // y
		{0, 9, 5, 4}, // end
	}
	if len(src) != len(want)-1 {
		t.Fatal("test table")
	}
	var prev Position
	for off, w := range want {
		got := x.Pos(uint32(off))
		if got != w {
			t.Fatalf("Pos(%d) = %+v, want %+v", off, got, w)
		}
		if got.UTF16Col < prev.UTF16Col || got.RuneCol < prev.RuneCol || got.ByteCol < prev.ByteCol {
			t.Fatalf("Pos(%d) = %+v went down from %+v", off, got, prev)
		}
		prev = got
	}
}

func TestLineIndexOffsetClamps(t *testing.T) {
	x := NewLineIndex("ab\r\n가나\n\xffz\r")
	cases := []struct {
		line int
		col  uint32
		kind ColumnKind
		want uint32
		ok   bool
	}{
		{0, 2, UTF16Column, 2, true}, // end of content, before the CR
		{0, 99, UTF16Column, 2, true},
		{0, 99, ByteColumn, 2, true},
		{1, 1, ByteColumn, 4, true},    // inside 가: its start
		{1, 2, ByteColumn, 4, true},    // inside 가: its start
		{1, 3, ByteColumn, 7, true},    // after 가
		{1, 99, RuneColumn, 10, true},  // past the content: before the LF
		{2, 1, UTF16Column, 12, true},  // the invalid byte is one unit
		{2, 99, UTF16Column, 14, true}, // the final CR has no LF after it: content
		{3, 0, UTF16Column, 0, false},  // NumLines is 3
		{-1, 0, UTF16Column, 0, false},
		{0, 0, 0, 0, false},
	}
	for _, c := range cases {
		if got, ok := x.Offset(c.line, c.col, c.kind); got != c.want || ok != c.ok {
			t.Errorf("Offset(%d,%d,%v) = %d,%v want %d,%v", c.line, c.col, c.kind, got, ok, c.want, c.ok)
		}
	}
	// a column between the two UTF-16 units of a pair gives the start of the pair
	y := NewLineIndex("a😀b")
	if got, _ := y.Offset(0, 2, UTF16Column); got != 1 {
		t.Fatalf("inside a surrogate pair: %d", got)
	}
	if got, _ := y.Offset(0, 3, UTF16Column); got != 5 {
		t.Fatalf("after the pair: %d", got)
	}
}

// TestLineIndexUTF8BOM documents the BOM contract: a UTF-8 BOM is text of the source, so on line 0 the columns include it.
// LSP clients that do not count the BOM are one UTF-16 unit (three bytes) lower there.
func TestLineIndexUTF8BOM(t *testing.T) {
	x := NewLineIndex(utf8BOM + "a\nb")
	if got := x.Pos(3); got != (Position{0, 3, 1, 1}) {
		t.Fatalf("Pos(3) = %+v", got)
	}
	if got := x.Pos(5); got != (Position{1, 0, 0, 0}) {
		t.Fatalf("Pos(5) = %+v", got)
	}
	if got, _ := x.Offset(0, 1, UTF16Column); got != 3 {
		t.Fatalf("Offset = %d", got)
	}
}
