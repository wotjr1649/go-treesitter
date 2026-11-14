//go:build lang_html

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/html"
)

func language() *treesitter.Language { return html.Language() }
