//go:build !no_ruby

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/ruby"

func init() { register("ruby", g.Language) }
