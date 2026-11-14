package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWasiCtype(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "gen", "pins.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p pins
	if err := json.Unmarshal(raw, &p); err != nil {
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
	args := []string{"-std=c11", "-I" + filepath.Join(here, "shim"), filepath.Join(here, "ctype_probe.c")}
	if runtime.GOOS == "windows" {
		exe += ".exe"
		args = append(args, "-static")
	}
	args = append(args, objects...)
	args = append(args, "-o", exe)
	if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
		t.Fatalf("compile: %v\n%s", err, out)
	}
	if out, err := exec.Command(exe).CombinedOutput(); err != nil {
		t.Fatalf("wasi-libc probe: %v\n%s", err, out)
	}
}
