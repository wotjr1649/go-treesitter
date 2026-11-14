//go:build lang_javascript

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/javascript"
)

func language() *treesitter.Language { return javascript.Language() }
