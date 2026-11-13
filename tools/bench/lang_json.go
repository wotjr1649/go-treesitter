//go:build lang_json

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/json"
)

func language() *treesitter.Language { return json.Language() }
