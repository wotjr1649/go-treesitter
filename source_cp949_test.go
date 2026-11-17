package treesitter

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"
)

// refChar is one decoded character: the code point, where its original bytes start and how many there are.
type refChar struct {
	r    rune
	o, w int
}

// refDecodeEUCKR is the decoder of the WHATWG Encoding Standard, "EUC-KR decoder", written step by step with its
// "lead" state and its "restore byte to ioQueue" rule, independently of decodeCP949's shape. lookup maps a pointer to a
// code point (0: none).
func refDecodeEUCKR(lookup func(ptr int) rune, b []byte) []refChar {
	var out []refChar
	lead, leadAt := 0, 0
	for i := 0; ; {
		if i == len(b) {
			if lead != 0 { // end of queue with a lead byte: error
				out = append(out, refChar{0xFFFD, leadAt, i - leadAt})
			}
			return out
		}
		c := int(b[i])
		switch {
		case lead != 0:
			l := lead
			lead = 0
			if c >= 0x41 && c <= 0xFE {
				if r := lookup((l-0x81)*190 + c - 0x41); r != 0 {
					out = append(out, refChar{r, leadAt, 2})
					i++
					continue
				}
			}
			if c < 0x80 { // an ASCII byte is restored to the queue and read again
				out = append(out, refChar{0xFFFD, leadAt, 1})
				continue
			}
			out = append(out, refChar{0xFFFD, leadAt, 2})
			i++
		case c < 0x80:
			out = append(out, refChar{rune(c), i, 1})
			i++
		case c >= 0x81 && c <= 0xFE:
			lead, leadAt = c, i
			i++
		default:
			out = append(out, refChar{0xFFFD, i, 1})
			i++
		}
	}
}

func tableLookup(ptr int) rune {
	if ptr < 0 || ptr >= len(cp949Table) {
		return 0
	}
	return rune(cp949Table[ptr])
}

// loadIndex reads the committed WHATWG index (the generator's input) with a separate small parser. It skips when the
// file is absent, which only happens outside a repository checkout.
func loadIndex(t testing.TB) map[int]rune {
	t.Helper()
	raw, err := os.ReadFile("tools/gen/testdata/index-euc-kr.txt")
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("tools/gen/testdata/index-euc-kr.txt is not in this tree")
	}
	if err != nil {
		t.Fatal(err)
	}
	idx := map[int]rune{}
	for _, line := range strings.Split(string(raw), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || strings.HasPrefix(line, "#") {
			continue
		}
		p, err1 := strconv.Atoi(f[0])
		cp, err2 := strconv.ParseUint(strings.TrimPrefix(f[1], "0x"), 16, 32)
		if err1 != nil || err2 != nil {
			t.Fatalf("bad index line %q", line)
		}
		idx[p] = rune(cp)
	}
	return idx
}

// TestCP949TableMatchesIndex: the generated table holds exactly the index's mappings (identity E3: both sides are the
// same pinned file; the generator test checks the file's hash).
func TestCP949TableMatchesIndex(t *testing.T) {
	idx := loadIndex(t)
	if len(idx) != 17048 {
		t.Fatalf("index has %d mappings", len(idx))
	}
	for p := range cp949Table {
		if got, want := tableLookup(p), idx[p]; got != want {
			t.Fatalf("pointer %d: table U+%04X, index U+%04X", p, got, want)
		}
	}
}

