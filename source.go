package treesitter

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"unicode/utf16"
	"unicode/utf8"
)

// Encoding is the encoding a Source was decoded from.
type Encoding uint8

const (
	UTF8 Encoding = iota + 1 // no BOM or a UTF-8 BOM: the bytes are used as they are (the parser skips a BOM itself)
	UTF16LE
	UTF16BE
	CP949 // decoded by NewSourceCP949: the WHATWG "EUC-KR" index = windows-949 (UHC); not IBM-949 (Java Cp949, ICU cp949)
)

func (e Encoding) String() string {
	switch e {
	case UTF8:
		return "UTF-8"
	case UTF16LE:
		return "UTF-16LE"
	case UTF16BE:
		return "UTF-16BE"
	case CP949:
		return "CP949"
	}
	return fmt.Sprintf("Encoding(%d)", uint8(e))
}

// Source is text converted to UTF-8 for the parser, with a map from UTF-8 offsets back to the original bytes. It is
// immutable and safe for concurrent use. Standard library only.
type Source struct {
	orig     []byte
	utf8     []byte
	enc      Encoding
	bom      int      // length of the BOM in orig
	repl     int      // U+FFFD written for input that was not valid in the source encoding
	checks   []uint32 // converted encodings only: byte offsets in orig of the characters at the checkpoints
	checkOff []uint32 // UTF-8 offsets of those same characters; a checkpoint is added about every checkStep bytes of utf8
}

const checkStep = 1024

// NewSource detects the encoding by BOM: FF FE = UTF-16LE, FE FF = UTF-16BE (converted, BOM dropped); anything else,
// including EF BB BF, is taken as UTF-8 unchanged (the BOM stays in place so offsets match the original).
// Unmarked UTF-16 cannot be told from other text; use NewSourceUTF16 for it. CP949 is never detected; use
// NewSourceCP949. The converted bytes are a copy; the original is retained (not copied) for OriginalOffset and must
// not be modified.
func NewSource(raw []byte) (*Source, error) {
	switch {
	case len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xFE:
		return NewSourceUTF16(raw, false)
	case len(raw) >= 2 && raw[0] == 0xFE && raw[1] == 0xFF:
		return NewSourceUTF16(raw, true)
	}
	bom := 0
	if len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF {
		bom = 3
	}
	return &Source{orig: raw, utf8: raw, enc: UTF8, bom: bom}, nil
}

// NewSourceUTF16 converts UTF-16 (big-endian when bigEndian) to UTF-8. A leading BOM of the matching order is dropped.
// An unpaired surrogate becomes U+FFFD. An odd byte count is an error (ErrInvalidInput).
// The original bytes are retained for offset mapping and must not be modified.
func NewSourceUTF16(raw []byte, bigEndian bool) (*Source, error) {
	if len(raw)%2 != 0 {
		return nil, fmt.Errorf("%w: UTF-16 input has an odd number of bytes (%d)", ErrInvalidInput, len(raw))
	}
	order := binary.ByteOrder(binary.LittleEndian)
	enc := UTF16LE
	if bigEndian {
		order, enc = binary.BigEndian, UTF16BE
	}
	s := &Source{orig: raw, enc: enc}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = order.Uint16(raw[2*i:])
	}
	if len(units) > 0 && units[0] == 0xFEFF {
		units, s.bom = units[1:], 2
	}
	out := make([]byte, 0, len(units)+len(units)/2)
	var nextCheck int
	for i := 0; i < len(units); {
		if len(out) >= nextCheck {
			s.checkOff = append(s.checkOff, uint32(len(out)))
			s.checks = append(s.checks, uint32(s.bom+2*i))
			nextCheck = len(out) + checkStep
		}
		u := units[i]
		switch {
		case utf16.IsSurrogate(rune(u)) && i+1 < len(units) && utf16.IsSurrogate(rune(units[i+1])) &&
			u < 0xDC00 && units[i+1] >= 0xDC00:
			out = utf8.AppendRune(out, utf16.DecodeRune(rune(u), rune(units[i+1])))
			i += 2
		default: // BMP rune, or an unpaired surrogate (AppendRune writes U+FFFD for it)
			if utf16.IsSurrogate(rune(u)) {
				s.repl++
			}
			out = utf8.AppendRune(out, rune(u))
			i++
		}
	}
	s.utf8 = out
	return s, nil
}

