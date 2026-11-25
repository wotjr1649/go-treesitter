package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// buildFake compiles internal/fakeworker once per adapter name into dir.
func buildFake(t *testing.T, dir string) {
	t.Helper()
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	first := filepath.Join(dir, "product"+exe)
	if out, err := exec.Command("go", "build", "-o", first, "../../internal/fakeworker").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	raw, _ := os.ReadFile(first)
	for _, a := range adapterOrder[1:] {
		if err := os.WriteFile(filepath.Join(dir, a+exe), raw, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// ciconfData writes the slice of a ciconf data directory that select reads: 40 inputs (30 clean, 10 with errors).
func ciconfData(t *testing.T, root string) {
	t.Helper()
	var corpus, edits strings.Builder
	corpus.WriteString("id,lang,source_ref,bytes,sha256,corpus_file,test_name,attrs\n")
	edits.WriteString("id,lang,base_ref,start,old_end,new_end,old_sha256,new_sha256,kind,seed\n")
	for i := 0; i < 40; i++ {
		src := []byte(fmt.Sprintf("int main_%03d(void) { return identifier_%d + 1; }\nint other_function_name = %d;\n", i, i, i))
		name := fmt.Sprintf("t-%03d", i+1)
		id := "c/" + name
		mustWrite(t, filepath.Join(root, "corpus", "c", name+".in"), src)
		fmt.Fprintf(&corpus, "%s,c,x#1,%d,%s,x.txt,t,\n", id, len(src), sha(src))
		hdr := "#tsg-dump/1 root_has_error=0 nodes=3\n"
		if i%4 == 3 {
			hdr = "#tsg-dump/1 root_has_error=1 nodes=3\n"
		}
		mustWrite(t, filepath.Join(root, "native", "dumps", "c", name+".dump"), []byte(hdr))
		nw := append([]byte{}, src...)
		nw[4] = 'X' // equal-length replace at byte 4
		mustWrite(t, filepath.Join(root, "edits", "c", name+"-e3.old"), src)
		mustWrite(t, filepath.Join(root, "edits", "c", name+"-e3.new"), nw)
		fmt.Fprintf(&edits, "%s-e3,c,%s,4,5,5,%s,%s,replace,1\n", id, id, sha(src), sha(nw))
	}
	mustWrite(t, filepath.Join(root, "corpus", "manifest.csv"), []byte(corpus.String()))
	mustWrite(t, filepath.Join(root, "edits", "manifest.csv"), []byte(edits.String()))
}

func mustWrite(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := writeFileAll(p, b); err != nil {
		t.Fatal(err)
	}
}

func TestSelectFreezesWorkload(t *testing.T) {
	dir := t.TempDir()
	data, work := filepath.Join(dir, "data"), filepath.Join(dir, "work")
	ciconfData(t, data)
	w, err := selectWorkload(selectOpts{data: data, out: work, langs: []string{"c"}, seed: "t"})
	if err != nil {
		t.Fatal(err)
	}
	clean, bad := 0, 0
	for _, in := range w.Inputs {
		if *in.Clean {
			clean++
		} else {
			bad++
		}
		if in.Token == nil || !in.Token.SamePts || in.Token.OldEnd != in.Token.NewEnd {
			t.Fatalf("%s: token edit %+v", in.ID, in.Token)
		}
	}
	if clean != 18 || bad != 6 || len(w.Inputs) != 24 {
		t.Fatalf("clean=%d errors=%d total=%d shortages=%v", clean, bad, len(w.Inputs), w.Shortages)
	}
	newline := 0
	for _, in := range w.Inputs {
		for _, e := range in.Edits {
			if e.Kind == "newline" {
				newline++
				if e.SamePts {
					t.Fatal("newline edit keeps the end point")
				}
			}
		}
	}
	if newline != 1 {
		t.Fatalf("newline edits %d", newline)
	}
	for i := 1; i < len(w.Inputs); i++ { // sorted by sha within clean/error groups: the same call again picks the same set
	}
	w2, _ := selectWorkload(selectOpts{data: data, out: filepath.Join(dir, "work2"), langs: []string{"c"}, seed: "t"})
	for i := range w.Inputs {
		if w.Inputs[i].ID != w2.Inputs[i].ID || w.Inputs[i].Token.NewSHA256 != w2.Inputs[i].Token.NewSHA256 {
			t.Fatal("selection is not deterministic")
		}
	}
}

func TestEndToEndWithFakeWorkers(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a helper binary")
	}
	dir := t.TempDir()
	data, work, bin, res, refs := filepath.Join(dir, "data"), filepath.Join(dir, "work"), filepath.Join(dir, "bin"), filepath.Join(dir, "res"), filepath.Join(dir, "refs")
	ciconfData(t, data)
	buildFake(t, bin)
	w, err := selectWorkload(selectOpts{data: data, out: work, langs: []string{"c"}, seed: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(work, "workload.json"), w); err != nil {
		t.Fatal(err)
	}
	// native references: the fake worker itself answers sexp/edit-sexp; malivvan and bonsai have none (diagnostic fallback)
	for _, a := range []string{"product", "official", "smacker", "gotreesitter"} {
		src, _ := os.ReadFile(filepath.Join(bin, "product"+exeSuffix()))
		mustWrite(t, filepath.Join(refs, a, "c"+exeSuffix()), src)
		_ = os.Chmod(filepath.Join(refs, a, "c"+exeSuffix()), 0o755)
	}
	if err := run(runOpts{workload: work, bin: bin, out: res, langs: []string{"c"}, adapters: adapterOrder, smoke: true,
		lRounds: 1, lReps: 2, sRounds: 1, sReps: 2, rssRuns: 1, budget: time.Minute, perOp: 20 * time.Second, seed: "t"}); err != nil {
		t.Fatal(err)
	}
	if err := cmdCheck([]string{"-workload", work, "-bin", bin, "-refs", refs, "-out", res, "-langs", "c", "-smoke", "-j", "2"}); err != nil {
		t.Fatal(err)
	}
	if err := cmdReport([]string{"-in", res}); err != nil {
		t.Fatal(err)
	}
	col := filepath.Join(dir, "collected")
	if err := cmdCollect([]string{"-in", res, "-out", col}); err != nil {
		t.Fatal(err)
	}
	summary, _ := os.ReadFile(filepath.Join(res, "summary.md"))
	corr, _ := os.ReadFile(filepath.Join(res, "correctness.csv"))
	s, cs := string(summary), string(corr)
	for _, want := range []string{"## Cell coverage", "| official | parse |", "| bonsai | snapshot |", "unsupported"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary lacks %q", want)
		}
	}
	// smacker answers a different tree: a mismatch row exists and is kept; official agrees exactly; bonsai has no
	// live tree (ssexp) and malivvan only the diagnostic reference.
	for _, want := range []string{",smacker,", "sexp-exact", "diagnostic-product", "mismatch"} {
		if !strings.Contains(cs, want) {
			t.Errorf("correctness.csv lacks %q", want)
		}
	}
	if t.Failed() {
		t.Logf("correctness.csv head:\n%s", cs[:min(len(cs), 2500)])
	}
	if _, err := os.Stat(filepath.Join(col, "SHA256SUMS")); err != nil {
		t.Fatal(err)
	}
	// a stray file and a link must be rejected
	mustWrite(t, filepath.Join(res, "stray.bin"), []byte("x"))
	if err := cmdCollect([]string{"-in", res, "-out", filepath.Join(dir, "c2")}); err == nil || !strings.Contains(err.Error(), "stray.bin") {
		t.Fatalf("stray file accepted: %v", err)
	}
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
