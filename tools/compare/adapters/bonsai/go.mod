module github.com/wotjr1649/go-treesitter/tools/compare/adapters/bonsai

go 1.26.0

// msuozzo/bonsai v0.4.1 (a2082365dea428bce3fbfb0f7cfa8ef67d0a56a0) with the released language modules; all modules release in lockstep.
// go.sum is produced by the bootstrap job (tools/compare/README.md) and committed after review; the measurement
// jobs fail when it is missing or when 'go mod verify' / '-mod=readonly' want to change anything.
require (
	github.com/msuozzo/bonsai v0.4.1
	github.com/msuozzo/bonsai/bonsai-c v0.4.1
	github.com/msuozzo/bonsai/bonsai-go v0.4.1
	github.com/msuozzo/bonsai/bonsai-javascript v0.4.1
	github.com/msuozzo/bonsai/bonsai-python v0.4.1
	github.com/wotjr1649/go-treesitter/tools/compare/harness v0.0.0
)

replace github.com/wotjr1649/go-treesitter/tools/compare/harness => ../../harness
