//go:build lang_rust

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/rust"
)

func language() *treesitter.Language { return rust.Language() }
