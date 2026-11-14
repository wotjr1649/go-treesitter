//go:build !no_tsql

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/tsql"

func init() { register("tsql", g.Language) }
