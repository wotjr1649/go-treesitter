module github.com/wotjr1649/go-treesitter/tools/compare/adapters/malivvan

go 1.26.0

// malivvan/tree-sitter v0.0.1 (5aa45b3d087ac4f9014faa31e2ea7a5eeabe834b): embedded wasm run by wazero; C only for this workload.
// go.sum is produced by the bootstrap job (tools/compare/README.md) and committed after review; the measurement
// jobs fail when it is missing or when 'go mod verify' / '-mod=readonly' want to change anything.
require (
	github.com/malivvan/tree-sitter v0.0.1
	github.com/wotjr1649/go-treesitter/tools/compare/harness v0.0.0
)

require github.com/tetratelabs/wazero v1.8.2 // indirect

replace github.com/wotjr1649/go-treesitter/tools/compare/harness => ../../harness
