module github.com/wotjr1649/go-treesitter/tools/conformance

go 1.26.0

require (
	github.com/wotjr1649/go-treesitter v0.1.0
	github.com/wotjr1649/go-treesitter/grammars/csharp v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/golang v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/json v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/python v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/tsql v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/tsx v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/typescript v0.0.0
)

// Local development: the conformance lane runs against the working tree.
replace (
	github.com/wotjr1649/go-treesitter => ../..
	github.com/wotjr1649/go-treesitter/grammars/csharp => ../../grammars/csharp
	github.com/wotjr1649/go-treesitter/grammars/golang => ../../grammars/golang
	github.com/wotjr1649/go-treesitter/grammars/json => ../../grammars/json
	github.com/wotjr1649/go-treesitter/grammars/python => ../../grammars/python
	github.com/wotjr1649/go-treesitter/grammars/tsql => ../../grammars/tsql
	github.com/wotjr1649/go-treesitter/grammars/tsx => ../../grammars/tsx
	github.com/wotjr1649/go-treesitter/grammars/typescript => ../../grammars/typescript
)
