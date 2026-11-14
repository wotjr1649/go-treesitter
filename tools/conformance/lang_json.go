//go:build !no_json

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/json"

func init() { register("json", g.Language) }
