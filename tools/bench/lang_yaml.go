//go:build lang_yaml

package main

import (
	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/yaml"
)

func language() *treesitter.Language { return yaml.Language() }
