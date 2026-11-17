package treesitter

import (
	"math/rand"
	"regexp"
	"regexp/syntax"
	"strings"
	"sync"
	"testing"
)

func regexVMTestProgram(t *testing.T, pattern string) *syntax.Prog {
	t.Helper()
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		t.Fatal(err)
	}
	prog, err := syntax.Compile(re.Simplify())
	if err != nil {
		t.Fatal(err)
	}
	return prog
}

func TestQueryRegexVMMatchesRegexp(t *testing.T) {
	patterns := []string{
		``, `a`, `a|bc`, `(ab|a)*c`, `(?:a?)*`, `a{0,3}b+`, `[^a]+`, `.`, `(?s:.)`,
		`^a`, `a$`, `\Aa\z`, `(?m:^a$)`, `(?i:k)`, `[a-z한-힣]+`, `\p{M}*`,
		`(a*)(b?)`, `(?:a|aa)*b`, `(?:.*a){2}`, `(?U:a.*b)`, `(?:a|)b`,
	}
	random := rand.New(rand.NewSource(48))
	alphabet := []rune("abckK\n 한한é\u0301😀�")
	texts := []string{"", "a", "ab", "\n", "한글", "�", "a\nb\n", strings.Repeat("a", 100)}
	for range 500 {
		runes := make([]rune, random.Intn(30))
		for i := range runes {
			runes[i] = alphabet[random.Intn(len(alphabet))]
		}
		texts = append(texts, string(runes))
	}
	for _, pattern := range patterns {
		prog := regexVMTestProgram(t, pattern)
		re := regexp.MustCompile(pattern)
		for _, text := range texts {
			if got, want := regexVMMatch(prog, text, nil), re.MatchString(text); got != want {
				t.Fatalf("pattern %q, text %q: VM=%v regexp=%v", pattern, text, got, want)
			}
		}
	}
}

func TestQueryRegexVMUnicodeBoundariesAndInvalidUTF8(t *testing.T) {
	for _, test := range []struct {
		pattern, text string
		want          bool
	}{
		{`\b한글\b`, "한글", true},
		{`\b한\b`, "한", true},
		{`\b\p{M}\b`, "\u0301", true},
		{`한\B글`, "한글", true},
		{`\b😀\b`, "😀", false},
		{`\B😀\B`, "😀", true},
		{`^\B$`, "", true},
		{`\b`, "", false},
		{`\B\w`, "b\x801", true},
		{`\B\w`, "é\x801", true},
		{`\B\w`, "一\x801", true},
		{`\b\W`, "b\x80 ", true},
		{`\b\W`, "é\x80 ", true},
		{`\b[^a].`, "\"1\xbfÀ\"", false},
		{`\b(b.(?:[^\Wb][^a]?[a-c]+|b$)?)*\bé`, "\"\u200da\xa9é\xc3\"", false},
		{`\B(?i)[^\Wk][^\Wb]?(?s:.)`, "\" b\u200d\x801\"", true},
		{`b.`, "b\x80", false},
		{`é.`, "é\x80", false},
		{`\b한\b`, "\xff한\xff", true},
		{`\b`, "\xff", false},
		{`\B`, "\xff", false},
		{`^\B`, "\xff ", false},
		{`\B$`, " \xff", false},
		{`^\B`, "�", true},
		{`\B$`, "�", true},
		{`.`, "\xff", false},
		{`[^a]`, "\xff", false},
		{`\x{FFFD}`, "\xff", false},
		{`\x{FFFD}`, "�", true},
		{`a.b`, "a\xffb", false},
		{`a.*b`, "a\xffb", false},
		{`b`, "a\xffb", true},
		{`\bb\b`, "\xffb", true},
		{`\b한\b`, "\xe1\x80한", true},
		{`\B`, "\xe1\x80", false},
		{`^$`, "\xff", false},
		{``, "\xff", true},
	} {
		prog := regexVMTestProgram(t, test.pattern)
		if got := regexVMMatch(prog, test.text, nil); got != test.want {
			t.Errorf("pattern %q, text %q: got %v, want %v", test.pattern, test.text, got, test.want)
		}
	}
}

func TestQueryRegexVMASCIIAndMixedBoundaries(t *testing.T) {
	for _, test := range []struct {
		pattern, text string
		want          bool
	}{
		{`\b한\b`, "한", false},
		{`\B한\B`, "한", true},
		{`^\B`, "\xff", true},
		{`\B$`, "\xff", true},
		{`\ba\b`, "\xffa\xff", true},
	} {
		prog := regexVMTestProgram(t, test.pattern)
		ascii := make(map[uint32]bool)
		for pc, inst := range prog.Inst {
			if inst.Op == syntax.InstEmptyWidth {
				ascii[uint32(pc)] = true
			}
		}
		if got := regexVMMatch(prog, test.text, ascii); got != test.want {
			t.Errorf("ASCII pattern %q, text %q: got %v, want %v", test.pattern, test.text, got, test.want)
		}
	}
	prog := regexVMTestProgram(t, `\b한(?P<ascii>\B)`)
	ascii := make(map[uint32]bool)
	for _, inst := range prog.Inst {
		if inst.Op == syntax.InstCapture && inst.Arg == 2 {
			ascii[inst.Out] = true
		}
	}
	if !regexVMMatch(prog, "한", ascii) || regexVMMatch(prog, "한", nil) {
		t.Fatal("Unicode and ASCII assertions must retain their individual modes")
	}
}

func TestQueryRegexVMRepeatedStatesAndConcurrentUse(t *testing.T) {
	prog := regexVMTestProgram(t, `(?:a|aa)*b`)
	text := strings.Repeat("a", 20000)
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			if regexVMMatch(prog, text, nil) {
				t.Error("unexpected match")
			}
			if !regexVMMatch(prog, text+"b", nil) {
				t.Error("expected match after repeated states")
			}
		})
	}
	workers.Wait()
}

func TestQueryRegexVMASCIIInteriorBoundary(t *testing.T) {
	for _, test := range []struct {
		pattern, text string
		want          bool
	}{
		{`(?-u:\B)`, "a한b", true},
		{`(?-u:\B)`, "a😀b", true},
		{`\B`, "a한b", true},
		{`(?-u:\B)\B`, "a한b", false},
		{`(?-u:\B)한`, "a한b", false},
		{`(?-u:\B)한`, "한", true},
		{`a한b(?-u:\B)c`, "a한bc", true},
		{`(?-u:\B)^`, "a한b", false},
		{`(?-u:\B)$`, "a한b", false},
	} {
		re, err := compileQueryRegex(test.pattern)
		if err != nil {
			t.Fatal(err)
		}
		if got := re.MatchString(test.text); got != test.want {
			t.Errorf("pattern %q, text %q: got %v, want %v", test.pattern, test.text, got, test.want)
		}
	}
}
