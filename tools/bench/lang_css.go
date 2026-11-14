//go:build lang_css

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/css"
)

func language() *treesitter.Language { return css.Language() }
