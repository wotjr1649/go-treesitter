//go:build !no_php

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/php"

func init() { register("php", g.Language) }
