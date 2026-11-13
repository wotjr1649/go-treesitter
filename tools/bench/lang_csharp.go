//go:build lang_csharp

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/csharp"
)

func language() *treesitter.Language { return csharp.Language() }
