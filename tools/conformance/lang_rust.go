//go:build !no_rust

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/rust"

func init() { register("rust", g.Language) }
