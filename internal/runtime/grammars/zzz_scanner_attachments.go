//go:build !grammar_subset

package grammars

import grammarruntime "github.com/wotjr1649/go-treesitter/internal/runtime/grammars/runtime"

func init() { grammarruntime.RegisterBuiltinScanners() }
