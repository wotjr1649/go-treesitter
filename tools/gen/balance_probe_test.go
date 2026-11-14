package main

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBalanceProbeOutputBoundary(t *testing.T) {
	repo := t.TempDir()
	base := opts{repo: repo, out: filepath.Join(repo, "scratch", "json"), work: filepath.Join(repo, "work")}
	if err := validateBalanceProbe(base); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, out, work, version string }{
		{"product output", filepath.Join(repo, "grammars", "json"), base.work, ""},
		{"product work", base.out, filepath.Join(repo, "grammars", "json"), ""},
		{"release", base.out, base.work, "v0.1.0"},
		{"missing output", "", base.work, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := base
			o.out, o.work, o.rootVersion = tc.out, tc.work, tc.version
			if err := validateBalanceProbe(o); err == nil {
				t.Fatal("unsafe probe output accepted")
			}
		})
	}
	if err := os.MkdirAll(base.out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base.out, "keep"), []byte("unrelated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateBalanceProbe(base); err == nil {
		t.Fatal("nonempty probe output accepted")
	}
}

func TestBalanceProbeFiles(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "한글 repo")
	files, err := balanceProbeFiles(opts{repo: repo, lang: "json"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "module_test.go", files["module_test.go"], 0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(files["go.mod"]), `=> "`+filepath.ToSlash(repo)+`"`) {
		t.Fatalf("local path not quoted: %s", files["go.mod"])
	}
	product, err := marshal(provenance{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(product), "testOnly") {
		t.Fatal("test-only marker changed product provenance")
	}
	probe, err := marshal(provenance{TestOnly: "balancing-cancel"})
	if err != nil || !json.Valid(probe) || !strings.Contains(string(probe), `"testOnly": "balancing-cancel"`) {
		t.Fatalf("probe marker missing: %s %v", probe, err)
	}
}
