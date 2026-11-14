//go:build !no_python

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/python"

func init() { register("python", g.Language) }
