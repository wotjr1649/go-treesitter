//go:build !no_postgres

package conformance

import g "github.com/wotjr1649/go-treesitter/grammars/postgres"

func init() { register("postgres", g.Language) }
