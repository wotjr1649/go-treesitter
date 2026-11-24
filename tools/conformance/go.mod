module github.com/wotjr1649/go-treesitter/tools/conformance

go 1.26.0

require (
	github.com/wotjr1649/go-treesitter v0.6.0
	github.com/wotjr1649/go-treesitter/grammars/bash v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/c v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/cpp v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/csharp v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/css v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/dart v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/golang v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/html v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/java v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/javascript v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/json v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/kotlin v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/postgres v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/php v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/powershell v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/python v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/r v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/ruby v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/rust v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/swift v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/tsql v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/tsx v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/typescript v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/xml v0.0.0
	github.com/wotjr1649/go-treesitter/grammars/yaml v0.0.0
	github.com/wotjr1649/go-treesitter/tools/bench/proc v0.0.0
)

// Local development: the conformance lane runs against the working tree.
replace (
	github.com/wotjr1649/go-treesitter => ../..
	github.com/wotjr1649/go-treesitter/grammars/bash => ../../grammars/bash
	github.com/wotjr1649/go-treesitter/grammars/c => ../../grammars/c
	github.com/wotjr1649/go-treesitter/grammars/cpp => ../../grammars/cpp
	github.com/wotjr1649/go-treesitter/grammars/csharp => ../../grammars/csharp
	github.com/wotjr1649/go-treesitter/grammars/css => ../../grammars/css
	github.com/wotjr1649/go-treesitter/grammars/dart => ../../grammars/dart
	github.com/wotjr1649/go-treesitter/grammars/golang => ../../grammars/golang
	github.com/wotjr1649/go-treesitter/grammars/html => ../../grammars/html
	github.com/wotjr1649/go-treesitter/grammars/java => ../../grammars/java
	github.com/wotjr1649/go-treesitter/grammars/javascript => ../../grammars/javascript
	github.com/wotjr1649/go-treesitter/grammars/json => ../../grammars/json
	github.com/wotjr1649/go-treesitter/grammars/kotlin => ../../grammars/kotlin
	github.com/wotjr1649/go-treesitter/grammars/postgres => ../../grammars/postgres
	github.com/wotjr1649/go-treesitter/grammars/php => ../../grammars/php
	github.com/wotjr1649/go-treesitter/grammars/powershell => ../../grammars/powershell
	github.com/wotjr1649/go-treesitter/grammars/python => ../../grammars/python
	github.com/wotjr1649/go-treesitter/grammars/r => ../../grammars/r
	github.com/wotjr1649/go-treesitter/grammars/ruby => ../../grammars/ruby
	github.com/wotjr1649/go-treesitter/grammars/rust => ../../grammars/rust
	github.com/wotjr1649/go-treesitter/grammars/swift => ../../grammars/swift
	github.com/wotjr1649/go-treesitter/grammars/tsql => ../../grammars/tsql
	github.com/wotjr1649/go-treesitter/grammars/tsx => ../../grammars/tsx
	github.com/wotjr1649/go-treesitter/grammars/typescript => ../../grammars/typescript
	github.com/wotjr1649/go-treesitter/grammars/xml => ../../grammars/xml
	github.com/wotjr1649/go-treesitter/grammars/yaml => ../../grammars/yaml
	github.com/wotjr1649/go-treesitter/tools/bench/proc => ../bench/proc
)
