// Package langid lets experimental packages ask whether two *treesitter.Language values wrap the same generated
// grammar without adding public API: package treesitter sets Same in init. Arguments are any to avoid an import cycle.
package langid

// Same reports whether a and b are *treesitter.Language values of one grammar (false for anything else).
var Same = func(a, b any) bool { return false }