// Bytes returns a read-only view of the UTF-8 text to pass to Parse. The caller must not modify its bytes.
func (s *Source) Bytes() []byte { return s.utf8 }

// Original returns a read-only view of the original input. The caller must not modify its bytes.
func (s *Source) Original() []byte { return s.orig }

// Encoding is the detected or given encoding.
func (s *Source) Encoding() Encoding { return s.enc }

// Replacements is the number of U+FFFD the conversion wrote for input that is not valid in the source encoding: an
// unpaired UTF-16 surrogate, or a CP949 byte or pair the WHATWG index does not map. It is 0 for UTF-8, which is never
// converted: its invalid bytes stay in Bytes() and are reported by HasInvalidUTF8 and rejected by WithStrictUTF8. A
// replacement is valid UTF-8, so neither of those sees one; check Replacements before trusting converted text, and
// never write converted text back over a file whose Replacements is not 0.
func (s *Source) Replacements() int { return s.repl }

// charWidths is the width in Bytes() and in Original of the character that starts at utf8[u] and orig[o].
func (s *Source) charWidths(u, o int) (w8, wo int) {
	r, w8 := utf8.DecodeRune(s.utf8[u:])
	if s.enc == CP949 {
		return w8, cp949Width(s.orig[o:], r)
	}
	if r >= 0x10000 {
		return w8, 4
	}
	return w8, 2
}

// seek walks from the last checkpoint at or before the target and returns the offsets (utf8, orig) of the start of the
// character that contains the target (or of the target itself, if it is a boundary). byUTF8 says which axis the target
// is on; it must lie inside the converted text.
func (s *Source) seek(target int, byUTF8 bool) (u, o int) {
	k := sort.Search(len(s.checkOff), func(i int) bool {
		if byUTF8 {
			return int(s.checkOff[i]) > target
		}
		return int(s.checks[i]) > target
	}) - 1
	u, o = int(s.checkOff[k]), int(s.checks[k])
	for u < len(s.utf8) {
		w8, wo := s.charWidths(u, o)
		if (byUTF8 && u+w8 > target) || (!byUTF8 && o+wo > target) {
			break // the target is this character's start or inside it: stay at its start
		}
		u, o = u+w8, o+wo
	}
	return u, o
}

// OriginalOffset maps an offset into Bytes to the byte offset of the same position in Original. In a converted
// source an offset inside a character maps to the start of that character's original bytes; an offset past the end
// maps to the end of Original; offset 0 of a UTF-16 source maps past its BOM. UTF-8 sources are not converted, so the offset is returned unchanged
// (clamped to the length). A CP949 character takes 1 or 2 original bytes, and a replacement U+FFFD (3 UTF-8 bytes)
// 1 or 2. Lookup costs about one checkpoint interval (1 KiB) of character decoding.
func (s *Source) OriginalOffset(off uint32) int {
	if s.enc == UTF8 {
		return min(int(off), len(s.orig))
	}
	if int64(off) >= int64(len(s.utf8)) {
		return len(s.orig)
	}
	_, o := s.seek(int(off), true)
	return o
}

// UTF8Offset maps a byte offset into Original to the offset of the same position in Bytes: the inverse of
// OriginalOffset. In a converted source an offset inside a character maps to the start of that character; an offset that is negative, or
// inside a UTF-16 BOM, maps to 0; an offset past the end maps to len(Bytes()). UTF-8 sources are not converted, so the
// offset is returned unchanged (clamped). Round trips hold at character boundaries only: for an offset o on a boundary
// of Original, OriginalOffset(UTF8Offset(o)) == o (a UTF-16 BOM excepted: both of its offsets give 2); for an offset u
// on a boundary of Bytes, UTF8Offset(OriginalOffset(u)) == u. For any other o in a converted source,
// OriginalOffset(UTF8Offset(o)) is the start of the character that contains o; in a UTF-8 source it is o (clamped).
//
// UTF8Offset is for positions in the original text, such as an editor selection. It is not meant for building an
// InputEdit: CP949 decoding is not local (see NewSourceCP949), so derive the edit from the old and new Bytes().
func (s *Source) UTF8Offset(orig int) uint32 {
	if s.enc == UTF8 {
		return uint32(max(0, min(orig, len(s.utf8))))
	}
	switch {
	case orig <= s.bom:
		return 0
	case orig >= len(s.orig):
		return uint32(len(s.utf8))
	}
	u, _ := s.seek(orig, false)
	return uint32(u)
}

