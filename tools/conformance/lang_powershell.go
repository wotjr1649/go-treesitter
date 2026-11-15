//go:build !no_powershell

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/powershell"

func init() { register("powershell", g.Language) }
