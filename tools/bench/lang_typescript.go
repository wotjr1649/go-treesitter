//go:build lang_typescript

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/typescript"
)

func language() *treesitter.Language { return typescript.Language() }
