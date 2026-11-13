//go:build lang_golang

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/golang"
)

func language() *treesitter.Language { return golang.Language() }
