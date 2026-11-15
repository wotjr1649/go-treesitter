//go:build lang_kotlin

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/kotlin"
)

func language() *treesitter.Language { return kotlin.Language() }
