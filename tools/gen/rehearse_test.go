package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/go-treesitter/tools/gen/grammarpatch"
)

func TestCompareOutputs(t *testing.T) {
	got := compareOutputs(
		map[string]string{"parser.c": "a", "grammar.json": "g", "old.h": "o"},
		map[string]string{"parser.c": "b", "grammar.json": "g", "new.h": "n"})
	want := []rehearsalFile{
		{Name: "new.h", Change: "added", Nightly: "n"},
		{Name: "old.h", Change: "removed", Pinned: "o"},
		{Name: "parser.c", Change: "changed", Pinned: "a", Nightly: "b"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("file %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
	if len(compareOutputs(map[string]string{"x": "1"}, map[string]string{"x": "1"})) != 0 {
		t.Fatal("equal trees must not differ")
	}
}

func TestSchemaSummary(t *testing.T) {
	old := []byte(`[{"type":"a","named":true}]`)
	if s := schemaSummary(old, old); s.Assessment != "identical" {
		t.Fatalf("%+v", s)
	}
	if s := schemaSummary(old, []byte(`[{"type":"a","named":true,"fields":{}}]`)); s.Assessment != "FAIL" || s.Changes != 1 || s.ByCode["FIELDS_PRESENCE_CHANGED"] != 1 {
		t.Fatalf("%+v", s)
	}
	if s := schemaSummary(old, []byte(`not json`)); s.Assessment != "BLOCKED" {
		t.Fatalf("%+v", s)
	}
}

// identical only for equal hashes; a missing or unreadable side is BLOCKED with its reason.
func TestNodeTypesSchema(t *testing.T) {
	present := map[string]string{"node-types.json": "h"}
	both := [2]bool{true, true}
	for name, tc := range map[string]struct {
		files  []rehearsalFile
		outs   map[string]string
		read   [2]bool
		assess string
	}{
		"same hash":          {nil, present, both, "identical"},
		"missing on both":    {nil, map[string]string{}, both, "BLOCKED"},
		"nightly removed":    {[]rehearsalFile{{Name: "node-types.json", Change: "removed"}}, present, [2]bool{true, false}, "BLOCKED"},
		"pinned lacks it":    {[]rehearsalFile{{Name: "node-types.json", Change: "added"}}, map[string]string{}, [2]bool{false, true}, "BLOCKED"},
		"nightly unreadable": {[]rehearsalFile{{Name: "node-types.json", Change: "changed"}}, present, [2]bool{true, false}, "BLOCKED"},
	} {
		s := nodeTypesSchema(tc.files, tc.outs, [2][]byte{}, tc.read)
		if s.Assessment != tc.assess || (tc.assess == "BLOCKED" && s.Reason == "") {
			t.Errorf("%s: %+v", name, s)
		}
	}
}

func digestReport(sec float64, errText, nightlySHA string) *rehearsalReport {
	r := &rehearsalReport{Grammars: []rehearsalGrammar{
		{Lang: "c", Status: "identical", Pinned: &rehearsalRun{OK: true, Seconds: sec}, Nightly: &rehearsalRun{OK: true, Seconds: sec}},
		{Lang: "kotlin", Status: "differs", Pinned: &rehearsalRun{OK: true, Error: errText}, Nightly: &rehearsalRun{OK: true},
			Files: []rehearsalFile{
				{Name: "node-types.json", Change: "changed", Pinned: "n1", Nightly: "n2"},
				{Name: "parser.c", Change: "changed", Pinned: "p1", Nightly: nightlySHA}},
			Schema: &rehearsalSchema{Assessment: "FAIL", Changes: 1, ByCode: map[string]int{"NODE_ADDED": 1}, ByRisk: map[string]int{"low": 1}}},
	}}
	r.Nightly.Version, r.Nightly.BuildCommit, r.Nightly.GzipSHA256, r.Nightly.BinSHA256 = "tree-sitter 0.28.0 (aaaaaaa)", "aaaaaaa", "g1", "b1"
	return r
}

// Same file list and statuses, only a generated file's SHA-256 differs: the digest must change.
func TestRehearsalDigestCoversOutputHashes(t *testing.T) {
	if digestReport(1, "", "p2").computeDigest() == digestReport(1, "", "p3").computeDigest() {
		t.Fatal("digest ignores a changed nightly parser.c")
	}
	r := digestReport(1, "", "p2")
	base := r.computeDigest()
	r.Grammars[1].Schema.ByRisk = map[string]int{"high": 1}
	if r.computeDigest() == base {
		t.Fatal("digest ignores the per-risk counts")
	}
}

// Only the CLI's identity, timings or error text change: the digest must not.
func TestRehearsalDigestIgnoresIdentity(t *testing.T) {
	base := digestReport(1, "", "p2").computeDigest()
	r := digestReport(99, "other error", "p2")
	r.Nightly.Version, r.Nightly.BuildCommit, r.Nightly.GzipSHA256, r.Nightly.BinSHA256 = "tree-sitter 0.28.1 (bbbbbbb)", "bbbbbbb", "g2", "b2"
	r.Nightly.Meta = &nightlyMeta{URL: nightlyAssetPrefix + "x.gz", TagCommitBefore: strings.Repeat("b", 40)}
	if r.computeDigest() != base {
		t.Fatal("digest depends on the CLI identity, timings or error text")
	}
}

// A pinned failure does not hide the nightly outcome: both run, both are reported, nothing is compared.
func TestRehearsalPinnedFailsNightlySucceeds(t *testing.T) {
	defer func(p func(grammarpatch.Config) (string, *grammarpatch.Provenance, error)) { prepareGrammar = p }(prepareGrammar)
	var ran []bool
	prepareGrammar = func(c grammarpatch.Config) (string, *grammarpatch.Provenance, error) {
		ran = append(ran, c.Unpinned)
		if !c.Unpinned {
			return "", nil, errors.New("tree-sitter generate: timeout")
		}
		return c.Work, &grammarpatch.Provenance{Outputs: map[string]string{"parser.c": "x"}}, nil
	}
	r := &rehearsal{pins: &Pins{}, out: t.TempDir(), nightlyCLI: "nightly", nightlyVersion: "tree-sitter 0.28.0 (aaaaaaa)"}
	g := r.grammar("kotlin", GrammarPin{GrammarDir: "src"}, nil)
	if len(ran) != 2 || ran[0] || !ran[1] {
		t.Fatalf("prepare calls (unpinned flags): %v", ran)
	}
	if g.Status != "pinned-failed" || g.Pinned.OK || !strings.Contains(g.Pinned.Error, "timeout") || !g.Nightly.OK || g.Files != nil {
		t.Fatalf("%+v pinned=%+v nightly=%+v", g, g.Pinned, g.Nightly)
	}
	md := (&rehearsalReport{Grammars: []rehearsalGrammar{g}, Nightly: nightlyIdentity{Identity: "verified"}}).markdown()
	if !strings.Contains(md, "1 pinned-failed") || !strings.Contains(md, "timeout") {
		t.Fatalf("summary:\n%s", md)
	}
	if (r.grammar("c", GrammarPin{}, errors.New("sha256 mismatch"))).Status != "input-failed" || len(ran) != 2 {
		t.Fatal("a common input failure must stop both CLIs")
	}
}

func TestNightlyIdentityProblems(t *testing.T) {
	tag := "f0090e112482d66653b43ca73e85668db1cde794"
	ok := &nightlyMeta{URL: nightlyAssetPrefix + "tree-sitter-linux-x64.gz", Size: 10, APIDigest: "sha256:abc", TagCommitBefore: tag, TagCommitAfter: tag}
	if p := append(assetProblems(ok, 10, "abc"), commitProblems(ok, "f0090e11")...); len(p) != 0 {
		t.Fatal(p)
	}
	if len(assetProblems(ok, 11, "abc")) != 1 || len(assetProblems(&nightlyMeta{URL: ok.URL, Size: 10}, 10, "abc")) != 1 {
		t.Fatal("size mismatch or a missing API digest accepted")
	}
	moved := *ok
	moved.TagCommitAfter, moved.TagMoved = strings.Repeat("1", 40), true
	for name, p := range map[string][]string{
		"moved tag":        commitProblems(&moved, "f0090e11"),
		"other commit":     commitProblems(ok, "1234567"),
		"no commit":        commitProblems(ok, ""),
		"no commit, local": commitProblems(nil, ""),
	} {
		if len(p) == 0 {
			t.Errorf("%s accepted", name)
		}
	}
	if m := versionCommit.FindStringSubmatch("tree-sitter 0.28.0 (" + tag + ")"); m == nil || m[1] != tag {
		t.Fatalf("version parse: %v", m)
	}
}

// From the grammar entry: equal file maps still get a schema verdict, and a read node-types.json is compared even when
// saving its copy fails.
func TestRehearsalSchemaFromEntry(t *testing.T) {
	defer func(p func(grammarpatch.Config) (string, *grammarpatch.Provenance, error)) { prepareGrammar = p }(prepareGrammar)
	trees := map[bool]map[string]string{}
	prepareGrammar = func(c grammarpatch.Config) (string, *grammarpatch.Provenance, error) {
		root := filepath.Join(c.Work, "first")
		outs := map[string]string{}
		for n, s := range trees[c.Unpinned] {
			p := filepath.Join(root, "src", n)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return "", nil, err
			}
			if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
				return "", nil, err
			}
			outs[n] = sumBytes([]byte(s))
		}
		return root, &grammarpatch.Provenance{Outputs: outs}, nil
	}
	r := &rehearsal{pins: &Pins{}, out: t.TempDir()}
	trees[false], trees[true] = map[string]string{"parser.c": "p"}, map[string]string{"parser.c": "p"}
	g := r.grammar("tiny", GrammarPin{GrammarDir: "src"}, nil)
	if g.Status != "identical" || g.Schema == nil || g.Schema.Assessment != "BLOCKED" || !strings.Contains(g.Schema.Reason, "both sides") {
		t.Fatalf("no node-types.json on either side: %+v schema=%+v", g, g.Schema)
	}
	trees[false]["node-types.json"], trees[true]["node-types.json"] = `[{"type":"a","named":true}]`, `[{"type":"a","named":true}]`
	if g := r.grammar("tiny", GrammarPin{GrammarDir: "src"}, nil); g.Status != "identical" || g.Schema.Assessment != "identical" {
		t.Fatalf("equal node-types.json: %+v schema=%+v", g, g.Schema)
	}
	trees[true]["node-types.json"] = `[{"type":"a","named":true,"fields":{}}]`
	if err := os.WriteFile(filepath.Join(r.out, "files"), nil, 0o644); err != nil { // copies cannot be saved
		t.Fatal(err)
	}
	g = r.grammar("tiny", GrammarPin{GrammarDir: "src"}, nil)
	if g.Status != "differs" || g.Schema.Assessment != "FAIL" || g.Schema.Changes != 1 || len(g.ArtifactErrors) != 2 || !strings.HasPrefix(g.ArtifactErrors[0], "saving") {
		t.Fatalf("read but unsaved node-types.json: %+v schema=%+v", g, g.Schema)
	}
}

// pinnedPlatform makes the rehearsal use the host platform if pins.json has a pinned CLI for it, else the first pinned
// one (CI also runs these tests on hosts without a pinned CLI, such as windows-arm64). Only the pins lookup and the
// report label change; the nightly test binary still runs natively.
func pinnedPlatform(t *testing.T) string {
	t.Helper()
	pins, err := loadPins("pins.json")
	if err != nil {
		t.Fatal(err)
	}
	keys := slices.Sorted(maps.Keys(pins.Tools.TreeSitterCLIArchives))
	if len(keys) == 0 {
		t.Fatal("pins.json has no pinned CLI")
	}
	p := keys[0]
	if _, ok := pins.Tools.TreeSitterCLIArchives[hostPlatform()]; ok {
		p = hostPlatform()
	}
	orig := rehearsalPlatform
	t.Cleanup(func() { rehearsalPlatform = orig })
	rehearsalPlatform = func() string { return p }
	return p
}

// A platform without a pinned CLI stops before anything runs, with the reason; it never compares against nothing.
func TestRehearsalUnpinnedPlatform(t *testing.T) {
	defer func(f func() string) { rehearsalPlatform = f }(rehearsalPlatform)
	rehearsalPlatform = func() string { return "plan9-mips" }
	dir := t.TempDir()
	err := runRehearsalCommand([]string{"-root", ".", "-dl", dir, "-tree-sitter", "pinned", "-nightly-gz", filepath.Join(dir, "x.gz"),
		"-nightly-unsandboxed", "-out", filepath.Join(dir, "out"), "json"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "no pinned CLI for plan9-mips") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "out", "report.json")); !os.IsNotExist(err) {
		t.Fatal("a report was written for a platform without a pinned CLI")
	}
}

