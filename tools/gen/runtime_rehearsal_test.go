package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runtimeFixture(t *testing.T) (src, archive string) {
	t.Helper()
	dir := t.TempDir()
	src = filepath.Join(dir, "src")
	tree := filepath.Join(src, "tree-sitter-fake")
	if err := os.MkdirAll(filepath.Join(tree, "lib", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"lib/src/a.c": "int a;\n", "LICENSE": "MIT\n"} {
		if err := os.WriteFile(filepath.Join(tree, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	archive = filepath.Join(dir, "fake.tar.gz")
	if err := os.WriteFile(archive, []byte("archive bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	return src, archive
}

func TestRuntimePinsCommandAndGuard(t *testing.T) {
	src, archive := runtimeFixture(t)
	out := filepath.Join(t.TempDir(), "pins.json")
	args := []string{"-root", ".", "-src", src, "-extract-dir", "tree-sitter-fake", "-archive", archive,
		"-release", "release-0.27-9a1a129b", "-commit", strings.Repeat("ab", 20), "-out", out}
	var stdout bytes.Buffer
	if err := runRuntimePinsCommand(args, &stdout); err != nil {
		t.Fatal(err)
	}
	if err := checkRuntimeOnlyPins("pins.json", out); err != nil {
		t.Fatal(err)
	}
	pins, err := loadPins(out)
	if err != nil {
		t.Fatal(err)
	}
	rt := pins.Runtime
	if rt.Release != "release-0.27-9a1a129b" || rt.Archive != "fake.tar.gz" || rt.ExtractDir != "tree-sitter-fake" ||
		rt.ArchiveSHA256 != sumBytes([]byte("archive bytes")) || rt.LicenseSHA256 != sumBytes([]byte("MIT\n")) || rt.TreeSHA256 == "" {
		t.Fatalf("runtime block not rewritten: %+v", rt)
	}
	if rt.Repo == "" || len(rt.Dirs) == 0 || pins.Tools.TreeSitterCLI == "" {
		t.Fatalf("copy lost committed fields: %+v", pins)
	}
	if err := runRuntimePinsCommand(args, &stdout); err == nil {
		t.Fatal("existing -out was overwritten")
	}
	// Any change outside the runtime block is refused.
	b, _ := os.ReadFile(out)
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, bytes.Replace(b, []byte(pins.Tools.TreeSitterCLI), []byte("9.9.9"), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkRuntimeOnlyPins("pins.json", bad); err == nil {
		t.Fatal("pins copy with a moved CLI accepted")
	}
	// The committed file itself is never a valid -out, and bad identities are refused.
	for _, tc := range [][]string{
		{"-out", "pins.json"},
		{"-commit", "main"},
		{"-release", "../x"},
		{"-extract-dir", "../x"},
	} {
		a := append(append([]string{}, args...), tc...) // later flags win
		if err := runRuntimePinsCommand(a, &stdout); err == nil {
			t.Fatalf("%v accepted", tc)
		}
	}
}

func TestRuntimeRehearsalOutputBoundary(t *testing.T) {
	repo := t.TempDir()
	o := opts{repo: repo, out: filepath.Join(repo, "grammars", "json"), work: filepath.Join(repo, "work")}
	if err := validateTestOnlyOutput(o, "-runtime-rehearsal"); err == nil || !strings.Contains(err.Error(), "-runtime-rehearsal") {
		t.Fatalf("product output accepted: %v", err)
	}
}
