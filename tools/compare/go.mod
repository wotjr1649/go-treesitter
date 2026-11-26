module github.com/wotjr1649/go-treesitter/tools/compare

go 1.26.0

require (
	github.com/wotjr1649/go-treesitter/tools/bench/proc v0.0.0
	github.com/wotjr1649/go-treesitter/tools/compare/harness v0.0.0
)

// Local development: both helpers live in this repository. This module imports no third-party code; the candidate
// libraries are confined to the adapter modules under adapters/, one module each (their C runtimes would collide
// in a single binary).
replace (
	github.com/wotjr1649/go-treesitter/tools/bench/proc => ../bench/proc
	github.com/wotjr1649/go-treesitter/tools/compare/harness => ./harness
)
