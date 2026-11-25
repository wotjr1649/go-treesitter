package main

import (
	"archive/zip"
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
	wj, _ := os.ReadFile(filepath.Join(work, "workload.json"))
	mustWrite(t, filepath.Join(res, "workload.json"), wj) // the measuring script copies it next to the results
	mustWrite(t, filepath.Join(res, "builds.csv"), []byte("adapter,mode,status,exit,bytes,delta_vs_empty_bytes,seconds,diagnostics\nproduct,x,ok,0,10,0,1,\n"))
	mustWrite(t, filepath.Join(res, "provenance", "workflow.json"), []byte(workflowJSON))
	col := filepath.Join(dir, "collected")
	if err := cmdCollect([]string{"-in", res, "-out", col}); err != nil {
		t.Fatal(err)
	}
	// the validated directory becomes the one bundle the job uploads; the aggregate job validates it again from scratch
	bundles := filepath.Join(dir, "bundles")
	if err := cmdPack([]string{"-dir", col, "-out", filepath.Join(bundles, "compare-r1-c.zip", "bundle.zip")}); err != nil {
		t.Fatal(err)
	}
	noccDir := filepath.Join(dir, "nocc")
	mustWrite(t, filepath.Join(noccDir, "provenance", "workflow.json"), []byte(workflowJSON))
	mustWrite(t, filepath.Join(noccDir, "builds.csv"), []byte(noccCSV("product,x,ok,0,30,20,1,ok,ok,", "official,x,failed,1,0,0,1,n/a,expected-unsupported,cgo")))
	if err := cmdPack([]string{"-dir", noccDir, "-out", filepath.Join(bundles, "compare-nocc-r1.zip", "bundle.zip")}); err != nil {
		t.Fatal(err)
	}
	agg := filepath.Join(dir, "agg")
	if err := cmdAggregate([]string{"-in", bundles, "-out", agg, "-work", filepath.Join(dir, "aggwork"), "-runners", "r1", "-langs", "c"}); err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(agg, "summary.md")); !strings.Contains(string(b), "## r1 / c") || !strings.Contains(string(b), "Build outcomes") {
		t.Fatalf("aggregate summary:\n%s", b)
	}
	if ents, _ := os.ReadDir(agg); len(ents) != 2 { // only the summary and the hashes are left to upload
		t.Fatalf("aggregate output holds %d entries", len(ents))
	}
	os.RemoveAll(filepath.Join(bundles, "compare-nocc-r1.zip"))
	agg2 := filepath.Join(dir, "agg2")
	if err := cmdAggregate([]string{"-in", bundles, "-out", agg2, "-work", filepath.Join(dir, "aggwork2"), "-runners", "r1", "-langs", "c"}); err == nil {
		t.Fatal("a missing artifact was accepted")
	}
	if _, err := os.Stat(filepath.Join(agg2, "summary.md")); err == nil {
		t.Fatal("a failed aggregation left a summary to upload")
	}
	if err := cmdReport([]string{"-in", col}); err != nil {
		t.Fatal(err)
	}
	if err := cmdSeal([]string{"-dir", col}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(col, "SHA256SUMS")); !strings.Contains(string(b), "summary.md") {
		t.Fatal("seal did not hash the report")
	}
	summary, _ := os.ReadFile(filepath.Join(col, "summary.md"))
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
	// complete cells only: the fake workers finish every round, so measured cells exist and the ratios table has rows
	var cellList []Cell
	if err := readJSON(filepath.Join(res, "cells.json"), &cellList); err != nil {
		t.Fatal(err)
	}
	measured := 0
	for _, c := range cellList {
		if c.State == "measured" && c.OK["time"] == c.Want && c.OK["alloc"] == c.Want {
			measured++
		}
	}
	if measured == 0 || !strings.Contains(s, "| c | official | parse | small |") {
		t.Errorf("measured=%d, summary:\n%s", measured, s)
	}
	// a forged row for a cell the plan does not contain is rejected
	forged := filepath.Join(dir, "forged")
	if err := os.CopyFS(forged, os.DirFS(res)); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(filepath.Join(forged, "measurements.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(`{"cell":"c|official|parse|nope","pass":"time","round":0,"role":"candidate","adapter":"official","lang":"c","op":"parse","exit":0,"wall_ns":1}` + "\n")
	f.Close()
	if err := cmdCollect([]string{"-in", forged, "-out", filepath.Join(dir, "c3")}); err == nil || !strings.Contains(err.Error(), "expected cell") {
		t.Fatalf("forged row accepted: %v", err)
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

func TestSealCap(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), []byte("x"))
	if err := cmdSeal([]string{"-dir", dir}); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "big.bin"), make([]byte, maxTotal+1))
	if err := cmdSeal([]string{"-dir", dir}); err == nil {
		t.Fatal("over-cap set sealed")
	}
}

