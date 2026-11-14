//go:build !no_c

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/c"

func init() { register("c", g.Language) }
