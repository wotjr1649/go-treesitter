package conformance

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"

	treesitter "github.com/wotjr1649/go-treesitter"
)

func FuzzSourceEncodings(f *testing.F) {
	f.Add([]byte(""), uint8(0), uint16(0))
	f.Add([]byte("\xef\xbb\xbf한\xff"), uint8(0), uint16(4))
	f.Add([]byte("\xff\xfe\x3d\xd8\x00\xde\x00\xd8\xfd\xff"), uint8(0), uint16(5))
	f.Add([]byte("\xfe\xff\xd8\x3d\xde\x00\xdc\x00\xff\xfd"), uint8(0), uint16(5))
	f.Add([]byte("\xff\xfe\x00"), uint8(0), uint16(0))
	f.Add([]byte("\x61\x00\x00\xd8"), uint8(1), uint16(3))
	f.Add([]byte("\x00\x61\xdc\x00"), uint8(2), uint16(3))
	f.Add(bytes.Repeat([]byte("\x00\xac"), 520), uint8(1), uint16(1030))
	f.Fuzz(func(t *testing.T, raw []byte, mode uint8, offset uint16) {
		if len(raw) > 4096 {
			t.Skip()
		}
		mode %= 3
		encoding := treesitter.UTF8
		switch {
		case mode == 1 || mode == 0 && bytes.HasPrefix(raw, []byte("\xff\xfe")):
			encoding = treesitter.UTF16LE
		case mode == 2 || mode == 0 && bytes.HasPrefix(raw, []byte("\xfe\xff")):
			encoding = treesitter.UTF16BE
		}
		var s *treesitter.Source
		var err error
		if mode == 0 {
			s, err = treesitter.NewSource(raw)
		} else {
			s, err = treesitter.NewSourceUTF16(raw, encoding == treesitter.UTF16BE)
		}
		if encoding != treesitter.UTF8 && len(raw)%2 != 0 {
			if s != nil || !errors.Is(err, treesitter.ErrInvalidInput) {
				t.Fatalf("odd UTF-16 length: %v", err)
			}
			return
		}
		if err != nil || s == nil || s.Encoding() != encoding || !bytes.Equal(s.Original(), raw) {
			t.Fatalf("invalid %v source: %v", encoding, err)
		}
		want, replacements, bom := string(raw), 0, 0
		if encoding != treesitter.UTF8 {
			var order binary.ByteOrder = binary.LittleEndian
			if encoding == treesitter.UTF16BE {
				order = binary.BigEndian
			}
			units := make([]uint16, len(raw)/2)
			for i := range units {
				units[i] = order.Uint16(raw[2*i:])
			}
			if len(units) != 0 && units[0] == 0xfeff {
				units, bom = units[1:], 2
			}
			want = string(utf16.Decode(units))
			replacements = strings.Count(want, "\ufffd")
			for _, u := range units {
				if u == 0xfffd {
					replacements--
				}
			}
		}
		if string(s.Bytes()) != want || s.Replacements() != replacements {
			t.Fatalf("%v decoded text or replacement count differs from reference", encoding)
		}

		u := int(offset) % (len(want) + 1)
		o := int(offset) % (len(raw) + 1)
		wantOrig, wantUTF8 := u, o
		if encoding != treesitter.UTF8 {
			wantOrig, wantUTF8 = len(raw), len(want)
			orig := bom
			for i, r := range want {
				width := 2
				if r > 0xffff {
					width = 4
				}
				if u >= i && u < i+utf8.RuneLen(r) {
					wantOrig = orig
				}
				if o >= orig && o < orig+width {
					wantUTF8 = i
				}
				orig += width
			}
			if o < bom {
				wantUTF8 = 0
			}
		}
		if got := s.OriginalOffset(uint32(u)); got != wantOrig {
			t.Fatalf("OriginalOffset(%d) = %d, want %d", u, got, wantOrig)
		}
		if got := s.UTF8Offset(o); got != uint32(wantUTF8) {
			t.Fatalf("UTF8Offset(%d) = %d, want %d", o, got, wantUTF8)
		}
		if s.UTF8Offset(-1) != 0 || s.UTF8Offset(len(raw)+1) != uint32(len(want)) ||
			s.OriginalOffset(uint32(len(want)+1)) != len(raw) {
			t.Fatal("source offsets did not clamp at the text boundaries")
		}
	})
}

func FuzzLineIndex(f *testing.F) {
	f.Add("", uint16(0))
	f.Add("a\r\n\nb\r", uint16(2))
	f.Add("한😀\n\xff\xc3", uint16(5))
	f.Add("a\xff\x80b", uint16(2))
	f.Add(strings.Repeat("\n", 65)+"😀", uint16(66))
	f.Fuzz(func(t *testing.T, src string, offset uint16) {
		if len(src) > 4096 {
			t.Skip()
		}
		off := uint32(offset) % uint32(len(src)+1)
		x := treesitter.NewLineIndex(src)
		point := fuzzBytePoint(src, off)
		start := int(off - point.Column)
		i := start
		want := treesitter.Position{Line: int(point.Row), ByteCol: point.Column}
		for i < int(off) {
			r, w := utf8.DecodeRuneInString(src[i:])
			if i+w > int(off) {
				break
			}
			i += w
			want.RuneCol++
			want.UTF16Col += uint32(utf16.RuneLen(r))
		}
		if got := x.Pos(off); got != want {
			t.Fatalf("Pos(%d) = %+v, want %+v", off, got, want)
		}
		if x.NumLines() != strings.Count(src, "\n")+1 || x.Line(off) != want.Line || x.LineStart(want.Line) != uint32(start) {
			t.Fatalf("line geometry disagrees at byte %d", off)
		}
		if x.Pos(math.MaxUint32) != x.Pos(uint32(len(src))) {
			t.Fatal("Pos did not clamp to EOF")
		}

		content := src[start:]
		if line, _, ok := strings.Cut(content, "\n"); ok {
			content = strings.TrimSuffix(line, "\r")
		}
		end := uint32(start + len(content))
		floor := min(uint32(i), end)
		for index, col := range []uint32{want.ByteCol, want.UTF16Col, want.RuneCol} {
			kind := treesitter.ByteColumn + treesitter.ColumnKind(index)
			if got, ok := x.Offset(want.Line, col, kind); !ok || got != floor {
				t.Fatalf("Offset(%d, %d, %v) = %d, %v, want %d", want.Line, col, kind, got, ok, floor)
			}
			if got, ok := x.Offset(want.Line, math.MaxUint32, kind); !ok || got != end {
				t.Fatalf("Offset did not clamp to the end of line %d for %v", want.Line, kind)
			}
		}
	})
}
