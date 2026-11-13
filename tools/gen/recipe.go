package main

import (
	"fmt"
	"sort"
	"strings"
)

// recipe is one build variant. Size round (session 15, P3): v0 is the baseline; v1 runs wasm-opt on clang's output;
// v2 additionally keeps the wasm data sections in a go:embed file (wasm2go -embed) instead of Go string literals.
type recipe struct {
	opt   string // wasm-opt level ("-O3", "-Oz"), "" = no wasm-opt
	embed bool   // wasm2go -embed: engine_gen.dat next to engine_gen.go
}

var recipes = map[string]recipe{
	"v0":    {},
	"v1-O3": {opt: "-O3"},
	"v1-Oz": {opt: "-Oz"},
	"v2-v0": {embed: true},
	"v2-O3": {opt: "-O3", embed: true},
	"v2-Oz": {opt: "-Oz", embed: true},
}

// optFeatures declares what clang emitted: `--strip-all` removes the target_features section, so wasm-opt would
// otherwise reject bulk-memory and the other post-MVP instructions.
var optFeatures = []string{"--enable-bulk-memory", "--enable-bulk-memory-opt", "--enable-sign-ext", "--enable-mutable-globals",
	"--enable-nontrapping-float-to-int", "--enable-multivalue", "--enable-reference-types"}

func lookupRecipe(name string) (recipe, error) {
	r, ok := recipes[name]
	if !ok {
		names := make([]string, 0, len(recipes))
		for n := range recipes {
			names = append(names, n)
		}
		sort.Strings(names)
		return r, fmt.Errorf("unknown recipe %q (have %s)", name, strings.Join(names, ", "))
	}
	return r, nil
}

// wasmOptArgs is the exact wasm-opt invocation (without the input and output paths).
func (r recipe) wasmOptArgs() []string { return append(append([]string{}, optFeatures...), r.opt) }

// wasm2goFlags is the wasm2go invocation's flags (without -pkg and -o).
func (r recipe) wasm2goFlags() []string {
	if r.embed {
		return []string{"-unsafe", "-embed"}
	}
	return []string{"-unsafe"}
}

// recipeLines is the recipe's contribution to RecipeSHA256. v0 contributes nothing, so its hash is the one the
// baseline modules were generated with.
func (r recipe) recipeLines(name string, pins *Pins) string {
	if name == "v0" {
		return ""
	}
	s := fmt.Sprintf("\nrecipe=%s", name)
	if r.opt != "" {
		s += fmt.Sprintf("\nwasm-opt(after clang, before wasm2go)=%s\nbinaryen=%s", strings.Join(r.wasmOptArgs(), " "), pins.Tools.Binaryen)
	}
	if r.embed {
		s += "\nwasm2go-embed=engine_gen.dat"
	}
	return s
}
