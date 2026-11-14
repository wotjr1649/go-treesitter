//go:build lang_bash

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/bash"
)

func language() *treesitter.Language { return bash.Language() }
