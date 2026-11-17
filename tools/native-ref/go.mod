module github.com/wotjr1649/go-treesitter/tools/native-ref

go 1.26.0

require github.com/wotjr1649/go-treesitter/tools/gen v0.0.0

replace github.com/wotjr1649/go-treesitter/tools/gen => ../gen

require github.com/wotjr1649/go-treesitter/tools/bench/proc v0.0.0

replace github.com/wotjr1649/go-treesitter/tools/bench/proc => ../bench/proc
