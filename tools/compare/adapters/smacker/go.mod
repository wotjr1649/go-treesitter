module github.com/wotjr1649/go-treesitter/tools/compare/adapters/smacker

go 1.26.0

// smacker/go-tree-sitter at dd81d9e9be82a8cac96ed1d50c7389c5f1997c02 (stock bundled grammar packages; no JSON package exists).
// go.sum is produced by the bootstrap job (tools/compare/README.md) and committed after review; the measurement
// jobs fail when it is missing or when 'go mod verify' / '-mod=readonly' want to change anything.
require (
	github.com/smacker/go-tree-sitter v0.0.0-20240827094217-dd81d9e9be82
	github.com/wotjr1649/go-treesitter/tools/compare/harness v0.0.0
)

replace github.com/wotjr1649/go-treesitter/tools/compare/harness => ../../harness