// TestCP949DecodeExhaustive: every 1-byte and 2-byte input, and every 3-byte input over the bytes that matter, decode
// like the reference decoder run on the index file: text, replacement count and the original width of each character.
func TestCP949DecodeExhaustive(t *testing.T) {
	idx := loadIndex(t)
	look := func(p int) rune { return idx[p] }
	check := func(raw []byte) {
		t.Helper()
		s, err := NewSourceCP949(raw)
		if err != nil {
			t.Fatalf("% X: %v", raw, err)
		}
		chars := refDecodeEUCKR(look, raw)
		var want []byte
		repl := 0
		for _, c := range chars {
			want = utf8.AppendRune(want, c.r)
			if c.r == 0xFFFD {
				repl++
			}
		}
		if !bytes.Equal(s.Bytes(), want) || s.Replacements() != repl {
			t.Fatalf("% X: got %q (%d repl), want %q (%d)", raw, s.Bytes(), s.Replacements(), want, repl)
		}
		u := 0
		for _, c := range chars { // character boundaries on both sides
			if s.OriginalOffset(uint32(u)) != c.o || s.UTF8Offset(c.o) != uint32(u) {
				t.Fatalf("% X char at %d: OriginalOffset(%d)=%d UTF8Offset(%d)=%d", raw, c.o, u, s.OriginalOffset(uint32(u)), c.o, s.UTF8Offset(c.o))
			}
			u += utf8.RuneLen(c.r)
		}
	}
	for a := 0; a < 256; a++ {
		check([]byte{byte(a)})
		for b := 0; b < 256; b++ {
			if a == 0xFF && b == 0xFE || a == 0xFE && b == 0xFF {
				continue // a BOM prefix is refused: TestNewSourceCP949
			}
			check([]byte{byte(a), byte(b)})
		}
	}
	alphabet := []byte{0x00, 0x0A, 0x0D, 0x22, 0x41, 0x5A, 0x5C, 0x61, 0x7F, 0x80, 0x81, 0xA1, 0xB0, 0xC6, 0xC9, 0xFD, 0xFE, 0xFF}
	for _, a := range alphabet {
		for _, b := range alphabet {
			for _, c := range alphabet {
				if (a == 0xFF && b == 0xFE) || (a == 0xFE && b == 0xFF) {
					continue
				}
				check([]byte{a, b, c})
			}
		}
	}
}

func TestNewSourceCP949(t *testing.T) {
	// "가나 한글" as CP949 bytes, with a stray lead byte before a newline and a quote.
	raw := []byte{0xB0, 0xA1, 0xB3, 0xAA, ' ', 0xC7, 0xD1, 0xB1, 0xDB, 0xB0, '\n', 'x', 0xB0, '"', 'y'}
	s, err := NewSourceCP949(raw)
	if err != nil || s.Encoding() != CP949 || s.Encoding().String() != "CP949" {
		t.Fatalf("%v %v", err, s.Encoding())
	}
	if want := "가나 한글�\nx�\"y"; string(s.Bytes()) != want {
		t.Fatalf("got %q want %q", s.Bytes(), want)
	}
	if s.Replacements() != 2 || &s.Original()[0] != &raw[0] {
		t.Fatalf("Replacements %d, original retained: %v", s.Replacements(), &s.Original()[0] == &raw[0])
	}
	// The newline after a lead byte stays a line break: the WHATWG rule re-reads an ASCII byte.
	if NewLineIndex(string(s.Bytes())).NumLines() != 2 {
		t.Fatal("the stray lead byte swallowed the newline")
	}
	for _, bom := range [][]byte{{0xEF, 0xBB, 0xBF, 'a'}, {0xFF, 0xFE, 'a', 0}, {0xFE, 0xFF, 0, 'a'}, {0xEF, 0xBB, 0xBF}} {
		if _, err := NewSourceCP949(bom); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("% X: %v", bom, err)
		}
	}
	if e, err := NewSourceCP949(nil); err != nil || len(e.Bytes()) != 0 || e.OriginalOffset(3) != 0 || e.UTF8Offset(3) != 0 || e.Replacements() != 0 {
		t.Fatalf("empty: %v", err)
	}
	// Decoded replacements are valid UTF-8: the strict-UTF-8 guard does not see them, Replacements does.
	if !utf8.Valid(s.Bytes()) {
		t.Fatal("decoded text must be valid UTF-8")
	}
	// EUDC lead bytes decode to U+FFFD whole.
	if e, _ := NewSourceCP949([]byte{0xC9, 0xA1, 0xFE, 0xA1, 'a'}); string(e.Bytes()) != "��a" || e.Replacements() != 2 {
		t.Fatalf("EUDC: %q", e.Bytes())
	}
}

