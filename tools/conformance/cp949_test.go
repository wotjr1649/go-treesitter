package conformance

import (
	"path/filepath"
	"unicode/utf8"

	"testing"

	treesitter "github.com/wotjr1649/go-treesitter"
)

// cp949Encoder is the inverse of NewSourceCP949 for the mapped pairs, built from the public decoder alone: decoding every
// possible pair yields each character's CP949 bytes (the table has no duplicate code points).
func cp949Encoder(t *testing.T) map[rune][2]byte {
	t.Helper()
	enc := map[rune][2]byte{}
	for lead := 0x81; lead <= 0xFE; lead++ {
		for trail := 0x41; trail <= 0xFE; trail++ {
			s, err := treesitter.NewSourceCP949([]byte{byte(lead), byte(trail)})
			if err != nil {
				t.Fatal(err)
			}
			if s.Replacements() != 0 {
				continue
			}
			r, w := utf8.DecodeRune(s.Bytes())
			if w != len(s.Bytes()) {
				t.Fatalf("pair %02X %02X decoded to %d runes", lead, trail, utf8.RuneCount(s.Bytes()))
			}
			if _, dup := enc[r]; dup {
				t.Fatalf("U+%04X decoded from two pairs", r)
			}
			enc[r] = [2]byte{byte(lead), byte(trail)}
		}
	}
	return enc
}

// encodeCP949 returns the CP949 bytes of src and, for every byte offset of src, the offset of the same position in the
// result (valid for offsets at rune starts). ok is false if src has a rune CP949 cannot hold.
func encodeCP949(enc map[rune][2]byte, src []byte) (raw []byte, at []int, ok bool) {
	at = make([]int, len(src)+1)
	for i := 0; i < len(src); {
		r, w := utf8.DecodeRune(src[i:])
		if r == utf8.RuneError && w == 1 {
			return nil, nil, false
		}
		for j := 0; j < w; j++ {
			at[i+j] = len(raw)
		}
		switch pair, mapped := enc[r]; {
		case r < 0x80:
			raw = append(raw, byte(r))
		case mapped:
			raw = append(raw, pair[0], pair[1])
		default:
			return nil, nil, false
		}
		i += w
	}
	at[len(src)] = len(raw)
	return raw, at, true
}

// TestCorpusCP949RealEngine: corpus inputs that CP949 can hold are encoded, decoded by NewSourceCP949 and parsed. The
// decoded bytes equal the UTF-8 original, so the tree is the original's: for these inputs the dump equals the recorded
// native C dump (E5 on those inputs only). Korean text inserted into the first inputs of each language has no native dump;
// its trees are compared with the parse of the UTF-8 text (E3, self-consistency). In both groups OriginalOffset of every
// 7th node start and end equals the CP949 offset an independent encoder gives, and UTF8Offset maps it back.
func TestCorpusCP949RealEngine(t *testing.T) {
	root := dataRoot(t)
	rows := readCSV(t, filepath.Join(root, "corpus", "manifest.csv"))
	enc := cp949Encoder(t)
	if len(enc) != 17048 {
		t.Fatalf("%d encodable characters, want the 17048 of the WHATWG index", len(enc))
	}
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			p := newParser(t, lang)
			var corpusFiles, withKorean, synthetic, offsets, notEncodable int
			check := func(name string, src []byte, wantDump []byte) {
				t.Helper()
				raw, at, ok := encodeCP949(enc, src)
				if !ok {
					t.Fatalf("%s: not encodable", name)
				}
				s, err := treesitter.NewSourceCP949(raw)
				if err != nil || s.Replacements() != 0 || string(s.Bytes()) != string(src) {
					t.Fatalf("%s: NewSourceCP949: %v, %d replacements, text equal %v", name, err, s.Replacements(), err == nil && string(s.Bytes()) == string(src))
				}
				snap, err := p.ParseSnapshot(bg, s.Bytes())
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				if wantDump == nil {
					ref, err := p.ParseSnapshot(bg, src)
					if err != nil {
						t.Fatalf("%s: %v", name, err)
					}
					wantDump = Dump(ref)
				}
				if string(Dump(snap)) != string(wantDump) {
					t.Fatalf("%s: dump of the decoded source differs: %s", name, firstDiff(Dump(snap), wantDump))
				}
				for node := range snap.All() {
					if node.Index()%7 != 0 {
						continue
					}
					for _, off := range []uint32{node.StartByte(), node.EndByte()} {
						if got, want := s.OriginalOffset(off), at[off]; got != want {
							t.Fatalf("%s node %d offset %d: OriginalOffset %d, encoder %d", name, node.Index(), off, got, want)
						}
						if got := s.UTF8Offset(at[off]); got != off {
							t.Fatalf("%s node %d: UTF8Offset(%d) = %d, want %d", name, node.Index(), at[off], got, off)
						}
						offsets++
					}
				}
			}
			for _, r := range rows {
				if r[1] != lang {
					continue
				}
				name := filepath.Base(r[0])
				src := readFile(t, corpusFile(t, root, lang, name))
				if !utf8.Valid(src) || (len(src) >= 3 && src[0] == 0xEF && src[1] == 0xBB && src[2] == 0xBF) {
					continue
				}
				if _, _, ok := encodeCP949(enc, src); !ok {
					notEncodable++
					continue
				}
				korean := len(src) != utf8.RuneCount(src)
				if !korean && corpusFiles-withKorean >= 4 { // a few ASCII-only inputs; every input that holds non-ASCII text
					continue
				}
				corpusFiles++
				if korean {
					withKorean++
				}
				check(name, src, readFile(t, filepath.Join(root, "native", "dumps", lang, name+".dump")))
			}
			// Korean text inserted into the first inputs: after the first line break and at the end.
			for _, r := range rows {
				if r[1] != lang || synthetic >= 12 {
					continue
				}
				name := filepath.Base(r[0])
				src := readFile(t, corpusFile(t, root, lang, name))
				if !utf8.Valid(src) || (len(src) >= 3 && src[0] == 0xEF && src[1] == 0xBB && src[2] == 0xBF) {
					continue
				}
				if _, _, ok := encodeCP949(enc, src); !ok {
					continue
				}
				const ins = "\n가나다 한글 漢字 ※ ㉠\n"
				nl := 0
				for nl < len(src) && src[nl] != '\n' {
					nl++
				}
				check(name+"+korean-end", append(append([]byte{}, src...), ins...), nil)
				check(name+"+korean-mid", append(append(append([]byte{}, src[:nl]...), ins...), src[nl:]...), nil)
				synthetic += 2
			}
			summary(t, map[string]any{"lang": lang, "corpusInputs": corpusFiles, "withNonASCII": withKorean, "syntheticKorean": synthetic,
				"notEncodable": notEncodable, "offsetsChecked": offsets})
		})
	}
}
