// Package treesitter provides CGO-free syntax parsing with explicit completion
// and error outcomes. Results and requests use the independent syntax package.
package treesitter

import (
	"github.com/wotjr1649/go-treesitter/internal/gtsadapter"
	"github.com/wotjr1649/go-treesitter/syntax"
)

// New returns a parser. Each Parse call owns its runtime parser; trees belong to
// the caller and must be closed. Inspect Result.Outcome even when error is nil.
func New() syntax.Parser { return gtsadapter.Adapter{} }
