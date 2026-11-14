package treesitter

import "testing"

// FakeMatchParsers gives the external experimental/match tests three parsers on the fake engine: p, same (a second
// Language value wrapping p's grammar) and other (a different grammar with identical symbol ids).
func FakeMatchParsers(t *testing.T) (p, same, other *Parser) {
	t.Helper()
	fl := newFakeLang(nil)
	p = newP(t, fl)
	same, err := NewParser(NewLanguage(fl.lang.g))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { same.Close() })
	return p, same, newP(t, newFakeLang(nil))
}

// RandFakeSource is randSource for external tests.
var RandFakeSource = randSource
