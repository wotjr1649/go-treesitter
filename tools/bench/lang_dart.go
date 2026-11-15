//go:build lang_dart

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/dart"
)

func language() *treesitter.Language { return dart.Language() }
