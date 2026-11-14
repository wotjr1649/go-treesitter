package treesitter

import "github.com/wotjr1649/go-treesitter/internal/langid"

// A grammar module's Language() may return a new value per call, so identity is the wrapped grammar, not the pointer.
func init() {
	langid.Same = func(a, b any) bool {
		x, ok1 := a.(*Language)
		y, ok2 := b.(*Language)
		return ok1 && ok2 && x.g == y.g
	}
}
