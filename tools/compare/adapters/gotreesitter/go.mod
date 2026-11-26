module github.com/wotjr1649/go-treesitter/tools/compare/adapters/gotreesitter

go 1.26.0

// odvcencio/gotreesitter v0.58.0 (4ab5d4e2504437ca0e387b6cde8b9af12e460bfa): standalone grammar packages, release-default engine and scanners, no build tags.
// go.sum is produced by the bootstrap job (tools/compare/README.md) and committed after review; the measurement
// jobs fail when it is missing or when 'go mod verify' / '-mod=readonly' want to change anything.
require (
	github.com/odvcencio/gotreesitter v0.58.0
	github.com/wotjr1649/go-treesitter/tools/compare/harness v0.0.0
)

replace github.com/wotjr1649/go-treesitter/tools/compare/harness => ../../harness
