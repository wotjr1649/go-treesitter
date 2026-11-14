//go:build !no_html

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/html"

func init() { register("html", g.Language) }