func noccCSV(product, official string) string {
	rows := []string{"adapter,mode,status,exit,bytes,delta_vs_empty_bytes,seconds,smoke_parse,outcome,diagnostics", "empty,x,ok,0,10,0,1,n/a,ok,"}
	rows = append(rows, product)
	rows = append(rows, official, "smacker,x,failed,1,0,0,1,n/a,expected-unsupported,cgo",
		"gotreesitter,x,ok,0,30,20,1,ok,ok,", "bonsai,x,ok,0,30,20,1,ok,ok,", "malivvan,x,ok,0,30,20,1,ok,ok,")
	return strings.Join(rows, "\n") + "\n"
}

func TestNoccGate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "builds.csv")
	good, off := "product,x,ok,0,30,20,1,ok,ok,", "official,x,failed,1,0,0,1,n/a,expected-unsupported,cgo"
	mustWrite(t, p, []byte(noccCSV(good, off)))
	if err := cmdGateNocc([]string{"-builds", p}); err != nil {
		t.Fatal(err)
	}
	for name, csv := range map[string]string{
		"product smoke failure":   noccCSV("product,x,ok,0,30,20,1,none,smoke-failure,", off),
		"status/exit disagree":    noccCSV("product,x,ok,1,30,20,1,ok,ok,", off),
		"ok without smoke":        noccCSV("product,x,ok,0,30,20,1,none,ok,", off),
		"pure-Go expected-unsup.": noccCSV(good, off) + "bonsai,x,failed,1,0,0,1,n/a,expected-unsupported,\n",
		"missing adapter":         strings.Replace(noccCSV(good, off), "malivvan,x,ok,0,30,20,1,ok,ok,\n", "", 1),
	} {
		mustWrite(t, p, []byte(csv))
		if err := cmdGateNocc([]string{"-builds", p}); err == nil {
			t.Errorf("%s passed the gate", name)
		}
	}
}

