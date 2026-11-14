//go:build lang_java

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/java"
)

func language() *treesitter.Language { return java.Language() }
