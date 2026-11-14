//go:build !no_typescript

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/typescript"

func init() { register("typescript", g.Language) }
