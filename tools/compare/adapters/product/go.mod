module github.com/wotjr1649/go-treesitter/tools/compare/adapters/product

go 1.26.0

// Pinned consumer configuration (plan phase 0): root v0.7.0 (7e777f7b0b9e99da540ca2c2c3f1481fb1c128b3) and the five grammar modules v0.6.0 from the public proxy.
// go.sum is produced by the bootstrap job (tools/compare/README.md) and committed after review; the measurement
// jobs fail when it is missing or when 'go mod verify' / '-mod=readonly' want to change anything.
require (
	github.com/wotjr1649/go-treesitter v0.7.0
	github.com/wotjr1649/go-treesitter/grammars/c v0.6.0
	github.com/wotjr1649/go-treesitter/grammars/golang v0.6.0
	github.com/wotjr1649/go-treesitter/grammars/javascript v0.6.0
	github.com/wotjr1649/go-treesitter/grammars/json v0.6.0
	github.com/wotjr1649/go-treesitter/grammars/python v0.6.0
	github.com/wotjr1649/go-treesitter/tools/compare/harness v0.0.0
)

replace github.com/wotjr1649/go-treesitter/tools/compare/harness => ../../harness
