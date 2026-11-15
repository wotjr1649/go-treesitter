//go:build lang_xml

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/xml"
)

func language() *treesitter.Language { return xml.Language() }
