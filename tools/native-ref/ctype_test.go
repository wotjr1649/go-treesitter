package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wotjr1649/go-treesitter/tools/bench/proc"
)

func TestWasiCtype(t *testing.T) {
	p, err := readPins(filepath.Join("..", "gen", "pins.json"))
	if err != nil {
		t.Fatal(err)
	}
	here, err := sourceDir()
	if err != nil {
		t.Fatal(err)
	}
	dl := os.Getenv("TSG_NATIVE_DL")
	if dl == "" {
		dl = t.TempDir()
	}
	cc := os.Getenv("CC")
	if cc == "" {
		cc = "gcc"
	}
	work := t.TempDir()
	objects, err := buildCtype(p.WasiLibc, dl, work, here, cc, "-O2", true)
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(work, "ctype-probe")
	args := []string{"-std=c11", "-fsigned-char", "-fno-builtin", "-Werror=implicit-function-declaration", "-I" + filepath.Join(here, "shim"), filepath.Join(here, "ctype_probe.c")}
	if runtime.GOOS == "windows" {
		exe += ".exe"
		args = append(args, "-static")
	}
	args = append(args, objects...)
	args = append(args, "-o", exe)
	if out, err := proc.Command(cc, args...).CombinedOutput(); err != nil {
		t.Fatalf("compile: %v\n%s", err, out)
	}
	if out, err := proc.Command(exe).CombinedOutput(); err != nil {
		t.Fatalf("wasi-libc probe: %v\n%s", err, out)
	}
}
