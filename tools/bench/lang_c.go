//go:build lang_c

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/c"
)

func language() *treesitter.Language { return c.Language() }