// Unreadable or inconsistent nightly facts end in the same held report (no digest), before anything downloaded runs.
func TestRehearsalHeldReports(t *testing.T) {
	pinnedPlatform(t)
	dir := t.TempDir()
	gzOK := func() []byte {
		var b bytes.Buffer
		z := gzip.NewWriter(&b)
		z.Write([]byte("not a real CLI"))
		z.Close()
		return b.Bytes()
	}()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	meta := func(gz []byte, size int) []byte {
		b, _ := json.Marshal(nightlyMeta{URL: nightlyAssetPrefix + "x.gz", Size: int64(size), APIDigest: "sha256:" + sumBytes(gz),
			TagCommitBefore: strings.Repeat("a", 40), TagCommitAfter: strings.Repeat("a", 40)})
		return b
	}
	notGz := []byte("plain bytes, not gzip")
	for name, tc := range map[string]struct {
		gz, meta string
		want     string
	}{
		"missing meta":  {write("ok.gz", gzOK), filepath.Join(dir, "absent.json"), "reading -nightly-meta"},
		"broken meta":   {write("ok.gz", gzOK), write("broken.json", []byte("{")), "not a JSON object"},
		"missing asset": {filepath.Join(dir, "absent.gz"), write("m.json", meta(gzOK, len(gzOK))), "reading the asset"},
		"size mismatch": {write("ok.gz", gzOK), write("size.json", meta(gzOK, len(gzOK)+1)), "release API size"},
		"not gzip":      {write("plain.gz", notGz), write("plain.json", meta(notGz, len(notGz))), "decompressing"},
	} {
		out := filepath.Join(dir, "out-"+strings.ReplaceAll(name, " ", "-"))
		err := runRehearsalCommand([]string{"-root", ".", "-dl", t.TempDir(), "-tree-sitter", "pinned", "-nightly-gz", tc.gz,
			"-nightly-meta", tc.meta, "-nightly-sandbox", filepath.Join(dir, "never-run"), "-out", out, "json"}, io.Discard)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", name, err)
			continue
		}
		var rep rehearsalReport
		b, rerr := os.ReadFile(filepath.Join(out, "report.json"))
		if rerr != nil || json.Unmarshal(b, &rep) != nil || rep.Nightly.Identity != "held" || rep.Grammars != nil || rep.Nightly.BinSHA256 != "" {
			t.Errorf("%s: report %v %s", name, rerr, b)
		}
		if _, err := os.Stat(filepath.Join(out, "digest.txt")); !os.IsNotExist(err) {
			t.Errorf("%s: digest written", name)
		}
		if _, err := os.Stat(filepath.Join(out, "summary.md")); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Without the wrapper (and without the explicit local opt-in) nothing runs and no report is started.
	out := filepath.Join(dir, "out-nosandbox")
	if err := runRehearsalCommand([]string{"-dl", dir, "-tree-sitter", "p", "-nightly-gz", write("ok.gz", gzOK), "-out", out}, io.Discard); err == nil {
		t.Fatal("ran without -nightly-sandbox or -nightly-unsandboxed")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("output directory created without a sandbox decision")
	}
}

