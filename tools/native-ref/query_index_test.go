package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Exercise the actual C invalidation path and a mutation without relying on allocator address reuse.
func TestQueryIndexInvalidation(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "gen", "pins.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p pins
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	dl := os.Getenv("TSG_NATIVE_DL")
	if dl == "" {
		dl = t.TempDir()
	}
	work := t.TempDir()
	done := map[string]bool{}
	g := p.Grammars["json"]
	for _, s := range []source{p.Runtime, g} {
		if err := prepare(s, dl, work, true, done); err != nil {
			t.Fatal(err)
		}
	}
	gdir := filepath.Join(work, g.ExtractDir, filepath.FromSlash(g.GrammarDir))
	for name, want := range g.Files {
		if got, err := fileSHA(filepath.Join(gdir, name)); err != nil || got != want {
			t.Fatalf("%s sha256 %s, pinned %s (%v)", name, got, want, err)
		}
	}
	shim, err := os.ReadFile(filepath.Join("..", "gen", "shim", "tsg_abi.c"))
	if err != nil {
		t.Fatal(err)
	}
	const invalidation = "qcursor_drop_tree(t); ts_tree_delete(t);"
	if bytes.Count(shim, []byte(invalidation)) != 1 {
		t.Fatal("query-index invalidation mutation must target exactly one call")
	}
	here, err := sourceDir()
	if err != nil {
		t.Fatal(err)
	}
	cc := os.Getenv("CC")
	if cc == "" {
		cc = "gcc"
	}
	lib := filepath.Join(work, p.Runtime.ExtractDir, "lib")
	for _, mutation := range []bool{false, true} {
		name := "product"
		if mutation {
			name = "without-invalidation"
		}
		t.Run(name, func(t *testing.T) {
			body := shim
			if mutation {
				body = bytes.Replace(shim, []byte(invalidation), []byte("ts_tree_delete(t);"), 1)
			}
			copyPath := filepath.Join(work, name+"-shim.c")
			if err := os.WriteFile(copyPath, body, 0o644); err != nil {
				t.Fatal(err)
			}
			exe := filepath.Join(work, name)
			args := []string{"-std=c11", "-O2", "-Wno-attributes", "-I" + filepath.Join(lib, "include"), "-I" + filepath.Join(lib, "src"), "-I" + gdir,
				"-DTSG_LANG_FN=" + g.CSymbol, `-DTSG_INDEX_SHIM="` + filepath.ToSlash(copyPath) + `"`}
			if runtime.GOOS == "windows" {
				exe += ".exe"
				args = append(args, "-static")
			} else {
				args = append(args, "-D_POSIX_C_SOURCE=200112L", "-D_DEFAULT_SOURCE")
				if runtime.GOOS == "darwin" {
					args = append(args, "-D_DARWIN_C_SOURCE")
				}
			}
			args = append(args, filepath.Join(lib, "src", "lib.c"), filepath.Join(gdir, "parser.c"), filepath.Join(here, "query_index_probe.c"), "-o", exe)
			if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
				t.Fatalf("compile: %v\n%s", err, out)
			}
			out, err := exec.Command(exe).CombinedOutput()
			if !mutation && err != nil {
				t.Fatalf("product shim: %v\n%s", err, out)
			}
			if mutation && (err == nil || !strings.Contains(string(out), "cached query index survived tree deletion")) {
				t.Fatalf("missing-invalidation mutation was not detected: %v\n%s", err, out)
			}
		})
	}
}
