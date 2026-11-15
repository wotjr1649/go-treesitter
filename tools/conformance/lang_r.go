//go:build !no_r

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/r"

func init() { register("r", g.Language) }
