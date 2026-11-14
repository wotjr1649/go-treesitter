//go:build !no_cpp

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/cpp"

func init() { register("cpp", g.Language) }
