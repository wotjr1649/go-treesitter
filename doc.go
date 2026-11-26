// Package treesitter is a CGO-free Go library for tree-sitter.
//
// The root module contains no grammar. Each language is a separate module under grammars/ whose Language
// function returns the *Language that NewParser takes, for example
// github.com/wotjr1649/go-treesitter/grammars/golang. A grammar module carries its own translated tree-sitter
// runtime and grammar, so building needs no C toolchain (CGO_ENABLED=0) and the grammar tags version
// independently of this module. NewLanguage is the entry point of generated grammar code, not for applications.
//
// # Parsing
//
// NewParser creates a Parser for one Language. Parser.ParseSnapshot returns an immutable *Snapshot: it owns its
// data, needs no Close and may be shared between goroutines. Parser.Parse returns a *Tree for incremental work
// (Tree.Edit, then Parse with the edited tree as the old tree) and for queries (NewQuery, Tree.Matches).
//
// A nil error means a complete tree, which can still contain syntax errors: check Node.HasError, or
// Tree.Outcome (AcceptedClean or AcceptedWithErrors). Any other result is a *ParseError, which carries the
// Outcome; match failures with errors.Is against the Err sentinels (ErrTimeout, ErrCancelled, ErrResourceLimit,
// and so on) and use errors.As for the diagnostic fields. ErrQueryMatchLimit, the one sentinel that is not a
// *ParseError, also matches ErrResourceLimit. Error message text may change in any release.
//
// # Lifetimes and concurrency
//
// Close every Tree and every Parser. Closing or resetting a Parser invalidates its live trees, but snapshots stay
// valid. A Parser and its trees are one confinement unit: use them from one goroutine at a time. NewParserPool
// lends parsers to concurrent goroutines. Options bound the cost of untrusted input: WithMaxInputBytes the input,
// WithMaxSnapshotNodes the node count of an exported Snapshot, and WithMaxMemoryBytes the linear memory of one
// Parser instance (not the Go heap of sources and snapshots).
//
// # Input and positions
//
// Input is UTF-8. NewSource converts UTF-16 that starts with a byte order mark, NewSourceUTF16 converts UTF-16
// of a known byte order, and NewSourceCP949 converts CP949; nothing guesses an encoding. Snapshot.Lines returns
// a LineIndex that converts byte offsets to lines and byte, UTF-16 or rune columns.
//
// # Stability
//
// From root 1.0 this package follows the compatibility rules in docs/design/api-stability.md (the v0.x Preview and
// release candidates propose them and promise nothing yet): a grammar module can change trees and captures when its source pin moves, while the API and its documented behavior with a fixed
// grammar stay compatible. Package github.com/wotjr1649/go-treesitter/experimental/match is EXPERIMENTAL and is
// outside that promise; it may change or be removed in any release.
//
// Runnable examples are in tools/conformance/example_test.go (https://github.com/wotjr1649/go-treesitter/blob/main/tools/conformance/example_test.go).
// They live in that module because a grammar module requires this one: importing a grammar here would make an import
// cycle, and an external test package would add a grammar dependency to this module.
package treesitter