// checkOffsetMap: for raw decoded by decodeFn, every offset on both sides maps as the reference boundaries say.
func checkCP949Offsets(t testing.TB, raw []byte) {
	t.Helper()
	s, err := NewSourceCP949(raw)
	if err != nil {
		if !bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) && !bytes.HasPrefix(raw, []byte{0xFF, 0xFE}) && !bytes.HasPrefix(raw, []byte{0xFE, 0xFF}) {
			t.Fatalf("% X: %v", raw, err)
		}
		return
	}
	chars := refDecodeEUCKR(tableLookup, raw)
	var want []byte
	utf8At := make([]int, 0, len(chars)+1) // utf8 start of each character
	for _, c := range chars {
		utf8At = append(utf8At, len(want))
		want = utf8.AppendRune(want, c.r)
	}
	if !bytes.Equal(s.Bytes(), want) {
		t.Fatalf("% X: got %q want %q", raw, s.Bytes(), want)
	}
	// UTF8Offset(o) for every original offset
	j := 0
	for o := 0; o <= len(raw); o++ {
		for j+1 < len(chars) && chars[j+1].o <= o {
			j++
		}
		wantU := len(want)
		if o < len(raw) {
			wantU = utf8At[j]
		}
		if got := s.UTF8Offset(o); got != uint32(wantU) {
			t.Fatalf("UTF8Offset(%d) = %d, want %d", o, got, wantU)
		}
	}
	// OriginalOffset(u) for every utf8 offset, plus past the end
	j = 0
	for u := 0; u <= len(want)+2; u++ {
		for j+1 < len(chars) && utf8At[j+1] <= u {
			j++
		}
		wantO := len(raw)
		if u < len(want) {
			wantO = chars[j].o
		}
		if got := s.OriginalOffset(uint32(u)); got != wantO {
			t.Fatalf("OriginalOffset(%d) = %d, want %d", u, got, wantO)
		}
	}
	if s.UTF8Offset(-5) != 0 || s.UTF8Offset(len(raw)+9) != uint32(len(want)) {
		t.Fatal("clamping")
	}
	// round trips at boundaries
	for i, c := range chars {
		if s.UTF8Offset(s.OriginalOffset(uint32(utf8At[i]))) != uint32(utf8At[i]) || s.OriginalOffset(s.UTF8Offset(c.o)) != c.o {
			t.Fatalf("round trip at character %d", i)
		}
	}
}

func cp949Seeds() [][]byte {
	return [][]byte{
		nil, {0xB0}, {0xFF}, {0x80}, {0xB0, 0xA1}, {0xB0, 'a'}, {0xB0, '\n'}, {0xB0, 0xFF}, {0xC9, 0xA1}, {0xFE, 0xFE},
		{'a', 0xB0, 0xA1, 0xB0, 0xB0, 0xA1, 0x80, 'z'}, {0xEF, 0xBB, 0xBF, 'a'}, {0xFF, 0xFE, 0}, {0xA4, 0xA1, 0xA4, 0xBF},
	}
}

func TestCP949OffsetMap(t *testing.T) {
	for _, raw := range cp949Seeds() {
		checkCP949Offsets(t, raw)
	}
	// Long random inputs cross many 1 KiB checkpoints; valid pairs, stray bytes and ASCII in proportion.
	r := rand.New(rand.NewPCG(7, 8))
	for k := 0; k < 12; k++ {
		raw := make([]byte, 3000+r.IntN(4000))
		for i := range raw {
			switch r.IntN(8) {
			case 0, 1, 2:
				raw[i] = byte(0xB0 + r.IntN(40)) // Hangul lead/trail range: mostly valid pairs
			case 3:
				raw[i] = byte(0x80 + r.IntN(128))
			case 4:
				raw[i] = byte(0x41 + r.IntN(26))
			default:
				raw[i] = byte(r.IntN(0x80))
			}
		}
		if raw[0] >= 0xEF { // not a BOM prefix
			raw[0] = 'a'
		}
		checkCP949Offsets(t, raw)
	}
}

func FuzzCP949Offsets(f *testing.F) {
	for _, s := range cp949Seeds() {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1<<14 {
			t.Skip()
		}
		checkCP949Offsets(t, raw)
	})
}

