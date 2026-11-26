package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestFieldsDigest writes one line per input, "<lang>\t<input id>\t<sha256 input>\t<sha256 Fields(snapshot)>", to
// $TSG_FIELDS_DIGEST_OUT. It asserts nothing: tools/gen/field-precheck-soundness.sh runs it twice, once against the
// product grammar modules and once (go test -modfile) against modules built with tools/gen -field-full-compare, and
// diffs the files. Inputs: the corpus rows plus the edited inputs (data/edits/<lang>/*.new), the same sets as TestFieldLookup
// and TestFieldLookupEdits, so a difference points at a place those gates do compare with C.
func TestFieldsDigest(t *testing.T) {
	out := os.Getenv("TSG_FIELDS_DIGEST_OUT")
	if out == "" {
		t.Skip("TSG_FIELDS_DIGEST_OUT unset")
	}
	root := dataRoot(t)
	rows := readCSV(t, filepath.Join(root, "corpus", "manifest.csv"))
	var sb strings.Builder
	for _, lang := range langs(t) {
		p := newParser(t, lang)
		do := func(id, path string) {
			src := readFile(t, path)
			snap, err := p.ParseSnapshot(bg, src)
			if err != nil {
				t.Fatalf("%s/%s: ParseSnapshot: %v", lang, id, err)
			}
			fmt.Fprintf(&sb, "%s\t%s\t%s\t%s\n", lang, id, sha(src), sha(Fields(snap)))
		}
		for _, r := range rows {
			if r[1] == lang {
				do(r[0], corpusFile(t, root, lang, filepath.Base(r[0])))
			}
		}
		edits, _ := filepath.Glob(filepath.Join(root, "edits", lang, "*.new"))
		sort.Strings(edits)
		for _, f := range edits {
			do("edit/"+filepath.Base(f), f)
		}
	}
	if err := os.WriteFile(out, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}
