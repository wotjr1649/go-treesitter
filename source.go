package treesitter

import (
	"encoding/binary"
	"fmt"
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
)

func (e Encoding) String() string {
	switch e {
	case UTF8:
		return "UTF-8"
	case UTF16LE:
		return "UTF-16LE"
	case UTF16BE:
		return "UTF-16BE"
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
	checks   []uint32 // UTF-16 only: units before utf8 offset checkStep*k (k-th entry, at a rune boundary)
	checkOff []uint32 // UTF-8 offsets of those checkpoints
}

const checkStep = 1024

// NewSource detects the encoding by BOM: FF FE = UTF-16LE, FE FF = UTF-16BE (converted, BOM dropped); anything else,
// including EF BB BF, is taken as UTF-8 unchanged (the BOM stays in place so offsets match the original).
// Unmarked UTF-16 cannot be told from other text; use NewSourceUTF16 for it. The converted bytes are a copy; the
// original is retained (not copied) for OriginalOffset and must not be modified.
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
			s.checks = append(s.checks, uint32(i))
			nextCheck = len(out) + checkStep
		}
		u := units[i]
		switch {
		case utf16.IsSurrogate(rune(u)) && i+1 < len(units) && utf16.IsSurrogate(rune(units[i+1])) &&
			u < 0xDC00 && units[i+1] >= 0xDC00:
			out = utf8.AppendRune(out, utf16.DecodeRune(rune(u), rune(units[i+1])))
			i += 2
		default: // BMP rune, or an unpaired surrogate (AppendRune writes U+FFFD for it)
			out = utf8.AppendRune(out, rune(u))
			i++
		}
	}
	s.utf8 = out
	return s, nil
}

// Bytes is the UTF-8 text to pass to Parse. Do not modify it.
func (s *Source) Bytes() []byte { return s.utf8 }

// Original is the input as given.
func (s *Source) Original() []byte { return s.orig }

// Encoding is the detected or given encoding.
func (s *Source) Encoding() Encoding { return s.enc }

// OriginalOffset maps an offset into Bytes to the byte offset of the same position in Original. Offsets inside a
// multi-byte rune map to the start of that rune's code units; offsets past the end map to the end of Original.
// Lookup costs about one checkpoint interval (1 KiB) of rune decoding.
func (s *Source) OriginalOffset(off uint32) int {
	if s.enc == UTF8 {
		return min(int(off), len(s.orig))
	}
	if int64(off) >= int64(len(s.utf8)) {
		return len(s.orig)
	}
	k := sort.Search(len(s.checkOff), func(i int) bool { return s.checkOff[i] > off }) - 1
	pos, units := int(s.checkOff[k]), int(s.checks[k])
	for pos < int(off) {
		r, w := utf8.DecodeRune(s.utf8[pos:])
		if pos+w > int(off) {
			break // inside a rune: stay at its start
		}
		pos += w
		units++
		if r >= 0x10000 {
			units++
		}
	}
	return s.bom + 2*units
}
