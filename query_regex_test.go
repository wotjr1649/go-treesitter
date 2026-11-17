package treesitter

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

func TestQueryRegexTranslation(t *testing.T) {
	for _, tc := range []struct{ pattern, yes, no string }{
		{`^\w+$`, "한글한", "😀"}, {`^\d+$`, "１２３", "三"}, {`^\s$`, "\u3000", "a"},
		{`^[A-Z\d_]+$`, "ABC１２_", "한"}, {`^[^\W_]+$`, "한１", "_"}, {`^[a\W]+$`, "a😀", "b"},
		{`^\\w$`, `\w`, "한"}, {`^\_\%\!$`, "_%!", "abc"},
		{`^\u{D55C}\U0000AE00$`, "한글", "글한"}, {`^(?P<word>한|글)+$`, "한글", "a"},
		{`^(?i)[^\Wk]$`, "한", "K"}, {`^(?i)[^k]$`, "한", "K"},
		{`^(?i:k)$`, "K", "s"}, {`^(?i:ſ)$`, "S", "k"}, {`^(?i:ß)$`, "ẞ", "ss"},
		{`^(?i:Σ)$`, "ς", "s"}, {`^(?i:ǅ)$`, "Ǆ", "d"}, {`^(?i:Ꭰ)$`, "ꭰ", "a"},
		{`^(?i:k(?-i:k))$`, "Kk", "KK"}, {`(?m:^a$)`, "b\na\nb", "ba"},
		{`^(?s:.)$`, "\n", "aa"}, {`^(?-u:\w+\b)$`, "abc_1", "한글"},
		{`^(?-u:[^\W_])$`, "a", "_"}, {`(?-u:\b)한\b`, "a한", "한"},
		{`^\p{Hangul}+$`, "한글", "abc"}, {`^\P{L}$`, "😀", "한"},
		{`^\B$`, "", "\xff"}, {`\b한\b`, "\xff한\xff", "\xff"},
		{`^a{01}$`, "a", "a{01}"}, {`^a{001,002}$`, "aa", "a{001,002}"},
		{`^.$`, "�", "\xff"}, {`^[^a]$`, "�", "\xff"}, {`^\x{FFFD}$`, "�", "\xff"},
	} {
		t.Run(tc.pattern, func(t *testing.T) {
			re, err := compileQueryRegex(tc.pattern)
			if err != nil {
				t.Fatal(err)
			}
			if !re.MatchString(tc.yes) || re.MatchString(tc.no) {
				t.Errorf("yes=%q got %v; no=%q got %v", tc.yes, re.MatchString(tc.yes), tc.no, re.MatchString(tc.no))
			}
		})
	}
}

