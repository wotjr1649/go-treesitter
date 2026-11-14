//go:build !no_java

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/java"

func init() { register("java", g.Language) }
