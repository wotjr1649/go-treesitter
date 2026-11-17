//go:build !no_swift

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/swift"

func init() { register("swift", g.Language) }
