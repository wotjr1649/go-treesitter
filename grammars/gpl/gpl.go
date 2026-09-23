// Package gpl adds the pinned caddy, disassembly and jq grammars to treesitter.New.
// Import it for side effects only after accepting their GPL-3.0 terms.
// This is a separate module; the base module does not depend on it.
package gpl

import _ "github.com/wotjr1649/go-treesitter/grammars/gpl/internal/gtsadapter"
