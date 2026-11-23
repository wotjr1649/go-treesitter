package main

import (
	"encoding/json"
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

func TestUnpatchedRegenerationIsPrepared(t *testing.T) {
	o := opts{}
	_, err := preparePatches(&o, &Pins{}, GrammarPin{Regenerate: true}, t.TempDir(), &Identity{})
	if err == nil || !strings.Contains(err.Error(), "-dl is required") {
		t.Fatalf("unpatched regeneration skipped: %v", err)
	}
}

func TestGeneratorExceptionPins(t *testing.T) {
	p, err := loadPins("pins.json")
	if err != nil {
		t.Fatal(err)
	}
	g := p.Grammars["csharp"]
	for _, tc := range []struct {
		name       string
		regenerate bool
		exception  *GeneratorException
	}{
		{"missing", false, nil},
		{"reason", false, &GeneratorException{FailingInputs: []string{"input"}}},
		{"inputs", false, &GeneratorException{Reason: "why"}},
		{"regenerated-exception", true, &GeneratorException{Reason: "why", FailingInputs: []string{"input"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clone := *p
			clone.Grammars = map[string]GrammarPin{"csharp": g}
			changed := g
			changed.Regenerate = tc.regenerate
			changed.GeneratorException = tc.exception
			clone.Grammars["csharp"] = changed
			b, err := json.Marshal(clone)
			if err != nil {
				t.Fatal(err)
			}
			name := filepath.Join(t.TempDir(), "pins.json")
			if err := os.WriteFile(name, b, 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := loadPins(name); err == nil {
				t.Fatal("invalid schema-3 exception accepted")
			}
		})
	}
}

func TestReferenceModePins(t *testing.T) {
	p, err := loadPins("pins.json")
	if err != nil {
		t.Fatal(err)
	}
	base := p.Grammars["swift"]
	if base.Reference != "none" || len(p.Grammars) != 25 {
		t.Fatalf("Swift reference or grammar count drifted: %q, %d", base.Reference, len(p.Grammars))
	}
	for _, tc := range []struct {
		name string
		edit func(*GrammarPin)
		want string
	}{
		{"none", func(g *GrammarPin) {}, ""},
		{"committed", func(g *GrammarPin) { g.Reference = "committed" }, ""},
		{"omitted", func(g *GrammarPin) { g.Reference = "" }, ""},
		{"unknown", func(g *GrammarPin) { g.Reference = "typo" }, "unknown reference mode"},
		{"unknown-input", func(g *GrammarPin) { g.GenerateFrom = "../grammar.json" }, "generateFrom"},
		{"requires-generation", func(g *GrammarPin) { g.Regenerate = false }, "reference none requires"},
		{"forbids-parser-pin", func(g *GrammarPin) { g.Files = map[string]string{"parser.c": ""} }, "reference none requires"},
		{"exception-ok", func(g *GrammarPin) {}, ""},
		{"exception-missing-reason", func(g *GrammarPin) {
			x := *g.UpstreamTestException
			x.Reason = ""
			g.UpstreamTestException = &x
		}, "invalid upstream test exception"},
		{"exception-missing-retire", func(g *GrammarPin) {
			x := *g.UpstreamTestException
			x.RetireWhen = ""
			g.UpstreamTestException = &x
		}, "invalid upstream test exception"},
		{"exception-wrong-cli", func(g *GrammarPin) {
			x := *g.UpstreamTestException
			x.CLI = "0.0.0"
			g.UpstreamTestException = &x
		}, "invalid upstream test exception"},
		{"exception-wrong-step", func(g *GrammarPin) {
			x := *g.UpstreamTestException
			x.Step = "other"
			g.UpstreamTestException = &x
		}, "invalid upstream test exception"},
		{"forbids-exception", func(g *GrammarPin) {
			g.GeneratorException = &GeneratorException{Reason: "reason", FailingInputs: []string{"case"}}
		}, "reference none requires"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := base
			tc.edit(&g)
			clone := *p
			clone.Grammars = map[string]GrammarPin{"swift": g}
			b, err := json.Marshal(clone)
			if err != nil {
				t.Fatal(err)
			}
			name := filepath.Join(t.TempDir(), "pins.json")
			if err := os.WriteFile(name, b, 0644); err != nil {
				t.Fatal(err)
			}
			_, err = loadPins(name)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestUpstreamTestExceptionSwiftOnly(t *testing.T) {
	p, err := loadPins("pins.json")
	if err != nil {
		t.Fatal(err)
	}
	clone := *p
	clone.Grammars = map[string]GrammarPin{"c": p.Grammars["swift"]}
	b, err := json.Marshal(clone)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(t.TempDir(), "pins.json")
	if err := os.WriteFile(name, b, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPins(name); err == nil || !strings.Contains(err.Error(), "invalid upstream test exception") {
		t.Fatalf("want invalid upstream test exception, got %v", err)
	}
}

// The generator gate accepts the Swift exception only for CLI 0.27.2, so moving the
// CLI pin and the exception together must not make loadPins accept a stale exception.
func TestUpstreamTestExceptionCLIValue(t *testing.T) {
	p, err := loadPins("pins.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, cli := range []string{"0.27.3", ""} {
		clone := *p
		clone.Tools.TreeSitterCLI = cli
		g := clone.Grammars["swift"]
		x := *g.UpstreamTestException
		x.CLI = cli
		g.UpstreamTestException = &x
		clone.Grammars = map[string]GrammarPin{"swift": g}
		b, err := json.Marshal(clone)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(t.TempDir(), "pins.json")
		if err := os.WriteFile(name, b, 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := loadPins(name); err == nil || !strings.Contains(err.Error(), "invalid upstream test exception") {
			t.Fatalf("cli %q: want invalid upstream test exception, got %v", cli, err)
		}
	}
}
