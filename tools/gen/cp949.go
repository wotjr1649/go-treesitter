package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/format"
	"io"
	"os"
	"strconv"
	"strings"
)

// The WHATWG Encoding Standard "EUC-KR" index (it is windows-949 / UHC, not IBM-949). Its bytes are committed under
// testdata/ and checked against this hash; the generated table in the root module is a pure function of them.
const (
	cp949IndexURL        = "https://github.com/whatwg/encoding/blob/be17ac5c49881336538acfc7b40dba3c8009144b/index-euc-kr.txt"
	cp949IndexSHA256     = "89af20dd867c84cefb710b1790229786cfef2bf11916361a210d81b90381e267"
	cp949IndexIdentifier = "1d97134cbf187263585bc8f593ca4196654ed4c7a673f5672eaad4f5d9fdc4ba"
	cp949IndexDate       = "2024-09-18"
	cp949Mappings        = 17048
	cp949TableLen        = 23750 // max pointer 23749 + 1; lead 0xFE and the user-defined lead 0xC9 are unmapped
)

// bsd3 is the BSD 3-Clause text the WHATWG licence applies to portions incorporated into source code. A test compares it
// with LICENSES/whatwg-encoding.txt.
const bsd3 = `Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

3. Neither the name of the copyright holder nor the names of its
   contributors may be used to endorse or promote products derived from
   this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.`

// parseEUCKRIndex reads the index file: header comments, then one "pointer<TAB>0xCODEPOINT<TAB>comment" row per mapping.
// It returns table[pointer] = code point, 0 for unmapped pointers. Every deviation from the pinned shape is an error.
func parseEUCKRIndex(raw []byte) (*[cp949TableLen]uint16, error) {
	sum := sha256.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); got != cp949IndexSHA256 {
		return nil, fmt.Errorf("index-euc-kr.txt sha256 %s, pinned %s", got, cp949IndexSHA256)
	}
	var table [cp949TableLen]uint16
	var ident, date string
	n, last := 0, -1
	for i, line := range strings.Split(string(raw), "\n") {
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "#"):
			if v, ok := strings.CutPrefix(line, "# Identifier: "); ok {
				ident = v
			}
			if v, ok := strings.CutPrefix(line, "# Date: "); ok {
				date = v
			}
			continue
		}
		f := strings.SplitN(strings.TrimLeft(line, " "), "\t", 3)
		if len(f) != 3 {
			return nil, fmt.Errorf("line %d: want pointer, code point, comment: %q", i+1, line)
		}
		ptr, err := strconv.Atoi(f[0])
		if err != nil || ptr < 0 || ptr >= cp949TableLen {
			return nil, fmt.Errorf("line %d: bad pointer %q", i+1, f[0])
		}
		hexcp, ok := strings.CutPrefix(f[1], "0x")
		cp, err := strconv.ParseUint(hexcp, 16, 32)
		if !ok || err != nil {
			return nil, fmt.Errorf("line %d: bad code point %q", i+1, f[1])
		}
		switch {
		case ptr <= last:
			return nil, fmt.Errorf("line %d: pointer %d not increasing", i+1, ptr)
		case cp == 0 || cp > 0xFFFF || (cp >= 0xD800 && cp <= 0xDFFF) || cp == 0xFFFD:
			return nil, fmt.Errorf("line %d: code point U+%04X is not a table value (0 means unmapped, FFFD is the error mark)", i+1, cp)
		}
		table[ptr], last = uint16(cp), ptr
		n++
	}
	if ident != cp949IndexIdentifier || date != cp949IndexDate {
		return nil, fmt.Errorf("index header Identifier %q Date %q, pinned %q %q", ident, date, cp949IndexIdentifier, cp949IndexDate)
	}
	if n != cp949Mappings || last != cp949TableLen-1 {
		return nil, fmt.Errorf("%d mappings, max pointer %d; pinned %d and %d", n, last, cp949Mappings, cp949TableLen-1)
	}
	seen := map[uint16]int{}
	for p, cp := range table {
		if cp == 0 {
			continue
		}
		if q, dup := seen[cp]; dup {
			return nil, fmt.Errorf("U+%04X mapped from pointers %d and %d: the encoder side would be ambiguous", cp, q, p)
		}
		seen[cp] = p
	}
	return &table, nil
}

// genCP949 renders the root module's cp949_table.go.
func genCP949(index []byte) ([]byte, error) {
	table, err := parseEUCKRIndex(index)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString("// Code generated by tools/gen cp949; DO NOT EDIT.\n//\n")
	fmt.Fprintf(&b, "// Source: WHATWG Encoding Standard, index-euc-kr.txt (the \"EUC-KR\" index: windows-949 / UHC).\n")
	fmt.Fprintf(&b, "// %s\n// file sha256 %s, Identifier %s, Date %s.\n", cp949IndexURL, cp949IndexSHA256, cp949IndexIdentifier, cp949IndexDate)
	b.WriteString("//\n// Copyright © WHATWG (Apple, Google, Mozilla, Microsoft).\n")
	b.WriteString("// The index is licensed under CC BY 4.0; to the extent portions of it are incorporated into source code,\n")
	b.WriteString("// such portions are licensed under the BSD 3-Clause License below (see also LICENSES/whatwg-encoding.txt).\n//\n")
	for _, l := range strings.Split(bsd3, "\n") {
		b.WriteString(strings.TrimRight("// "+l, " ") + "\n")
	}
	b.WriteString("\npackage treesitter\n\n")
	fmt.Fprintf(&b, "// cp949Table maps a double-byte pointer, (lead-0x81)*190 + (trail-0x41), to a BMP code point; 0 is unmapped.\n")
	fmt.Fprintf(&b, "// It is read only by decodeCP949, so a program that never calls NewSourceCP949 does not link it.\n")
	fmt.Fprintf(&b, "var cp949Table = [%d]uint16{\n", cp949TableLen)
	for i := 0; i < cp949TableLen; i += 16 {
		for j := i; j < min(i+16, cp949TableLen); j++ {
			fmt.Fprintf(&b, "0x%04X,", table[j])
			if j%16 != 15 && j != cp949TableLen-1 {
				b.WriteByte(' ')
			}
		}
		b.WriteByte('\n')
	}
	b.WriteString("}\n")
	return format.Source(b.Bytes())
}

// runCP949Command: `gen cp949 [index-file [out-file]]`; the defaults are the committed index and the root module's table.
func runCP949Command(args []string, _ io.Writer) error {
	in, out := "testdata/index-euc-kr.txt", "../../cp949_table.go"
	if len(args) > 2 {
		return fmt.Errorf("usage: gen cp949 [index-file [out-file]]")
	}
	if len(args) > 0 {
		in = args[0]
	}
	if len(args) > 1 {
		out = args[1]
	}
	raw, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	b, err := genCP949(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(out, b, 0o644)
}
