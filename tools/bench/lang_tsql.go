//go:build lang_tsql

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/tsql"
)

func language() *treesitter.Language { return tsql.Language() }
