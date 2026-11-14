//go:build lang_tsx

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/tsx"
)

func language() *treesitter.Language { return tsx.Language() }
