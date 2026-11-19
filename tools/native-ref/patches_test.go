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
	"runtime"
	"strings"
	"testing"

	"github.com/wotjr1649/go-treesitter/tools/gen/patches"
)

const referenceTestCLIVersion = "0.0.0"

// The copied test executable supplies deterministic CLI output without a download.
func TestMain(m *testing.M) {
	if parser, ok := os.LookupEnv("TSG_NATIVE_REF_TEST_PARSER"); ok && len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version":
			fmt.Println("tree-sitter", referenceTestCLIVersion)
			os.Exit(0)
		case "generate":
			path := filepath.Join("src", "parser.c")
			var err error
			if parser == "" {
				err = os.Remove(path)
			} else {
				err = os.WriteFile(path, []byte(parser), 0o644)
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}

func TestUnpatchedRegenerationRequiresPinnedCLI(t *testing.T) {
	_, err := prepareGrammarPatches("pins.json", pins{}, source{Regenerate: true}, t.TempDir(), t.TempDir(), "tiny", false, nil)
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
			err := runMode(pinsFile, dl, work, out, "missing-compiler", "-O2", "", "", false, false, false, true, false, "", langs)
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
	files := map[string]string{"src/parser.c": "committed parser\n", "src/scanner.c": "old scanner\n", "src/grammar.js": "old grammar\n", "lib/runtime.c": "runtime\n"}
	dl := filepath.Join(root, "dl")
	archiveHash := writeReferenceTestArchive(t, filepath.Join(dl, "tiny.tar.gz"), files)
	recorded := []patches.Pin{
		writeReferenceTestPatch(t, root, "src/grammar.js", files["src/grammar.js"], "new grammar\n"),
		writeReferenceTestPatch(t, root, "src/scanner.c", files["src/scanner.c"], "new scanner\n"),
	}
	g := source{Archive: "tiny.tar.gz", ArchiveSHA256: archiveHash, ExtractDir: "tiny", GrammarDir: "src", Dirs: []string{"src"}, Regenerate: true, Patches: recorded,
		Files: map[string]string{"parser.c": referenceTestHash(files["src/parser.c"]), "scanner.c": recorded[1].BeforeSHA256}}
	p := pins{Runtime: source{Archive: g.Archive, ArchiveSHA256: archiveHash, ExtractDir: "runtime", Dirs: []string{"lib"}}, Grammars: map[string]source{"tiny": g}}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	pinsFile := filepath.Join(root, "pins.json")
	writeReferenceTestFile(t, pinsFile, data)
	work := filepath.Join(root, "reference")
	if err := runMode(pinsFile, dl, work, "", "missing-compiler", "-O2", "", "", false, true, false, true, false, "", []string{"tiny"}); err != nil {
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

func TestLFSReferenceRegeneration(t *testing.T) {
	const upstream = "upstream parser\n"
	const patched = "patched parser\n"
	const grammar = "upstream grammar\n"
	oid := referenceTestHash(upstream)
	pointer := fmt.Sprintf("version https://git-lfs.github.com/spec/v1\noid sha256:%s\nsize %d\n", oid, len(upstream))
	root := t.TempDir()
	dl := filepath.Join(root, "dl")
	archiveHash := writeReferenceTestArchive(t, filepath.Join(dl, "tiny.tar.gz"), map[string]string{
		"src/parser.c": pointer, "src/grammar.js": grammar, "lib/runtime.c": "runtime\n",
	})
	parserPatch := writeReferenceTestPatch(t, root, "src/parser.c", upstream, patched)
	parserPatch.GeneratedBeforeSHA256, parserPatch.GeneratedAfterSHA256 = parserPatch.BeforeSHA256, parserPatch.AfterSHA256
	parserPatch.BeforeSHA256 = referenceTestHash(pointer)
	g := source{Archive: "tiny.tar.gz", ArchiveSHA256: archiveHash, ExtractDir: "tiny", GrammarDir: "src", Dirs: []string{"src"},
		Files:   map[string]string{"parser.c": referenceTestHash(pointer)},
		Patches: []patches.Pin{writeReferenceTestPatch(t, root, "src/grammar.js", grammar, "patched grammar\n"), parserPatch}}
	p := pins{Runtime: source{Archive: g.Archive, ArchiveSHA256: archiveHash, ExtractDir: "runtime", Dirs: []string{"lib"}}, Grammars: map[string]source{"tiny": g}}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cli, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	const cliArchive = "tree-sitter.gz"
	p.Tools.TreeSitterCLI = referenceTestCLIVersion
	p.Tools.TreeSitterCLIArchives = map[string]struct {
		URL    string `json:"url"`
		SHA256 string `json:"sha256"`
	}{runtime.GOOS + "-" + runtime.GOARCH: {URL: "https://example.invalid/" + cliArchive, SHA256: writeReferenceTestGzip(t, filepath.Join(dl, cliArchive), cli)}}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	pinsFile := filepath.Join(root, "pins.json")
	writeReferenceTestFile(t, pinsFile, data)

	const differing = "different parser\n"
	for _, tc := range []struct {
		name, generated, wantParser, wantError string
		unpatched                              bool
	}{
		{name: "matching", generated: upstream, wantParser: patched},
		{name: "unpatched", generated: upstream, wantParser: upstream, unpatched: true},
		{name: "missing", wantError: "generated parser.c missing"},
		{name: "mismatching", generated: differing, wantParser: differing,
			wantError: "regenerated LFS reference sha256 " + referenceTestHash(differing) + ", committed OID " + oid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TSG_NATIVE_REF_TEST_PARSER", tc.generated)
			work := t.TempDir()
			err := runMode(pinsFile, dl, work, "", "missing-compiler", "-O2", "", "", false, true, tc.unpatched, true, false, "", []string{"tiny"})
			if (tc.wantError == "" && err != nil) || (tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError))) {
				t.Fatalf("error = %v, want %q", err, tc.wantError)
			}
			dir := filepath.Join(work, "patches-tiny", "first", "src")
			if got, err := os.ReadFile(filepath.Join(dir, "grammar.js")); err != nil || string(got) != grammar {
				t.Fatalf("reference grammar.js = %q, %v; want %q", got, err, grammar)
			}
			got, err := os.ReadFile(filepath.Join(dir, "parser.c"))
			if tc.wantParser == "" {
				if !os.IsNotExist(err) {
					t.Fatalf("missing reference parser.c = %q, %v", got, err)
				}
			} else if err != nil || string(got) != tc.wantParser {
				t.Fatalf("reference parser.c = %q, %v; want %q", got, err, tc.wantParser)
			}
		})
	}
}

func referenceTestHash(data string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
}

func writeReferenceTestFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func writeReferenceTestArchive(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: "upstream/" + name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return writeReferenceTestGzip(t, name, archive.Bytes())
}

func writeReferenceTestGzip(t *testing.T, name string, data []byte) string {
	t.Helper()
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	if _, err := gz.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return writeReferenceTestFile(t, name, archive.Bytes())
}

func writeReferenceTestPatch(t *testing.T, root, source, before, after string) patches.Pin {
	t.Helper()
	patch := fmt.Sprintf("--- a/%s\n+++ b/%s\n@@ -1 +1 @@\n-%s+%s", source, source, before, after)
	name := "patches/tiny/" + filepath.Base(source) + ".patch"
	return patches.Pin{File: name, Source: source,
		SHA256:       writeReferenceTestFile(t, filepath.Join(root, filepath.FromSlash(name)), []byte(patch)),
		BeforeSHA256: referenceTestHash(before), AfterSHA256: referenceTestHash(after), UpstreamReport: "https://example.invalid/issue/1"}
}
