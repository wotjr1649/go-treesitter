package main

import (
	"strings"
	"testing"
)

const fieldMapC = `static const TSMapSlice ts_field_map_slices[PRODUCTION_ID_COUNT] = {
  [3] = {.index = 0, .length = 1},
  [4] = {.index = 1, .length = 2},
};

static const TSFieldMapEntry ts_field_map_entries[] = {
  [0] =
    {field_name, 0},
  [1] =
    {field_left, 0},
    {field_name, 1, .inherited = true},
};

static const TSSymbol other[] = {
  {field_ignored, 9},
};
`

func TestFieldMapSummary(t *testing.T) {
	slices, entries := fieldMapOf([]byte(fieldMapC))
	if len(slices) != 2 || strings.Join(entries, " ") != "field_name,0 field_left,0 field_name,1,inherited" {
		t.Fatalf("parsed %q %q", slices, entries)
	}
	if f := fieldMapSummary([]byte(fieldMapC), []byte(fieldMapC)); f.Assessment != "identical" || f.Entries != [2]int{3, 3} || f.text() != "identical" {
		t.Fatalf("same tables: %+v", f)
	}
	// The candidate inherits one more field through a supertype: one more inherited entry and slice.
	cand := strings.Replace(fieldMapC, "    {field_name, 1, .inherited = true},\n", "    {field_name, 1, .inherited = true},\n    {field_right, 2, .inherited = true},\n", 1)
	cand = strings.Replace(cand, "  [4] = {.index = 1, .length = 2},\n", "  [4] = {.index = 1, .length = 3},\n", 1)
	f := fieldMapSummary([]byte(fieldMapC), []byte(cand))
	if f.Assessment != "differs" || f.Entries != [2]int{3, 4} || f.Inherited != [2]int{1, 2} || f.Fields["field_right (inherited)"] != [2]int{0, 1} || len(f.Fields) != 1 {
		t.Fatalf("one more inherited entry: %+v", f)
	}
	if got := f.text(); !strings.Contains(got, "entries 3→4") || !strings.Contains(got, "inherited 1→2") {
		t.Fatalf("cell %q", got)
	}
	// Same counts but another child index is still a difference; a grammar without fields has no tables on either side.
	moved := strings.Replace(fieldMapC, "{field_left, 0}", "{field_left, 1}", 1)
	if f := fieldMapSummary([]byte(fieldMapC), []byte(moved)); f.Assessment != "differs" || len(f.Fields) != 0 {
		t.Fatalf("moved child index: %+v", f)
	}
	if f := fieldMapSummary([]byte("int x;"), []byte("int y;")); f.Assessment != "identical" || f.Entries != [2]int{} {
		t.Fatalf("no fields: %+v", f)
	}
	if (*rehearsalFieldMap)(nil).text() != "-" {
		t.Fatal("nil cell")
	}
}

func TestReleaseProblems(t *testing.T) {
	tag := strings.Repeat("a", 40)
	m := &nightlyMeta{TagCommitBefore: tag, TagCommitAfter: tag}
	if p := releaseProblems(m, "tree-sitter 0.28.0", "0.28.0"); len(p) != 0 {
		t.Fatalf("consistent release: %v", p)
	}
	if p := releaseProblems(m, "tree-sitter 0.27.2", "0.28.0"); len(p) != 1 {
		t.Fatalf("wrong version: %v", p)
	}
	if p := releaseProblems(&nightlyMeta{TagCommitBefore: tag, TagCommitAfter: strings.Repeat("b", 40), TagMoved: true}, "tree-sitter 0.28.0", "0.28.0"); len(p) != 1 {
		t.Fatalf("moved tag: %v", p)
	}
	if p := assetProblemsUnder(cliAssetBase+"v0.28.0/", &nightlyMeta{URL: cliAssetBase + "v0.28.0/x.gz", Size: 1, APIDigest: "sha256:a"}, 1, "a"); len(p) != 0 {
		t.Fatalf("asset under the release directory: %v", p)
	}
	if p := assetProblemsUnder(cliAssetBase+"v0.28.0/", &nightlyMeta{URL: nightlyAssetPrefix + "x.gz", Size: 1, APIDigest: "sha256:a"}, 1, "a"); len(p) != 1 {
		t.Fatalf("asset outside the release directory: %v", p)
	}
}