// The no-compiler build runs the real go tool offline on tiny stdlib-and-harness adapters: a pure-Go product must build and
// parse the smoke fixture, a cgo adapter must be reported as expected-unsupported, and a broken smoke parse is a smoke-failure.
func TestNoccBuildsAndClassifies(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go build")
	}
	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go tool")
	}
	harnessDir, _ := filepath.Abs("../../harness")
	ad := filepath.Join(t.TempDir(), "adapters")
	mod := func(name string) {
		mustWrite(t, filepath.Join(ad, name, "go.mod"), []byte("module example.invalid/"+name+"\n\ngo 1.26.0\n\nrequire github.com/wotjr1649/go-treesitter/tools/compare/harness v0.0.0\n\nreplace github.com/wotjr1649/go-treesitter/tools/compare/harness => "+filepath.ToSlash(harnessDir)+"\n"))
	}
	good := "package main\nimport h \"github.com/wotjr1649/go-treesitter/tools/compare/harness\"\ntype s struct{ h.Base }\nfunc (s) Identity() map[string]string { return nil }\nfunc (s) Parse(b []byte) (h.Tree, error) { return b, nil }\nfunc (s) Sexp(h.Tree) (string, bool, error) { return \"(x)\", false, nil }\nfunc main() { h.Main(\"NAME\", func(string) (h.Session, error) { return s{}, nil }) }\n"
	for _, a := range noccAdapters {
		mod(a)
		src := strings.ReplaceAll(good, "NAME", a)
		switch a {
		case "empty":
			src = "package main\nfunc main() {}\n"
		case "official", "smacker":
			src = "package main\n// #include <stdio.h>\nimport \"C\"\nfunc main() {}\n"
		case "bonsai": // builds, but answers as another adapter: a smoke failure
			src = strings.ReplaceAll(good, "NAME", "somebody-else")
		}
		mustWrite(t, filepath.Join(ad, a, "main.go"), []byte(src))
	}
	out := filepath.Join(t.TempDir(), "builds.csv")
	cache, _ := exec.Command(goPath, "env", "GOMODCACHE").Output()
	if err := nocc(noccOpts{adapters: ad, gobin: filepath.Dir(goPath), modcache: strings.TrimSpace(string(cache)), out: out}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	got := map[string]string{}
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n")[1:] {
		f := strings.Split(l, ",")
		got[f[0]] = f[8]
	}
	want := map[string]string{"empty": "ok", "product": "ok", "official": "expected-unsupported", "smacker": "expected-unsupported",
		"gotreesitter": "ok", "bonsai": "smoke-failure", "malivvan": "ok"}
	for a, w := range want {
		if got[a] != w {
			t.Errorf("%s: %q, want %q\n%s", a, got[a], w, b)
		}
	}
	if err := cmdGateNocc([]string{"-builds", out}); err != nil {
		t.Errorf("the product is ok, the gate must pass: %v", err)
	}
}

// Only tree-equal (or exact) grades of the case the cell exercised qualify; lossy, shape-only and mismatch do not.
func TestGradedEqualOnlyForEqualTrees(t *testing.T) {
	g := map[string]string{
		"a|i|fresh|": "tree-equal", "b|i|fresh|": "mismatch", "c|i|fresh|": "shape-only", "d|i|fresh|": "tree-lossy", "e|i|fresh|": "sexp-exact",
		"a|i|incremental|token": "mismatch", "e|i|incremental|token": "tree-equal",
	}
	parse, edit := Cell{Input: "i", Op: "parse"}, Cell{Input: "i", Op: "edit"}
	for ad, want := range map[string]bool{"a": true, "b": false, "c": false, "d": false, "e": true, "z": false} {
		if gradedEqual(g, ad, parse) != want {
			t.Errorf("parse %s: want %t", ad, want)
		}
	}
	if gradedEqual(g, "a", edit) || !gradedEqual(g, "e", edit) { // a fresh grade never approves the incremental case
		t.Error("edit cells must use the incremental token case")
	}
}

func TestMeasuredNeedsEveryRound(t *testing.T) {
	c := Cell{State: "planned", Want: 2, OK: map[string]int{"time": 2, "alloc": 1}}
	r := &runner{cells: []Cell{c}}
	r.finalize()
	if r.cells[0].State != "incomplete" {
		t.Fatalf("%s", r.cells[0].State)
	}
	r.cells = []Cell{{State: "planned", Want: 2, bad: true, OK: map[string]int{"time": 1}}}
	r.finalize()
	if r.cells[0].State != "partial" {
		t.Fatalf("%s", r.cells[0].State)
	}
	r.cells = []Cell{{State: "planned", Want: 2, OK: map[string]int{"time": 2, "alloc": 2}}}
	r.finalize()
	if r.cells[0].State != "measured" {
		t.Fatalf("%s", r.cells[0].State)
	}
}

func zipOf(t *testing.T, entries func(zw *zip.Writer)) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "b.zip")
	f, _ := os.Create(p)
	zw := zip.NewWriter(f)
	entries(zw)
	zw.Close()
	f.Close()
	return p
}

