//go:build !no_tsx

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/tsx"

func init() { register("tsx", g.Language) }