// NewSourceCP949 converts CP949 text to UTF-8. CP949 here is the WHATWG Encoding Standard's "EUC-KR" index: windows-949
// (also called UHC), a superset of plain EUC-KR. It is not IBM-949, which Java calls "Cp949" and ICU "cp949". Bytes
// 0x00-0x7F are ASCII; a lead byte 0x81-0xFE and a trail byte 0x41-0xFE form one character if the pair is in the
// index (17,048 mappings). Anything else becomes U+FFFD, as the standard's decoder does it: a lead byte followed by an
// ASCII byte that completes no mapped pair is one replacement, and the ASCII byte is then read on its own, so a stray
// lead byte never swallows a newline, quote or backslash; a lone 0x80 or 0xFF, a lead byte at the end, and an unmapped
// pair with a non-ASCII trail byte (the user-defined areas with lead 0xC9 or 0xFE included) are replaced as a unit.
// Replacements counts them; refuse input where it is not 0 if the text will be written back.
//
// There is no detection and no BOM handling: input that starts with a UTF-8 BOM (EF BB BF) or a UTF-16 mark (FF FE,
// FE FF) is not taken as CP949 and returns ErrInvalidInput; use NewSource for it. Decoding depends on pair alignment, so
// it is not local: after any change to the original, decode the whole input again. The original is retained, not
// copied, and must not be modified.
func NewSourceCP949(raw []byte) (*Source, error) {
	for _, bom := range [][]byte{{0xEF, 0xBB, 0xBF}, {0xFF, 0xFE}, {0xFE, 0xFF}} {
		if bytes.HasPrefix(raw, bom) {
			return nil, fmt.Errorf("%w: CP949 input starts with %X, a byte order mark; use NewSource", ErrInvalidInput, bom)
		}
	}
	if int64(len(raw)) > math.MaxUint32/3 { // an input byte can become three UTF-8 bytes; offsets are uint32
		return nil, fmt.Errorf("%w: CP949 input of %d bytes is too large to map offsets", ErrInvalidInput, len(raw))
	}
	s := &Source{orig: raw, enc: CP949}
	out := make([]byte, 0, len(raw)+len(raw)/2)
	var nextCheck int
	for i := 0; i < len(raw); {
		if len(out) >= nextCheck {
			s.checkOff = append(s.checkOff, uint32(len(out)))
			s.checks = append(s.checks, uint32(i))
			nextCheck = len(out) + checkStep
		}
		if raw[i] < 0x80 {
			out = append(out, raw[i])
			i++
			continue
		}
		r, w := decodeCP949(raw[i:])
		if r == utf8.RuneError {
			s.repl++
		}
		out = utf8.AppendRune(out, r)
		i += w
	}
	s.utf8 = out
	return s, nil
}

// decodeCP949 decodes the character at the start of b (len(b) > 0) as the WHATWG EUC-KR decoder does and returns it with
// the number of bytes it consumed. A replacement is U+FFFD, which the table never holds.
func decodeCP949(b []byte) (rune, int) {
	lead := b[0]
	switch {
	case lead < 0x80:
		return rune(lead), 1
	case lead == 0x80 || lead == 0xFF || len(b) == 1:
		return utf8.RuneError, 1
	}
	if trail := b[1]; trail >= 0x41 && trail <= 0xFE {
		if p := int(lead-0x81)*190 + int(trail-0x41); p < len(cp949Table) && cp949Table[p] != 0 {
			return rune(cp949Table[p]), 2
		}
	}
	if b[1] < 0x80 {
		return utf8.RuneError, 1 // an ASCII byte is not consumed by the failed pair
	}
	return utf8.RuneError, 2
}

// cp949Width is the number of original bytes behind the character r that decodeCP949 produced from the start of b. It
// needs no table: a character other than U+FFFD is ASCII or a pair; a U+FFFD took one byte unless it was a pair with a
// non-ASCII trail byte. Keeping the table out of this path lets the linker drop it from programs that never decode CP949.
func cp949Width(b []byte, r rune) int {
	switch {
	case b[0] < 0x80:
		return 1
	case r != utf8.RuneError:
		return 2
	case b[0] == 0x80 || b[0] == 0xFF || len(b) == 1 || b[1] < 0x80:
		return 1
	}
	return 2
}
