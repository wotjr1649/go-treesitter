package grammarpatch

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/go-treesitter/tools/gen/patches"
)

// The unpinned path serves only the report-only rehearsal: it needs a recorded version and refuses pin fields, and the
// default (pinned) path never takes it.
func TestCheckCLIUnpinned(t *testing.T) {
	ok := Config{Unpinned: true, CLI: "tree-sitter", CLIVersion: "tree-sitter 0.28.0"}
	if err := checkCLI(ok); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]Config{
		"archive pin": {Unpinned: true, CLI: "x", CLIVersion: "v", CLIArchiveSHA256: "abc"},
		"archive":     {Unpinned: true, CLI: "x", CLIVersion: "v", CLIArchive: "a.gz"},
		"no version":  {Unpinned: true, CLI: "x"},
		"no binary":   {Unpinned: true, CLIVersion: "v"},
	} {
		if checkCLI(c) == nil {
			t.Errorf("%s: unpinned config accepted", name)
		}
	}
	// Without Unpinned the pinned verification runs and rejects a Config that lacks its pin.
	if checkCLI(Config{CLI: "tree-sitter", CLIVersion: "0.27.2"}) == nil {
		t.Error("pinned path accepted a Config without a pinned archive")
	}
}

// CheckInputs verifies the archive and the npm inputs without generating, so a later Prepare can run offline.
func TestCheckInputs(t *testing.T) {
	lock := `{"lockfileVersion":3,"packages":{"node_modules/tiny":{"version":"1","resolved":"https://registry.npmjs.org/tiny.tgz","integrity":"sha512-x"}}}`
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	tw := tar.NewWriter(z)
	for name, body := range map[string]string{"g/package-lock.json": lock, "g/grammar.js": "module.exports = 0;\n"} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(body)), Mode: 0o644, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	z.Close()
	archive := filepath.Join(t.TempDir(), "g.tar.gz")
	if err := os.WriteFile(archive, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	c := Config{Archive: archive, ArchiveSHA256: hash(b.Bytes()), GrammarDir: "src", Regenerate: true, NPMCache: t.TempDir()}
	if err := CheckInputs(c); err != nil {
		t.Fatal(err)
	}
	c.NPMDependencies = []string{"tiny"}
	if err := CheckInputs(c); err == nil || !strings.Contains(err.Error(), "enable fetching") {
		t.Fatalf("missing npm input accepted offline: %v", err)
	}
	c.ArchiveSHA256 = hash(nil)
	if err := CheckInputs(c); err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("archive hash mismatch accepted: %v", err)
	}
}