func TestUnpackRefusesHostileBundles(t *testing.T) {
	add := func(zw *zip.Writer, name string, body []byte) {
		w, _ := zw.Create(name)
		w.Write(body)
	}
	cases := map[string]func(zw *zip.Writer){
		"traversal":  func(zw *zip.Writer) { add(zw, "../evil.txt", []byte("x")) },
		"not listed": func(zw *zip.Writer) { add(zw, "scripts/run.sh", []byte("x")) },
		"duplicate":  func(zw *zip.Writer) { add(zw, "failures/a.txt", []byte("x")); add(zw, "failures/a.txt", []byte("y")) },
		"too many": func(zw *zip.Writer) {
			for i := 0; i <= maxEntries; i++ {
				add(zw, fmt.Sprintf("failures/f%d.txt", i), []byte("x"))
			}
		},
		"oversize": func(zw *zip.Writer) { add(zw, "failures/a.txt", make([]byte, maxFile+1)) },
		"symlink": func(zw *zip.Writer) {
			h := &zip.FileHeader{Name: "failures/a.txt"}
			h.SetMode(os.ModeSymlink | 0o777)
			w, _ := zw.CreateHeader(h)
			w.Write([]byte("/etc/passwd"))
		},
		"directory": func(zw *zip.Writer) { zw.Create("failures/") },
	}
	for name, f := range cases {
		dst := t.TempDir()
		if _, err := unpackBounded(zipOf(t, f), dst); err == nil {
			t.Errorf("%s: accepted", name)
		} else if ents, _ := os.ReadDir(dst); len(ents) != 0 {
			t.Errorf("%s: extracted before refusing", name)
		}
	}
	good := zipOf(t, func(zw *zip.Writer) { add(zw, "failures/a.txt", []byte("ok")) })
	dst := t.TempDir()
	if n, err := unpackBounded(good, dst); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "failures", "a.txt")); string(b) != "ok" {
		t.Fatal("round trip")
	}
}

// Typed schemas: unknown fields and trailing data are refused instead of decoded into an open type.
func TestStrictSchemas(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "run.json")
	mustWrite(t, p, []byte(`{"schema":1,"seed":"s","surprise":true}`))
	var v runInfo
	if err := strictFile(p, &v); err == nil {
		t.Fatal("unknown field accepted")
	}
	mustWrite(t, p, []byte(`{"schema":1} {"schema":2}`))
	if err := strictFile(p, &v); err == nil {
		t.Fatal("trailing data accepted")
	}
	mustWrite(t, p, []byte(`{"schema":1,"seed":"s"}`))
	if err := strictFile(p, &v); err != nil {
		t.Fatal(err)
	}
	if err := checkCSVShape("correctness.csv", [][]string{{"lang", "adapter"}}, nil); err == nil {
		t.Fatal("a wrong correctness header was accepted")
	}
}

const workflowJSON = `{"repository":"o/r","run_id":"1","run_attempt":"1","sha":"abc","ref":"refs/heads/main","workflow":"w","runner_arch":"X64","runner_image":"i","mode":"smoke"}`

// The top level must end after the value: Decoder.More is not an end-of-input check (a stray closing bracket fools it).
func TestStrictTopLevelEOFAndJSONL(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "run.json")
	var v runInfo
	for _, bad := range []string{`{"schema":1} ]`, `{"schema":1} }`, `{"schema":1} 5`, `{"schema":1}{`} {
		mustWrite(t, p, []byte(bad))
		if err := strictFile(p, &v); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	mustWrite(t, p, []byte("{\"schema\":1}\n\n"))
	if err := strictFile(p, &v); err != nil {
		t.Fatal(err)
	}
	exp := &expectation{cells: map[string]Cell{}, inputs: map[string]bool{}}
	jl := filepath.Join(dir, "measurements.jsonl")
	row := `{"cell":"official/c","pass":"preflight","round":0,"role":"candidate","adapter":"official","lang":"c","op":"ctl","exit":0,"wall_ns":1}`
	mustWrite(t, jl, []byte(row+"\n"))
	if err := validate(jl, "measurements.jsonl", exp); err != nil {
		t.Fatalf("a good row: %v", err)
	}
	mustWrite(t, jl, []byte(strings.Replace(row, `"adapter":"official"`, `"adapter":"product"`, 1)+"\n"))
	if err := validate(jl, "measurements.jsonl", exp); err == nil {
		t.Fatal("a preflight row for the wrong cell was accepted")
	}
	mustWrite(t, jl, []byte(row+" ]\n"))
	if err := validate(jl, "measurements.jsonl", exp); err == nil {
		t.Fatal("trailing data after a JSONL value was accepted")
	}
}

