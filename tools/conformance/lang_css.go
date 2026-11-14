//go:build !no_css

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/css"

func init() { register("css", g.Language) }
