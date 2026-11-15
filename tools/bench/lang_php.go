//go:build lang_php

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/php"
)

func language() *treesitter.Language { return php.Language() }
