//go:build !no_dart

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/dart"

func init() { register("dart", g.Language) }