// TestMain doubles as a fake nightly CLI (FAKE_TS_CLI=1): `--version` prints a version line that carries the environment
// it was given.
func TestMain(m *testing.M) {
	if os.Getenv("FAKE_TS_CLI") == "1" {
		fmt.Printf("tree-sitter 0.0.0 (aaaaaaa) token=%t mount=%t\n", os.Getenv("GH_TOKEN")+os.Getenv("GITHUB_TOKEN") != "",
			os.Getenv("TS_GENERATE_INPUT")+os.Getenv("TS_GENERATE_OUTPUT") != "")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// The Go entry with the direct opt-in (-nightly-unsandboxed): the nightly --version child gets neither the caller's
// GitHub token nor generate mount variables.
func TestRehearsalUnsandboxedVersionEnvironment(t *testing.T) {
	pinnedPlatform(t)
	self, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	var gz bytes.Buffer
	z := gzip.NewWriter(&gz)
	z.Write(self)
	z.Close()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fake.gz"), gz.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_TS_CLI", "1")
	t.Setenv("GH_TOKEN", "dummy-not-a-token")
	t.Setenv("GITHUB_TOKEN", "dummy-not-a-token")
	t.Setenv("TS_GENERATE_INPUT", "stale-input")
	t.Setenv("TS_GENERATE_OUTPUT", "stale-output")
	out := filepath.Join(dir, "out")
	if err := runRehearsalCommand([]string{"-root", ".", "-dl", t.TempDir(), "-tree-sitter", "pinned", "-nightly-gz", filepath.Join(dir, "fake.gz"),
		"-nightly-unsandboxed", "-out", out, "json"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	var rep rehearsalReport
	b, err := os.ReadFile(filepath.Join(out, "report.json"))
	if err != nil || json.Unmarshal(b, &rep) != nil {
		t.Fatalf("report: %v %s", err, b)
	}
	if rep.Nightly.Version != "tree-sitter 0.0.0 (aaaaaaa) token=false mount=false" || rep.Nightly.Sandbox != "none" || rep.Nightly.Identity != "unchecked" {
		t.Fatalf("nightly %+v", rep.Nightly)
	}
	if len(rep.Grammars) != 1 || rep.Grammars[0].Status != "input-failed" { // no archives in -dl: nothing generated
		t.Fatalf("grammars %+v", rep.Grammars)
	}
}
