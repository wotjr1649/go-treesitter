//go:build !no_yaml

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/yaml"

func init() { register("yaml", g.Language) }
