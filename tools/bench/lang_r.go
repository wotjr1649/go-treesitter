//go:build lang_r

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/r"
)

func language() *treesitter.Language { return r.Language() }
