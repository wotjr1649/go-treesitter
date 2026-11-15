//go:build !no_kotlin

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/kotlin"

func init() { register("kotlin", g.Language) }