func TestQueryRegexRejectsSilentMisreads(t *testing.T) {
	for _, pattern := range []string{
		`\<`, `\>`, `\b{start}`, `\b{end-half}`, `[a&&b]`, `[a--b]`, `[a~~b]`, `[[a]]`, `[[:unknown:]]`, `[[:alpha]`,
		`(?x:a b)`, `(?R:.)`, `(?-u:.)`, `(?-u:\xFF)`, `(?-u:\W)`, `(?-u:[^a])`, `(?i-u:a)`,
		`\Qx\E`, `\101`, `\0`, `\q`, `\p{Alphabetic}`, `\p{Emoji}`, `\p{scx=Greek}`, `\p{age:1.1}`,
		`\p{gcb:Extend}`, `a{1001}`, `(?<a.b>x)`, `(?P<x>a)(?P<x>b)`, `(?ii:a)`, `(?i-i:a)`,
		`(?i-:a)`, `(?i-)a`, `a(?i)*`, `a(?i){2}`, `a*(?U)?`, `[\b]`, `\u{D800}`, `\x{110000}`, `(`, `[`, `a{,2}`, `a{2,1}`, `\`, string([]byte{255}),
	} {
		if _, err := compileQueryRegex(pattern); err == nil {
			t.Errorf("accepted %q", pattern)
		} else if !strings.Contains(err.Error(), "regex") {
			t.Errorf("unclear error for %q: %v", pattern, err)
		}
	}
}

func TestQueryRegexMalformedPOSIXClasses(t *testing.T) {
	for _, pattern := range []string{`[[:]`, `[[:]]`, `[[:]a]`, `[^[:]`, `[a[:]`, `[[:^]`, `[[:a]`, `[[::]]`} {
		t.Run(pattern, func(t *testing.T) {
			fl := newFakeLang(func(_ *fakeLang, m *fakeModule) {
				m.qs().meta = fakeQueryMeta([]string{"match?", pattern}, [][2]uint32{{2, 0}, {1, 0}, {2, 1}, {0, 0}})
			})
			q, err := NewQuery(fl.lang, "query")
			var qe *QueryError
			if q != nil || !errors.As(err, &qe) || qe.Type != QueryErrorPredicate {
				t.Fatalf("query=%v error=%v; want a predicate error", q, err)
			}
		})
	}
}

func FuzzQueryRegexClass(f *testing.F) {
	for _, pattern := range []string{`[[:]`, `[[:]]`, `[[:]a]`, `[^[:]`, `[a[:]`, `[[:^]`, `[[:a]`, `[[::]]`, `[[:alpha:]]`, `(?i)[[:^upper:]]`, `[a-z]`, `[^\W_]`, `[]`, `[^]`, "[\xff]"} {
		f.Add(pattern)
	}
	f.Fuzz(func(t *testing.T, pattern string) {
		if len(pattern) > 32 {
			t.Skip()
		}
		// Invalid patterns may return errors; panics fail the fuzz case.
		compileQueryRegex(pattern)
	})
}

func TestQueryRegexPOSIXClasses(t *testing.T) {
	for _, name := range []string{"alnum", "alpha", "ascii", "blank", "cntrl", "digit", "graph", "lower", "print", "punct", "space", "upper", "word", "xdigit"} {
		for _, negate := range []string{"", "^"} {
			pattern := "^[[:" + negate + name + ":]]$"
			got, err := compileQueryRegex(pattern)
			if err != nil {
				t.Fatal(err)
			}
			want := regexp.MustCompile(pattern)
			for _, r := range "\x00\t\n\v\f\r !/09:@AZ[_`az{~\x7fé한ſK😀" {
				if got.MatchString(string(r)) != want.MatchString(string(r)) {
					t.Errorf("%s: U+%04X", pattern, r)
				}
			}
		}
	}
	for _, tc := range []struct{ pattern, yes, no string }{
		{`^(?i)[[:upper:]]$`, "K", "한"},
		{`^(?i)[[:^upper:]]$`, "한", "K"},
		{`^(?i)[^[:lower:]]$`, "한", "ſ"},
		{`^(?-u:[[:alpha:]_])$`, "_", "é"},
		{`^(?-u:[^[:^digit:]])$`, "1", "a"},
	} {
		re, err := compileQueryRegex(tc.pattern)
		if err != nil || !re.MatchString(tc.yes) || re.MatchString(tc.no) {
			t.Errorf("%s: yes=%q no=%q err=%v", tc.pattern, tc.yes, tc.no, err)
		}
	}
}

func TestQueryRegexPropertyRanges(t *testing.T) {
	p := regexTranslator{}
	for _, tables := range []map[string]*unicode.RangeTable{unicode.Categories, unicode.Scripts} {
		for name, table := range tables {
			set, err := p.property(name)
			if err != nil {
				t.Fatal(err)
			}
			check := func(lo, hi rune) {
				for _, r := range []rune{lo, lo + 1, hi, hi + 1} {
					want := unicode.Is(table, r) && (r < 0xD800 || r > 0xDFFF)
					if regexContains(set, r) != want {
						t.Errorf("%s: U+%04X", name, r)
					}
				}
			}
			for _, r := range table.R16 {
				check(rune(r.Lo), rune(r.Hi))
			}
			for _, r := range table.R32 {
				check(rune(r.Lo), rune(r.Hi))
			}
		}
	}
}

func TestQueryRegexLiteralPrefix(t *testing.T) {
	for _, pattern := range []string{`zzz`, `\x{7A}[z]z`, `é한`, `a\.\[\]\(\)\{\}\*\+\?\^\$\\`} {
		re, err := compileQueryRegex(pattern)
		if err != nil {
			t.Fatal(err)
		}
		prefix, complete := re.re.LiteralPrefix()
		if !complete || !re.MatchString(prefix) || re.MatchString(strings.ToUpper(prefix)) {
			t.Errorf("%s: prefix=%q complete=%v", pattern, prefix, complete)
		}
	}
}

func TestQueryRegexCanonicalForm(t *testing.T) {
	p := regexTranslator{src: `(?i:é)[a\W]`}
	canonical, err := p.expression(regexFlags{unicode: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(canonical, "éaW") || !strings.Contains(canonical, `\x{`) || !strings.Contains(canonical, `(?-i:`) {
		t.Fatal(canonical)
	}
	for _, tc := range []struct {
		op, pattern string
		ok          bool
	}{
		{"match?", `\w`, true}, {"any-match?", `\w`, true}, {"any-not-match?", `\<`, false},
	} {
		q := &Query{source: " "}
		err := q.readMeta(fakeQueryMeta([]string{tc.op, tc.pattern}, [][2]uint32{{2, 0}, {1, 0}, {2, 1}, {0, 0}}))
		if (err == nil) != tc.ok {
			t.Errorf("%s %q: %v", tc.op, tc.pattern, err)
		}
		if err == nil && strings.HasPrefix(tc.op, "any-") && len(q.patterns[0].predicates) != 0 {
			t.Fatal("any-match must not filter")
		}
	}
}

// Paired legacy/current measurements keep compilation outside the timed loop.
func BenchmarkQueryRegex(b *testing.B) {
	for _, tc := range []struct{ name, pattern, text string }{
		{"ASCII", `^[a-zA-Z_][a-zA-Z0-9_]*$`, "query_capture_123"},
		{"UnicodeClass", `\w+`, strings.Repeat("한글 identifier ", 8)},
		{"Boundary", `\bregion\b`, strings.Repeat("#region section ", 8)},
	} {
		old := regexp.MustCompile(tc.pattern)
		current, err := compileQueryRegex(tc.pattern)
		if err != nil {
			b.Fatal(err)
		}
		for _, impl := range []struct {
			name  string
			match func(string) bool
		}{{"legacy", old.MatchString}, {"current", current.MatchString}} {
			b.Run(tc.name+"/"+impl.name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if !impl.match(tc.text) {
						b.Fatal("no match")
					}
				}
			})
		}
	}
}
