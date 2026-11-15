//go:build !no_xml

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/xml"

func init() { register("xml", g.Language) }
