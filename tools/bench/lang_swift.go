//go:build lang_swift

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/swift"
)

func language() *treesitter.Language { return swift.Language() }
