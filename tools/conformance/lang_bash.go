//go:build !no_bash

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/bash"

func init() { register("bash", g.Language) }
