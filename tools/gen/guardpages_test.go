package main

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fixture: the helper block, New, X_initialize and a few methods of a committed wasm2go v0.4.22 output (json), plus a
// nested byte/word access. It is a file of its own so that the tests do not depend on which committed modules are guarded.
func fixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/guard/engine_gen.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPatchGuardPages(t *testing.T) {
	out, maxOff, err := patchGuardPages(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "engine_gen.go", out, parser.AllErrors); err != nil {
		t.Fatalf("patched source does not parse: %v", err)
	}
	if maxOff != 76 {
		t.Errorf("max static offset %d, want 76 (the largest +N in the fixture)", maxOff)
	}
	for _, bad := range []string{"_ = mem[", "encoding/binary", "engine.InitMemory(", "m.memory[uint64"} {
		if strings.Contains(out, bad) {
			t.Errorf("patched source still contains %q", bad)
		}
	}
	for _, want := range []string{
		"engine.InitGuardedMemory(&m.memory, 0x20000,",
		"const _ = uint(engine.GuardSize - 84)",
		"panic(\"guard pages need unaligned memory access\")",
		"*(*byte)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(m.memory)), uint64(uint32(v3))))",                                    // field form
		"*(*byte)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(mem)), uint64(uint32(int32(load32(mem, uint64(uint32(v0))+8))))+4))", // nested
		"memory_copy", // slice-checked bulk operations are kept
	} {
		if !strings.Contains(out, want) {
			t.Errorf("patched source lacks %q", want)
		}
	}
	if again, _, err := patchGuardPages(out); err == nil || again != "" {
		t.Error("patching an already patched module must fail, not pass through")
	}
}

// Every committed module that is not guarded yet has the pinned helper block and only recognised access forms.
func TestPatchGuardPagesCommittedModules(t *testing.T) {
	files, _ := filepath.Glob("../../grammars/*/engine_gen.go")
	if len(files) == 0 {
		t.Skip("no committed modules")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "engine.InitGuardedMemory(") {
			continue
		}
		if _, _, err := patchGuardPages(string(b)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}

func TestPatchGuardPagesFailsClosed(t *testing.T) {
	src := fixture(t)
	sub := func(re, repl string) func(string) string {
		return func(s string) string { return regexp.MustCompile(re).ReplaceAllString(s, repl) }
	}
	for name, mut := range map[string]func(string) string{
		"helpers changed": func(s string) string {
			return strings.Replace(s, "_ = mem[uint64(addr)+3]", "_ = mem[uint64(addr)+2]", 1)
		},
		"unknown address":   sub("load32\\(mem, uint64\\(uint32\\((v\\d)\\)\\)\\+", "load32(mem, uint64($1)+"),
		"byte access shape": sub("mem\\[uint64\\(uint32\\(v4\\)\\)\\]", "mem[v4]"),
		"slice of memory":   sub("mem\\[uint64\\(uint32\\(v4\\)\\)\\]", "mem[uint64(uint32(v4)):4]"),
		"no InitMemory":     sub("engine\\.InitMemory\\(", "engine.Other("),
		"heap memory":       func(s string) string { return s + "\nvar _ = make([]byte, 1)\n" },
		"access outside fn": func(s string) string { return s + "\nvar _ = load32(m.memory, uint64(uint32(0)))\n" },
	} {
		mutated := mut(src)
		if mutated == src {
			t.Errorf("%s: mutation did not apply", name)
			continue
		}
		if _, _, err := patchGuardPages(mutated); err == nil {
			t.Errorf("%s: patch accepted unrecognised text", name)
		}
	}
}

func TestGuardRecipeChangesRecipeHash(t *testing.T) {
	base, guard := recipes["v2-O3"], recipes["v3-guard-O3"]
	if !guard.guard || base.guard || base.opt != guard.opt || base.embed != guard.embed {
		t.Fatal("v3-guard-O3 must equal v2-O3 plus guard")
	}
	if strings.Contains(base.recipeLines("v2-O3", &Pins{}), "guard-pages") || !strings.Contains(guard.recipeLines("v3-guard-O3", &Pins{}), helperBlockSHA256) {
		t.Fatal("recipe lines do not bind the guard patch")
	}
}
