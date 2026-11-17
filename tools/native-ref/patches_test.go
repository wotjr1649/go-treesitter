package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/go-treesitter/tools/gen/patches"
)

func TestUnpatchedRegenerationRequiresPinnedCLI(t *testing.T) {
	_, err := prepareGrammarPatches("pins.json", pins{}, source{Regenerate: true}, t.TempDir(), t.TempDir(), "tiny", false)
	if err == nil || !strings.Contains(err.Error(), "no tree-sitter CLI archive") {
		t.Fatalf("unpatched regeneration skipped CLI validation: %v", err)
	}
}

func TestNoCommittedParserReferenceFailsBeforePreparingInputs(t *testing.T) {
	for _, langs := range [][]string{{"swift"}, nil} {
		t.Run(fmt.Sprint(langs), func(t *testing.T) {
			root := t.TempDir()
			pinsFile := filepath.Join(root, "pins.json")
			if err := os.WriteFile(pinsFile, []byte(`{"grammars":{"swift":{"reference":"none","regenerate":true}}}`), 0644); err != nil {
				t.Fatal(err)
			}
			dl, work, out := filepath.Join(root, "dl"), filepath.Join(root, "work"), filepath.Join(root, "out")
			err := runMode(pinsFile, dl, work, out, "missing-compiler", "-O2", "", "", false, false, false, true, false, langs)
			if err == nil || !strings.Contains(err.Error(), "swift: reference none: NOT_APPLICABLE_NO_COMMITTED_PARSER") {
				t.Fatalf("missing reference silently accepted or attempted build: %v", err)
			}
			for _, dir := range []string{dl, work, out} {
				if _, err := os.Stat(dir); !os.IsNotExist(err) {
					t.Fatalf("reference rejection prepared %s: %v", dir, err)
				}
			}
		})
	}
}

func TestReferenceKeepsOnlyCPatchesWithoutChangingCandidate(t *testing.T) {
	all := []patches.Pin{{Source: "postgres/grammar.js"}, {Source: "postgres/src/scanner.c", BeforeSHA256: "original", GeneratedBeforeSHA256: "generated"}, {Source: "src/shared.h"}, {Source: "grammar.json"}}
	got := referenceCPatches(all)
	if len(got) != 2 || got[0] != all[1] || got[1] != all[2] {
		t.Fatalf("wrong reference patches: %+v", got)
	}
	got[0].Source = "changed"
	if all[1].Source != "postgres/src/scanner.c" || len(all) != 4 {
		t.Fatal("reference selection changed candidate patches")
	}
}

func TestCommittedReferenceSkipsGrammarPatchAndKeepsScannerPatch(t *testing.T) {
	root := t.TempDir()
	write := func(name string, data []byte) string {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, data, 0644); err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("%x", sha256.Sum256(data))
	}
	files := map[string]string{"src/parser.c": "committed parser\n", "src/scanner.c": "old scanner\n", "src/grammar.js": "old grammar\n", "lib/runtime.c": "runtime\n"}
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: "upstream/" + name, Mode: 0644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	dl := filepath.Join(root, "dl")
	archiveHash := write(filepath.Join(dl, "tiny.tar.gz"), archive.Bytes())
	var recorded []patches.Pin
	for _, item := range []struct{ source, before, after string }{
		{"src/grammar.js", "old grammar\n", "new grammar\n"},
		{"src/scanner.c", "old scanner\n", "new scanner\n"},
	} {
		patch := fmt.Sprintf("--- a/%s\n+++ b/%s\n@@ -1 +1 @@\n-%s+%s", item.source, item.source, item.before, item.after)
		name := "patches/tiny/" + filepath.Base(item.source) + ".patch"
		recorded = append(recorded, patches.Pin{File: name, Source: item.source,
			SHA256:       write(filepath.Join(root, filepath.FromSlash(name)), []byte(patch)),
			BeforeSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(item.before))),
			AfterSHA256:  fmt.Sprintf("%x", sha256.Sum256([]byte(item.after))), UpstreamReport: "https://example.invalid/issue/1"})
	}
	g := source{Archive: "tiny.tar.gz", ArchiveSHA256: archiveHash, ExtractDir: "tiny", GrammarDir: "src", Dirs: []string{"src"}, Regenerate: true, Patches: recorded,
		Files: map[string]string{"parser.c": fmt.Sprintf("%x", sha256.Sum256([]byte(files["src/parser.c"]))), "scanner.c": recorded[1].BeforeSHA256}}
	p := pins{Runtime: source{Archive: g.Archive, ArchiveSHA256: archiveHash, ExtractDir: "runtime", Dirs: []string{"lib"}}, Grammars: map[string]source{"tiny": g}}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	pinsFile := filepath.Join(root, "pins.json")
	write(pinsFile, data)
	work := filepath.Join(root, "reference")
	if err := runMode(pinsFile, dl, work, "", "missing-compiler", "-O2", "", "", false, true, false, true, false, []string{"tiny"}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"parser.c": files["src/parser.c"], "grammar.js": files["src/grammar.js"], "scanner.c": "new scanner\n"} {
		got, err := os.ReadFile(filepath.Join(work, "tiny", "src", name))
		if err != nil || string(got) != want {
			t.Fatalf("reference %s = %q, %v; want %q", name, got, err, want)
		}
	}
	// Default preparation must still require regeneration rather than silently using reference C.
	if err := run(pinsFile, dl, filepath.Join(root, "default"), "", "missing-compiler", "-O2", "", "", false, true, false, []string{"tiny"}); err == nil || !strings.Contains(err.Error(), "no tree-sitter CLI archive") {
		t.Fatalf("default preparation skipped regeneration: %v", err)
	}
}
