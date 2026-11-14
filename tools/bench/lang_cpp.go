//go:build lang_cpp

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/cpp"
)

func language() *treesitter.Language { return cpp.Language() }
