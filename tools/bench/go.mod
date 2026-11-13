module github.com/wotjr1649/go-treesitter/tools/bench

go 1.27.2

require (
	github.com/wotjr1649/go-treesitter v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/csharp v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/golang v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/json v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/python v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/tsql v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/tsx v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/typescript v0.0.0
)

// Local development, like the grammar modules: the product lives in this repository.
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
