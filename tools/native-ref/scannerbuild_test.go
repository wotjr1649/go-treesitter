package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wotjr1649/go-treesitter/tools/gen/patches"
)

func TestScannerBuildPatchStages(t *testing.T) {
	g := source{Regenerate: true, GrammarDir: "sub/src", Files: map[string]string{"scanner.c": "pinned"},
		Patches: []patches.Pin{
			{File: "parser", Source: "sub/src/parser.c"},
			{File: "scanner1", Source: "sub/src/scanner.c"},
			{File: "grammar", Source: "sub/grammar.js"},
			{File: "scanner2", Source: "sub/src/scanner.c"},
			{File: "header", Source: "sub/src/header.h"},
		}}
	post := referenceCPatches(g.Patches)
	s := scannerBuild{dir: t.TempDir(), Grammar: "tiny", final: len(post), stages: map[int]scannerStage{}}
	if err := s.selectStages(g, post); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.pairs, []int{1, 2}) || len(s.stages) != 3 {
		t.Fatalf("wrong scanner stage indices: %v (%v)", s.pairs, s.stages)
	}
	for idx := range s.stages {
		id := string(rune('0' + idx))
		s.stages[idx] = scannerStage{exe: id, exeSHA: "exe" + id, sourcesSHA: "src" + id,
			files: map[string]string{"parser.c": "parser", "scanner.c": "scanner" + id}}
	}
	s.stages[3].files["parser.c"] = "changed"
	if err := s.finish("", ""); err == nil || !strings.Contains(err.Error(), "also changes parser.c") {
		t.Fatalf("accepted a non-scanner change between stages: %v", err)
	}
	s.stages[3].files["parser.c"] = "parser"
	if err := s.finish("", ""); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(s.dir, "tiny.json"))
	if err != nil {
		t.Fatal(err)
	}
	var built scannerBuild
	if err := json.Unmarshal(raw, &built); err != nil {
		t.Fatal(err)
	}
	first, second := built.Comparisons[0], built.Comparisons[1]
	if first.Patch != "scanner1" || second.Patch != "scanner2" ||
		first.Before != "1" || first.After != "2" || second.Before != "2" || second.After != "3" ||
		first.AfterSourcesSHA256 != second.BeforeSourcesSHA256 {
		t.Fatalf("wrong comparison manifest: %+v", built.Comparisons)
	}
}
