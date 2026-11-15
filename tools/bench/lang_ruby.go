//go:build lang_ruby

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/ruby"
)

func language() *treesitter.Language { return ruby.Language() }
