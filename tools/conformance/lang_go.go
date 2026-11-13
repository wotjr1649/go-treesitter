//go:build !no_go

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/golang"

func init() { register("go", g.Language) }
