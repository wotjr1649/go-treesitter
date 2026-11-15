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

func TestFieldPlanClassification(t *testing.T) {
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
	if err := prepare(p.Runtime, dl, work, true, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	shim, err := os.ReadFile(filepath.Join("..", "gen", "shim", "tsg_abi.c"))
	if err != nil {
		t.Fatal(err)
	}
	const guard = "other->field_id == f->field_id && other->child_index > f->child_index"
	if bytes.Count(shim, []byte(guard)) != 1 {
		t.Fatal("descending-field mutation must target exactly one guard")
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
			name = "without-descending-guard"
		}
		t.Run(name, func(t *testing.T) {
			body := shim
			if mutation {
				body = bytes.Replace(shim, []byte(guard), []byte("false"), 1)
			}
			copyPath := filepath.Join(work, name+"-shim.c")
			if err := os.WriteFile(copyPath, body, 0o644); err != nil {
				t.Fatal(err)
			}
			exe := filepath.Join(work, name)
			args := []string{"-std=c11", "-fsigned-char", "-O2", "-Wno-attributes", "-I" + filepath.Join(lib, "include"), "-I" + filepath.Join(lib, "src"),
				"-DTSG_LANG_FN=tsg_test_language", `-DTSG_FIELD_PLAN_SHIM="` + filepath.ToSlash(copyPath) + `"`}
			if runtime.GOOS == "windows" {
				exe += ".exe"
				args = append(args, "-static")
			} else {
				args = append(args, "-D_POSIX_C_SOURCE=200112L", "-D_DEFAULT_SOURCE")
				if runtime.GOOS == "darwin" {
					args = append(args, "-D_DARWIN_C_SOURCE")
				}
			}
			args = append(args, filepath.Join(lib, "src", "lib.c"), filepath.Join(here, "field_plan_probe.c"), "-o", exe)
			if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
				t.Fatalf("compile: %v\n%s", err, out)
			}
			out, err := exec.Command(exe).CombinedOutput()
			if !mutation && err != nil {
				t.Fatalf("product shim: %v\n%s", err, out)
			}
			if mutation && (err == nil || !strings.Contains(string(out), "field plan mismatch: descending same-field")) {
				t.Fatalf("missing descending guard was not detected: %v\n%s", err, out)
			}
		})
	}
}
