package main

import (
	"github.com/wotjr1649/go-treesitter/tools/gen/patches"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchedGrammarPreservesVerifiedSource(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src", "grammar", "src")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatal(err)
	}
	before := []byte("a\nb\n")
	after := []byte("a\nc\n")
	if err := os.WriteFile(filepath.Join(src, "parser.c"), before, 0644); err != nil {
		t.Fatal(err)
	}
	patch := []byte("--- a/src/parser.c\n+++ b/src/parser.c\n@@ -1,2 +1,2 @@\n a\n-b\n+c\n")
	patchDir := filepath.Join(root, "generator", "patches", "test")
	if err := os.MkdirAll(patchDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(patchDir, "0001-fix.patch"), patch, 0644); err != nil {
		t.Fatal(err)
	}
	pin := patches.Pin{File: "patches/test/0001-fix.patch", Source: "src/parser.c", SHA256: sumBytes(patch), BeforeSHA256: sumBytes(before), AfterSHA256: sumBytes(after), UpstreamReport: "https://example.com/issues/1"}
	g := GrammarPin{ExtractDir: "grammar", GrammarDir: "src", Dirs: []string{"src"}, Patches: []patches.Pin{pin}}
	id := Identity{SourceSHA256: map[string]string{}}
	out, err := patchedGrammar(opts{src: filepath.Join(root, "src"), root: filepath.Join(root, "generator")}, g, root, &id)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(filepath.Join(src, "parser.c"))
	got, _ := os.ReadFile(filepath.Join(out, "src", "parser.c"))
	if string(original) != string(before) || string(got) != string(after) {
		t.Fatalf("original=%q patched=%q", original, got)
	}
	if id.SourceSHA256["patch."+pin.File] != pin.SHA256 || id.SourceSHA256["patched."+pin.Source] != pin.AfterSHA256 {
		t.Fatal("patch identity missing")
	}
	readmeText := readme("test", &Pins{}, g, id)
	if !strings.Contains(readmeText, "differs temporarily from upstream") || !strings.Contains(readmeText, pin.UpstreamReport) {
		t.Fatal("patch disclosure missing")
	}
	if !strings.Contains(readme("yaml", &Pins{}, g, id), "premature end-of-input acceptance and silent source truncation") {
		t.Fatal("YAML patch rationale missing")
	}
}

func TestPatchDisclosureWithNotice(t *testing.T) {
	p := patches.Pin{File: "patches/test/0001.patch", Source: "src/parser.c", UpstreamReport: "https://example.invalid/issues/1"}
	s := readme("tsql", &Pins{}, GrammarPin{Patches: []patches.Pin{p}, NoticeSHA256: "notice"}, Identity{})
	if !strings.Contains(s, "differs temporarily from upstream") || !strings.Contains(s, p.UpstreamReport) || !strings.Contains(s, "`NOTICE`") {
		t.Fatal("patch disclosure or NOTICE missing")
	}
}

func TestPatchStatusThreshold(t *testing.T) {
	if strings.Contains(patchStatus("tiny", 3, 3), "WARNING") || !strings.Contains(patchStatus("tiny", 4, 3), "WARNING") || !strings.Contains(patchStatus("tiny", 0, 0), "patches=0") {
		t.Fatal("patch count threshold")
	}
}

func TestGrammarRegenerationPlan(t *testing.T) {
	p, err := loadPins("pins.json")
	if err != nil {
		t.Fatal(err)
	}
	g := p.Grammars["json"]
	g.Regenerate = true
	p.Grammars["json"] = g
	lines, err := p.plan("windows-amd64")
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "cli\t") {
			fields := strings.Split(line, "\t")
			if len(fields) != 6 || fields[1] != "tree-sitter-windows-x64.gz" || fields[3] != p.Tools.TreeSitterCLIArchives["windows-amd64"].SHA256 || fields[4] != "bin/tree-sitter.exe" {
				t.Fatalf("bad CLI plan: %s", line)
			}
			found++
		}
	}
	if found != 1 {
		t.Fatalf("CLI plan entries=%d", found)
	}
}
