//go:build lang_postgres

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/postgres"
)

func language() *treesitter.Language { return postgres.Language() }