// The common input check applies the input patches like Prepare does and finds a missing generation input in both
// modes (explicit grammar.json, default grammar.js in the generate directory); C patches on generated files are left to
// each CLI.
func TestCheckInputsAppliesInputPatches(t *testing.T) {
	c := fixture(t, true) // grammar.js input patch, grammar.js in the generate directory
	if err := CheckInputs(c); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c.Work); !os.IsNotExist(err) {
		t.Fatalf("CheckInputs used the caller's Work directory: %v", err)
	}
	bad := c
	bad.Patches = append([]patches.Pin(nil), c.Patches...)
	bad.Patches[0].BeforeSHA256 = hash(nil)
	if err := CheckInputs(bad); err == nil {
		t.Fatal("a broken input patch passed the common input check")
	}
	jsonMissing := c
	jsonMissing.GenerateFrom = "grammar.json"
	jsonMissing.GrammarDir = "nosuch"
	if err := CheckInputs(jsonMissing); err == nil || !strings.Contains(err.Error(), "generation input grammar.json") {
		t.Fatalf("a missing explicit grammar.json passed: %v", err)
	}
	jsMissing := c
	jsMissing.GrammarDir = "sub/src" // generate would run in sub/, which has no grammar.js
	if err := CheckInputs(jsMissing); err == nil || !strings.Contains(err.Error(), "grammar.js missing") {
		t.Fatalf("a missing default grammar.js passed: %v", err)
	}
	gen := fixture(t, false) // C patch on src/parser.c, applied after generation when regenerating
	gen.Regenerate, gen.GenerateFrom = true, "grammar.json"
	gen.Patches[0].BeforeSHA256 = hash(nil)
	if err := CheckInputs(gen); err != nil {
		t.Fatalf("a C patch on generated files belongs to each CLI's Prepare: %v", err)
	}
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for n, s := range files {
		p := filepath.Join(root, filepath.FromSlash(n))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// Only regular files with the names tree-sitter generate writes in that mode are imported; an expected file the CLI
// did not write disappears instead of keeping the archive's copy.
func TestImportGenerated(t *testing.T) {
	out, dst := t.TempDir(), t.TempDir()
	writeTree(t, out, map[string]string{"parser.c": "p", "grammar.json": "generated", "tree_sitter/parser.h": "h"})
	writeTree(t, dst, map[string]string{"grammar.json": "archive", "node-types.json": "archive schema", "scanner.c": "s"})
	if err := importGenerated(out, dst, false); err != nil {
		t.Fatal(err)
	}
	for n, want := range map[string]string{"parser.c": "p", "tree_sitter/parser.h": "h", "grammar.json": "generated", "scanner.c": "s"} {
		if b, err := os.ReadFile(filepath.Join(dst, filepath.FromSlash(n))); err != nil || string(b) != want {
			t.Fatalf("%s: %q %v", n, b, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "node-types.json")); !os.IsNotExist(err) {
		t.Fatalf("the archive's node-types.json survived a generation that did not write one: %v", err)
	}

	// From grammar.json: the input stays, and the output may not carry a grammar.json.
	out, dst = t.TempDir(), t.TempDir()
	writeTree(t, out, map[string]string{"parser.c": "p", "node-types.json": "[]"})
	writeTree(t, dst, map[string]string{"grammar.json": "input"})
	if err := importGenerated(out, dst, true); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dst, "grammar.json")); err != nil || string(b) != "input" {
		t.Fatalf("grammar.json input: %q %v", b, err)
	}
	writeTree(t, out, map[string]string{"grammar.json": "overwrite"})
	if err := importGenerated(out, dst, true); err == nil {
		t.Fatal("grammar.json in the output accepted when it is the input")
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "grammar.json")); string(b) != "input" {
		t.Fatalf("grammar.json input overwritten: %q", b)
	}

	for name, files := range map[string]map[string]string{
		"unknown file":     {"parser.c": "p", "scanner.c": "x"},
		"nested directory": {"parser.c": "p", "tree_sitter/x/y.h": "x"},
		"no parser.c":      {"node-types.json": "[]"},
	} {
		o := filepath.Join(t.TempDir(), "out")
		writeTree(t, o, files)
		if err := importGenerated(o, t.TempDir(), false); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

// TestMain doubles as a fake tree-sitter CLI (FAKE_TS_CLI=1) that reports the environment it was given.
func TestMain(m *testing.M) {
	if os.Getenv("FAKE_TS_CLI") == "1" {
		os.Exit(fakeCLI(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func fakeCLI(args []string) int {
	report := fmt.Sprintf("token=%t input=%s output=%s", os.Getenv("GH_TOKEN")+os.Getenv("GITHUB_TOKEN") != "",
		os.Getenv("TS_GENERATE_INPUT"), os.Getenv("TS_GENERATE_OUTPUT"))
	for i, a := range args {
		if a == "--output" && i+1 < len(args) {
			if os.WriteFile(filepath.Join(args[i+1], "parser.c"), []byte(report), 0o644) != nil {
				return 1
			}
			return 0
		}
	}
	return 2
}

// The unpinned generate child gets no GitHub token and exactly the two trusted mount paths, whatever the caller had.
func TestUnpinnedGenerateEnvironment(t *testing.T) {
	t.Setenv("FAKE_TS_CLI", "1")
	t.Setenv("GH_TOKEN", "dummy-not-a-token")
	t.Setenv("GITHUB_TOKEN", "dummy-not-a-token")
	t.Setenv("TS_GENERATE_INPUT", "stale-input")
	t.Setenv("TS_GENERATE_OUTPUT", "stale-output")
	root := filepath.Join(t.TempDir(), "first")
	writeTree(t, root, map[string]string{"grammar.js": "x"})
	c := Config{CLI: os.Args[0], Unpinned: true, GrammarDir: "src"}
	if err := generate(c, root); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "src", "parser.c"))
	if want := "token=false input=" + root + " output=" + root + ".out"; err != nil || string(b) != want {
		t.Fatalf("child environment %q, want %q (%v)", b, want, err)
	}
	for _, kv := range UnpinnedEnv() {
		if strings.HasPrefix(strings.ToUpper(kv), "TS_GENERATE_") || strings.Contains(strings.ToUpper(kv), "TOKEN=DUMMY") {
			t.Fatalf("UnpinnedEnv kept %q", kv)
		}
	}
}
