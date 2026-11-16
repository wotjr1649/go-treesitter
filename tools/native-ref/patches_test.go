package main

import (
	"strings"
	"testing"
)

func TestUnpatchedRegenerationRequiresPinnedCLI(t *testing.T) {
	_, err := prepareGrammarPatches("pins.json", pins{}, source{Regenerate: true}, t.TempDir(), t.TempDir(), "tiny", false)
	if err == nil || !strings.Contains(err.Error(), "no tree-sitter CLI archive") {
		t.Fatalf("unpatched regeneration skipped CLI validation: %v", err)
	}
}
