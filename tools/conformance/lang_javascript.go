//go:build !no_javascript

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/javascript"

func init() { register("javascript", g.Language) }