func TestBundleMustCarryEveryRequiredFile(t *testing.T) {
	dir := t.TempDir()
	d := filepath.Join(dir, "b")
	mustWrite(t, filepath.Join(d, "builds.csv"), []byte(noccCSV("product,x,ok,0,30,20,1,ok,ok,", "official,x,failed,1,0,0,1,n/a,expected-unsupported,cgo")))
	bundles := filepath.Join(dir, "bundles")
	if err := cmdPack([]string{"-dir", d, "-out", filepath.Join(bundles, "compare-nocc-r1.zip", "bundle.zip")}); err != nil {
		t.Fatal(err)
	}
	if _, err := aggregateOne(filepath.Join(bundles, "compare-nocc-r1"), filepath.Join(dir, "o"), true); err == nil || !strings.Contains(err.Error(), "workflow.json") {
		t.Fatalf("a bundle without provenance was accepted: %v", err)
	}
}

// The downloader saves each blob as one file, under a name the blob chose, in the directory of its artifact; anything
// else in that place is refused before a byte is read.
func TestBundleFileShape(t *testing.T) {
	root := t.TempDir()
	mk := func(name string, f func(d string)) {
		d := filepath.Join(root, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		f(d)
	}
	mk("plain", func(d string) { mustWrite(t, filepath.Join(d, "any-name.zip"), []byte("x")) })
	mk("two", func(d string) {
		mustWrite(t, filepath.Join(d, "a"), []byte("x"))
		mustWrite(t, filepath.Join(d, "b"), []byte("x"))
	})
	mk("empty", func(d string) {})
	mk("subdir", func(d string) { os.MkdirAll(filepath.Join(d, "sub"), 0o755) })
	mk("link", func(d string) {
		if err := os.Symlink(filepath.Join(root, "plain", "any-name.zip"), filepath.Join(d, "l.zip")); err != nil {
			t.Skip("no symlinks here:", err)
		}
	})
	mustWrite(t, filepath.Join(root, "loose"), []byte("x")) // a file where the directory belongs
	if p, err := bundleFile(root, "plain"); err != nil || filepath.Base(p) != "any-name.zip" {
		t.Fatal(p, err)
	}
	for _, n := range []string{"two", "empty", "subdir", "link", "loose", "absent"} {
		if _, err := bundleFile(root, n); err == nil {
			t.Errorf("%s: accepted", n)
		}
	}
}

func TestExactRoundsAndBothPasses(t *testing.T) {
	if !exactRounds([]int{0, 1, 2}) || exactRounds([]int{0, 2, 3}) || exactRounds([]int{1, 2, 3}) {
		t.Fatal("exactRounds")
	}
	a := []CellStat{{Lang: "c", Cand: "x", Op: "parse", Group: "small", Input: "i1"}, {Lang: "c", Cand: "x", Op: "parse", Group: "small", Input: "i2"}}
	b := []CellStat{{Lang: "c", Cand: "x", Op: "parse", Group: "small", Input: "i2"}}
	ta, tb := bothPasses(a, b)
	if len(ta) != 1 || ta[0].Input != "i2" || len(tb) != 1 {
		t.Fatalf("%v %v", ta, tb)
	}
}
