//go:build lang_powershell

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/powershell"
)

func language() *treesitter.Language { return powershell.Language() }
