package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The candidates of one report: an owner grammar behind its head with a tag-only release, a third-party grammar with a
// newer release (not behind its head) and a tool with a tag-only release. The golden text is the schema.
func TestCandidatesFromReport(t *testing.T) {
	api := tagServer(t, "v1.0.0", map[string]string{
		"/repos/wotjr1649/tree-sitter-owned": `[{"name":"v2.0.0"},{"name":"v1.0.0"}]`,
		"/repos/ncruces/wasm2go":             `[{"name":"v2.0.1"}]`,
	}, func(path string) string {
		switch {
		case strings.HasSuffix(path, "...v2.0.0"), strings.HasSuffix(path, "-rel/compare/pin...v1.0.0"):
			return `{"status":"ahead"}`
		case strings.HasSuffix(path, "-owned/compare/pin...head"):
			return `{"status":"ahead","ahead_by":3}`
		case strings.HasSuffix(path, "...v1.0.0"):
			return `{"status":"behind"}`
		}
		return `{"status":"identical"}`
	})
	p := tagPins(map[string]struct{ Repo, Commit string }{
		"owned": {"https://github.com/wotjr1649/tree-sitter-owned", "pin"},
		"rel":   {"https://github.com/someone/tree-sitter-rel", "pin"},
	})
	var out bytes.Buffer
	if stale, failed := report(p, &out, api); !stale || failed {
		t.Fatalf("stale=%v failed=%v\n%s", stale, failed, &out)
	}
	path := filepath.Join(t.TempDir(), "candidates.json")
	if err := writeCandidates(path, api, false); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	want := `{"version":1,"candidates":[
  {"component":"owned","pinned":"pin","newest":"head","kind":"head","repo":"wotjr1649/tree-sitter-owned","owner":true,"tagOnly":false,"grammar":true,"accepted":false},
  {"component":"owned","pinned":"pin","newest":"v2.0.0","kind":"release","repo":"wotjr1649/tree-sitter-owned","owner":true,"tagOnly":true,"grammar":true,"accepted":false},
  {"component":"rel","pinned":"pin","newest":"v1.0.0","kind":"release","repo":"someone/tree-sitter-rel","owner":false,"tagOnly":false,"grammar":true,"accepted":false},
  {"component":"wasm2go","pinned":"v2.0.0","newest":"v2.0.1","kind":"release","repo":"ncruces/wasm2go","owner":false,"tagOnly":true,"grammar":false,"accepted":false}
]}
`
	if string(got) != want {
		t.Errorf("candidates file:\n%s\nwant:\n%s", got, want)
	}
}

func TestCandidatesFileEmptyAndAfterFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidates.json")
	if err := writeCandidates(path, newAPI(), false); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "{\"version\":1,\"candidates\":[]}\n" {
		t.Errorf("empty list: %q", got)
	}
	if err := writeCandidates(path, newAPI(), true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("a failed run must leave no candidates file")
	}
}

func TestCandidatesSection(t *testing.T) {
	api := newAPI()
	api.acceptances = []acceptance{{Component: "held", Kind: "k", Accepted: "2026-01-01", Expires: "2027-01-01"}}
	api.propose(candidate{Component: "bash", Pinned: "p", Newest: "v1.2.3", Kind: "release", Repo: "wotjr1649/tree-sitter-bash", Grammar: true}, "k")
	api.propose(candidate{Component: "held", Pinned: "p", Newest: "v9.9.9", Kind: "release", Repo: "a/held", Grammar: true}, "k") // accepted
	api.propose(candidate{Component: "evil", Pinned: "p", Newest: "v1;touch x", Kind: "release", Repo: "a/evil", Grammar: true}, "")
	api.propose(candidate{Component: "owned", Pinned: "p", Newest: "abc123", Kind: "head", Repo: "wotjr1649/o", Grammar: true}, "")
	api.propose(candidate{Component: "wasm2go", Pinned: "v1", Newest: "v2", Kind: "release", Repo: "ncruces/wasm2go", TagOnly: true}, "")
	var b strings.Builder
	candidatesSection(&b, api)
	got := b.String()
	for _, want := range []string{
		"- `bash` release `v1.2.3`: `gh workflow run " + autoPRWorkflow + " -f grammar=bash -f tag=v1.2.3`",
		"- `owned` default-branch head `abc123` — no dispatch",
		"- `wasm2go` tag without a release, third party `v2` — no dispatch",
		"- (name not shown: unexpected characters) — no dispatch",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "held") || strings.Contains(got, "touch") || strings.Count(got, "gh workflow run") != 1 {
		t.Errorf("accepted and unsafe candidates must not get a command:\n%s", got)
	}
	var none strings.Builder
	candidatesSection(&none, newAPI())
	if none.Len() != 0 {
		t.Errorf("no candidates, no section: %q", none.String())
	}
}

// The flag reaches the file and the issue body of a whole run.
func TestRunWritesCandidates(t *testing.T) {
	_, _, _, _, _, candidates := runWatchFiles(t, fmt.Sprintf(runPins, ""), okGraphQL)
	if candidates != "{\"version\":1,\"candidates\":[]}\n" {
		t.Errorf("healthy run: %q", candidates)
	}
	if _, _, _, _, _, candidates = runWatchFiles(t, `{"grammars":`, okGraphQL); candidates != "" {
		t.Errorf("hard error must write nothing: %q", candidates)
	}
}
