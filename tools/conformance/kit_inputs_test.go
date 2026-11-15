package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCorpusKitInputs fails a kit grammar's shard when the kit input set did not load (the name matches the shard's
// -run TestCorpus pattern): a dropped -kit flag or an emptied manifest would otherwise leave every lane green.
func TestCorpusKitInputs(t *testing.T) {
	root := dataRoot(t)
	for _, lang := range langs(t) {
		if err := kitInputsMissing(root, lang, ciData(root)); err != nil {
			t.Error(err)
		}
	}
}

func TestKitInputsMissing(t *testing.T) {
	write := func(dir, name, body string) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	id := `{"kitManifestSHA256":"` + strings.Repeat("a", 64) + `"}`
	corpus := "id,lang\npython/kit-1,python\n"
	edits := "id,lang,a,b,c,d,e,f,kind\npython/kit-2,python,x,0,0,0,e,f,kit-pair\n"
	for _, tt := range []struct {
		name, lang, ci, corpus, edits string
		wantEdits, fail               bool
	}{
		{"loaded", "python", id, corpus, edits, true, false},
		{"no identity (flag dropped)", "python", `{"kitManifestSHA256":""}`, corpus, edits, true, true},
		{"no kit inputs", "python", id, "id,lang\npython/x-1,python\n", edits, true, true},
		{"no kit pairs", "python", id, corpus, "id,lang,a,b,c,d,e,f,kind\n", true, true},
		{"inputs-only needs no pairs", "python", id, corpus, "", false, false},
		{"non-kit grammar", "bash", `{}`, "id,lang\n", "", true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			write(dir, "ci.json", tt.ci)
			write(dir, "corpus/manifest.csv", tt.corpus)
			write(dir, "edits/manifest.csv", tt.edits)
			if err := kitInputsMissing(dir, tt.lang, tt.wantEdits); (err != nil) != tt.fail {
				t.Fatalf("fail=%v: %v", tt.fail, err)
			}
		})
	}
}
