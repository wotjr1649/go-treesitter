module github.com/wotjr1649/go-treesitter/tools/compare/adapters/official

go 1.26.0

// tree-sitter/go-tree-sitter v0.24.0 (5432ade78ad6bde797bcdaa272a5953eaba83971) with the five grammar modules at the versions of that release's go.mod.
// go.sum is produced by the bootstrap job (tools/compare/README.md) and committed after review; the measurement
// jobs fail when it is missing or when 'go mod verify' / '-mod=readonly' want to change anything.
require (
	github.com/tree-sitter/go-tree-sitter v0.24.0
	github.com/tree-sitter/tree-sitter-c v0.21.5-0.20240818205408-927da1f210eb
	github.com/tree-sitter/tree-sitter-go v0.21.3-0.20240818010209-8c0f0e7a6012
	github.com/tree-sitter/tree-sitter-javascript v0.21.5-0.20240818005344-15887341e5b5
	github.com/tree-sitter/tree-sitter-json v0.21.1-0.20240818005659-bdd69eb8c8a5
	github.com/tree-sitter/tree-sitter-python v0.21.1-0.20240818005537-55a9b8a4fbfb
	github.com/wotjr1649/go-treesitter/tools/compare/harness v0.0.0
)

require github.com/mattn/go-pointer v0.0.1 // indirect

replace github.com/wotjr1649/go-treesitter/tools/compare/harness => ../../harness
