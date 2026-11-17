package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
