//go:build lang_python

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/python"
)

func language() *treesitter.Language { return python.Language() }
