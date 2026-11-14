//go:build !no_csharp

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/csharp"

func init() { register("csharp", g.Language) }