// TestSourceUTF16Offsets: UTF8Offset and Replacements for UTF-16 in both byte orders, with and without a BOM, unpaired
// surrogates and astral characters, across checkpoints.
func TestSourceUTF16Offsets(t *testing.T) {
	r := rand.New(rand.NewPCG(9, 10))
	pool := []uint16{'a', '\n', 0xAC00, 0xD7A4, 0xD83D, 0xDE00, 0xD800, 0xDC00, 0x20AC, 0xFFFD}
	for _, big := range []bool{false, true} {
		for _, bom := range []bool{false, true} {
			var units []uint16
			for i := 0; i < 2500; i++ {
				units = append(units, pool[r.IntN(len(pool))])
			}
			var raw []byte
			if bom {
				units = append([]uint16{0xFEFF}, units...)
			}
			for _, u := range units {
				if big {
					raw = append(raw, byte(u>>8), byte(u))
				} else {
					raw = append(raw, byte(u), byte(u>>8))
				}
			}
			s, err := NewSourceUTF16(raw, big)
			if err != nil {
				t.Fatal(err)
			}
			// reference: walk the units
			start := 0
			if bom {
				start = 1
			}
			var wantRepl int
			var want []byte
			type ch struct{ o, u int }
			var cs []ch
			for i := start; i < len(units); {
				cs = append(cs, ch{2 * i, len(want)})
				if utf16.IsSurrogate(rune(units[i])) && i+1 < len(units) && units[i] < 0xDC00 && units[i+1] >= 0xDC00 && units[i+1] < 0xE000 {
					want = utf8.AppendRune(want, utf16.DecodeRune(rune(units[i]), rune(units[i+1])))
					i += 2
					continue
				}
				if utf16.IsSurrogate(rune(units[i])) {
					wantRepl++
				}
				want = utf8.AppendRune(want, rune(units[i]))
				i++
			}
			if !bytes.Equal(s.Bytes(), want) || s.Replacements() != wantRepl {
				t.Fatalf("big=%v bom=%v: text equal %v, Replacements %d want %d", big, bom, bytes.Equal(s.Bytes(), want), s.Replacements(), wantRepl)
			}
			j := 0
			for o := 0; o <= len(raw)+1; o++ {
				for j+1 < len(cs) && cs[j+1].o <= o {
					j++
				}
				wantU := len(want)
				switch {
				case o < 2*start:
					wantU = 0
				case o < len(raw):
					wantU = cs[j].u
				}
				if got := s.UTF8Offset(o); got != uint32(wantU) {
					t.Fatalf("big=%v bom=%v UTF8Offset(%d) = %d, want %d", big, bom, o, got, wantU)
				}
			}
			for _, c := range cs { // round trips at boundaries
				if s.OriginalOffset(uint32(c.u)) != c.o || s.UTF8Offset(c.o) != uint32(c.u) {
					t.Fatalf("round trip at %+v", c)
				}
			}
		}
	}
}

func TestSourceUTF8OffsetIsIdentity(t *testing.T) {
	s, _ := NewSource([]byte("\xef\xbb\xbf가x"))
	for _, o := range []int{0, 1, 3, 4, 5, 6, 7, 99} {
		if got, want := s.UTF8Offset(o), uint32(min(o, 7)); got != want || s.OriginalOffset(uint32(o)) != min(o, 7) {
			t.Fatalf("UTF8Offset(%d) = %d", o, got)
		}
	}
	// inside a UTF-8 character both functions are clamped identity, not a floor; Pos keeps ByteCol as the byte distance
	if g, _ := NewSource([]byte("가")); g.UTF8Offset(1) != 1 || g.OriginalOffset(1) != 1 {
		t.Fatal("UTF-8 offset inside a character is not an identity")
	}
	if p := NewLineIndex("가").Pos(1); p.ByteCol != 1 || p.UTF16Col != 0 || p.RuneCol != 0 {
		t.Fatalf("Pos inside a UTF-8 character = %+v", p)
	}
	if s.UTF8Offset(-1) != 0 || s.Replacements() != 0 {
		t.Fatal("negative offset or Replacements of a UTF-8 source")
	}
	bad, _ := NewSource([]byte("a\xffb"))
	if bad.Replacements() != 0 {
		t.Fatal("UTF-8 input is not converted; HasInvalidUTF8 reports it")
	}
	lone, _ := NewSourceUTF16([]byte{0x61, 0, 0x00, 0xD8, 0x62, 0}, false)
	if lone.Replacements() != 1 {
		t.Fatalf("lone surrogate: %d", lone.Replacements())
	}
}
