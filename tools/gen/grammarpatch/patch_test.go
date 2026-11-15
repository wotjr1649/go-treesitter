package grammarpatch

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wotjr1649/go-treesitter/tools/gen/patches"
)

func fixture(t *testing.T, source bool) Config {
	t.Helper()
	dir := t.TempDir()
	before, after, name := "int a;\n", "int b;\n", "src/parser.c"
	if source {
		name = "grammar.js"
		before = "module.exports = grammar({name: 'tiny', rules: {source_file: $ => repeat('a')}});\n"
		after = "module.exports = grammar({name: 'tiny', rules: {source_file: $ => repeat(choice('a', 'b'))}});\n"
	}
	files := map[string]string{"src/parser.c": "int a;\n", name: before,
		"tree-sitter.json": `{"grammars":[{"name":"tiny","scope":"source.tiny","path":".","file-types":["tiny"]}],"metadata":{"version":"0.0.1","license":"MIT"}}`}
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	tw := tar.NewWriter(z)
	for n, s := range files {
		if err := tw.WriteHeader(&tar.Header{Name: "tiny/" + n, Size: int64(len(s)), Mode: 0o644}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "tiny.tar.gz")
	if err := os.WriteFile(archive, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := "diff --git a/" + name + " b/" + name + "\n--- a/" + name + "\n+++ b/" + name + "\n@@ -1 +1 @@\n-" + before + "+" + after
	patchRoot := filepath.Join(dir, "patches", "tiny")
	if err := os.MkdirAll(patchRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(patchRoot, "0001-fix.patch"), []byte(patch), 0o644); err != nil {
		t.Fatal(err)
	}
	return Config{Archive: archive, ArchiveSHA256: hash(b.Bytes()), PatchRoot: dir, Work: filepath.Join(dir, "work"), GrammarDir: "src", Regenerate: source,
		Patches: []patches.Pin{{File: "patches/tiny/0001-fix.patch", SHA256: hash([]byte(patch)), UpstreamReport: "https://example.invalid/synthetic-test",
			Source: name, BeforeSHA256: hash([]byte(before)), AfterSHA256: hash([]byte(after))}}}
}

func TestRecordedPatch(t *testing.T) {
	c := fixture(t, false)
	root, prov, err := Prepare(c)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "src", "parser.c"))
	if err != nil || string(got) != "int b;\n" {
		t.Fatalf("patched C %q: %v", got, err)
	}
	if len(prov.Patches) != 1 || prov.Outputs["parser.c"] != hash(got) || prov.InputSHA256 == "" || !prov.Deterministic {
		t.Fatalf("missing provenance: %+v", prov)
	}
	if err := checkFile(c.Archive, c.ArchiveSHA256); err != nil {
		t.Fatal("original archive changed:", err)
	}
}

func TestPatchRejectsUnrecordedOrWrongInputs(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Config)
		want string
	}{
		{"archive", func(c *Config) { c.ArchiveSHA256 = strings.Repeat("0", 64) }, "sha256"},
		{"patch", func(c *Config) { c.Patches[0].SHA256 = strings.Repeat("0", 64) }, "sha256"},
		{"input", func(c *Config) { c.Patches[0].BeforeSHA256 = strings.Repeat("0", 64) }, "sha256"},
		{"output", func(c *Config) { c.Patches[0].AfterSHA256 = strings.Repeat("0", 64) }, "after sha256 mismatch"},
		{"path", func(c *Config) { c.Patches[0].File = "../outside.patch" }, "invalid grammar patch"},
		{"source-no-regenerate", func(c *Config) { c.Patches[0].Source = "grammar.js" }, "must set regenerate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fixture(t, false)
			tc.edit(&c)
			if _, _, err := Prepare(c); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestSyntheticGrammarRegeneration(t *testing.T) { testGrammarRegeneration(t, true) }

func TestUnpatchedGrammarRegeneration(t *testing.T) { testGrammarRegeneration(t, false) }

func testGrammarRegeneration(t *testing.T, patched bool) {
	archive := os.Getenv("TREE_SITTER_CLI_ARCHIVE")
	if archive == "" {
		t.Skip("set TREE_SITTER_CLI_ARCHIVE to the pinned host .gz for the real CLI integration test")
	}
	c := fixture(t, true)
	if !patched {
		c.Patches = nil
	}
	b, err := os.ReadFile(filepath.Join("..", "pins.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Tools struct {
			Version  string `json:"treeSitterCLI"`
			Archives map[string]struct {
				SHA256 string `json:"sha256"`
			} `json:"treeSitterCLIArchives"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	c.CLIArchive, c.CLIArchiveSHA256, c.CLIVersion = archive, p.Tools.Archives[runtime.GOOS+"-"+runtime.GOARCH].SHA256, p.Tools.Version
	if err := checkFile(archive, c.CLIArchiveSHA256); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	bin, err := io.ReadAll(z)
	if err != nil {
		t.Fatal(err)
	}
	c.CLI = filepath.Join(t.TempDir(), "tree-sitter")
	if runtime.GOOS == "windows" {
		c.CLI += ".exe"
	}
	if err := os.WriteFile(c.CLI, bin, 0o755); err != nil {
		t.Fatal(err)
	}
	root, prov, err := Prepare(c)
	if err != nil {
		t.Fatal(err)
	}
	parser, err := os.ReadFile(filepath.Join(root, "src", "parser.c"))
	if err != nil || !bytes.Contains(parser, []byte("tree_sitter_tiny")) || (bytes.Contains(parser, []byte("anon_sym_b")) != patched) {
		t.Fatalf("regenerated parser has wrong patch state: %v", err)
	}
	if len(prov.Patches) != len(c.Patches) || prov.InputSHA256 == "" || prov.CLI != c.CLIVersion || prov.JSRuntime != "native" || !prov.Deterministic || prov.Outputs["node-types.json"] == "" {
		t.Fatalf("incomplete generation provenance: %+v", prov)
	}
	// The second independent generation's entire tree has already been compared by Prepare.
	t.Logf("CLI=%s --js-runtime=%s input=%s parser.c=%s node-types.json=%s deterministic=%t", prov.CLI, prov.JSRuntime, prov.InputSHA256, prov.Outputs["parser.c"], prov.Outputs["node-types.json"], prov.Deterministic)
	bin[0] ^= 1
	if err := os.WriteFile(c.CLI, bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := verifyCLI(c); err == nil || !strings.Contains(err.Error(), "CLI differs") {
		t.Fatalf("modified CLI accepted: %v", err)
	}
}

func TestUnpatchedRegenerationRequiresCLI(t *testing.T) {
	c := fixture(t, true)
	c.Patches = nil
	if _, _, err := Prepare(c); err == nil || !strings.Contains(err.Error(), "pinned tree-sitter CLI") {
		t.Fatalf("unpatched regeneration bypassed CLI validation: %v", err)
	}
}
