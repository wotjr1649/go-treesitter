package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// isolateGit stops git from finding a repository above this test's temp dirs (the checkout, when TEMP is inside it).
func isolateGit(t *testing.T) {
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(t.TempDir()))
}

func TestLoadPatches(t *testing.T) {
	got, err := loadPatches([]byte(`{"grammars":{"a":{"repo":"x"},"b":{"patches":[{"file":"patches/b/1.patch","upstreamReport":"https://r/1"}]}}}`))
	if err != nil || len(got) != 1 || got["b"][0] != (patchRec{"patches/b/1.patch", "https://r/1"}) {
		t.Fatalf("got %v, err %v", got, err)
	}
	// Malformed optional metadata is an error value for the diagnostic; the readable grammars still load.
	for _, bad := range []string{`{}`, `[1]`, `"x"`, `[{"file":3}]`} {
		got, err := loadPatches([]byte(`{"grammars":{"a":{"patches":` + bad + `},"b":{"patches":[{"file":"p","upstreamReport":"r"}]}}}`))
		if err == nil || !strings.Contains(err.Error(), "grammar a") || len(got["b"]) != 1 || len(got["a"]) != 0 {
			t.Errorf("patches %s: got %v, err %v", bad, got, err)
		}
	}
}

func TestPatchSection(t *testing.T) {
	patches := map[string][]patchRec{
		"yaml": {{"patches/yaml/1.patch", "https://r/old"}, {"patches/yaml/2.patch", "https://r/new"}, {"patches/yaml/3.patch", "https://r/old2"}, {"patches/yaml/4.patch", "https://r/old"}},
		"ruby": {{"a", "https://r/1"}, {"b", "https://r/2"}, {"c", "https://r/3"}, {"d", "https://r/new"}},
		"bash": {{"e", ""}, {"f", "https://r/missing"}},
	}
	added := func(report string) (time.Time, error) {
		switch report {
		case "https://r/old", "https://r/old2":
			return time.Date(2025, 11, 17, 0, 0, 0, 0, time.UTC), nil
		case "https://r/missing":
			return time.Time{}, errors.New("not in git history")
		}
		return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), nil
	}
	api := newAPI()
	api.now = time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	var out bytes.Buffer
	patchSection(&out, api, patches, nil, added, nil)
	for _, want := range []string{"| yaml | 4 **above -patch-alert 3** | patches/yaml/1.patch | https://r/old | 2025-11-17 | **older than 12 months: owner re-decision** |",
		"| yaml | 4 **above -patch-alert 3** | patches/yaml/2.patch | https://r/new | 2026-06-01 | ok |", "| ruby | 4 **above -patch-alert 3** |",
		"| age unknown: no upstreamReport |", "age unknown: not in git history", "::warning::yaml: patch patches/yaml/1.patch"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q:\n%s", want, out.String())
		}
	}
	// One warning per report: patches 1 and 4 share old, patch 3 has its own.
	if len(api.findings) != 2 || api.findings[0].kind != "patch older than 12 months: https://r/old" || api.findings[1].kind != "patch older than 12 months: https://r/old2" {
		t.Fatalf("findings = %v", api.findings)
	}
	// Accepting one report's warning does not hide the other's.
	api.acceptances = []acceptance{{Component: "yaml", Kind: api.findings[0].kind, Accepted: "2026-11-01", Expires: "2027-11-01"}}
	if n := warningSummary(&out, api); n != 1 {
		t.Errorf("%d unaccepted warnings, want 1", n)
	}
}

func TestPinnedPatchTracking(t *testing.T) {
	raw, err := os.ReadFile("../../gen/pins.json")
	if err != nil {
		t.Fatal(err)
	}
	patches, err := loadPatches(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ lang, file, report string }{
		{"golang", "0001-new-value-argument.patch", "https://github.com/tree-sitter/tree-sitter-go/issues/201"},
		{"ruby", "0001-heredoc-utf8.patch", "https://github.com/tree-sitter/tree-sitter-ruby/issues/305"},
		{"ruby", "0002-global-variable-non-ascii.patch", "https://github.com/tree-sitter/tree-sitter-ruby/issues/306"},
		{"ruby", "0003-non-ascii-symbol.patch", "https://github.com/tree-sitter/tree-sitter-ruby/issues/292"},
		{"powershell", "0001-non-ascii-names.patch", "https://github.com/airbus-cert/tree-sitter-powershell/issues/70"},
		{"xml", "0001-non-ascii-names.patch", "https://github.com/tree-sitter-grammars/tree-sitter-xml/issues/40"},
		{"xml", "0002-tag-name-code-points.patch", "https://github.com/tree-sitter-grammars/tree-sitter-xml/issues/40"},
	} {
		want := patchRec{"patches/" + tc.lang + "/" + tc.file, tc.report}
		if !slices.Contains(patches[tc.lang], want) {
			t.Errorf("missing %+v in %s patches: %v", want, tc.lang, patches[tc.lang])
		}
	}
}

func TestPatchSectionDiagnostics(t *testing.T) {
	api := newAPI()
	var out bytes.Buffer
	patchSection(&out, api, map[string][]patchRec{"yaml": {{"p", "https://r/1"}}}, errors.New("grammar x: patches: bad"), nil, errors.New("shallow checkout"))
	for _, want := range []string{"Patch metadata unreadable (grammar x: patches: bad)", "Patch ages unavailable", "age not measured"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q:\n%s", want, out.String())
		}
	}
	if len(api.findings) != 0 { // missing data is a diagnostic here, not a tracking-issue warning
		t.Errorf("findings=%v", api.findings)
	}
	out.Reset()
	patchSection(&out, newAPI(), nil, nil, nil, nil)
	if !strings.Contains(out.String(), "No grammar carries a readable patch.") {
		t.Error(out.String())
	}
}

// gitFirstOnMain on a real repository: the first-parent merge that first added a report wins, a later rewrite of the
// patch or source repository does not restart it, and /388 does not match /3880.
func TestGitFirstOnMain(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	isolateGit(t)
	dir := t.TempDir()
	git := func(date string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE="+date, "GIT_AUTHOR_DATE="+date)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	pins := filepath.Join(dir, "pins.json")
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(pins, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const report = "https://github.com/tree-sitter/tree-sitter-javascript/pull/388"
	const origin = "https://github.com/tree-sitter/tree-sitter-javascript"
	const owner = "https://github.com/wotjr1649/tree-sitter-javascript"
	const onFeature = `{"grammars":{"javascript":{"repo":"` + origin + `","patches":[{"file":"old.patch","upstreamReport":"` + report + `"}]}}}`
	git("2025-01-01T00:00:00Z", "init", "-b", "main")
	write(`{"a":"` + report + `0"}`)
	git("2025-01-01T00:00:00Z", "add", ".")
	git("2025-01-01T00:00:00Z", "commit", "-m", "one")
	git("2025-02-01T00:00:00Z", "checkout", "-b", "feature")
	write(onFeature)
	git("2025-02-02T00:00:00Z", "commit", "-am", "branch work")
	git("2025-03-01T00:00:00Z", "checkout", "main")
	git("2025-03-01T00:00:00Z", "merge", "--no-ff", "-m", "merge", "feature")
	write(strings.Replace(onFeature, origin+`"`, owner+`"`, 1))
	git("2025-04-01T00:00:00Z", "commit", "-am", "move source")
	current := strings.Replace(strings.Replace(onFeature, origin+`"`, owner+`"`, 1), "old.patch", "new.patch", 1)
	write(current)
	git("2025-05-01T00:00:00Z", "commit", "-am", "rewrite")
	patches, err := loadPatches([]byte(current))
	if got := patches["javascript"]; err != nil || len(got) != 1 || got[0] != (patchRec{"new.patch", report}) {
		t.Fatalf("owner pin patch = %v, %v", got, err)
	}
	added, err := gitFirstOnMain(pins)
	if err != nil {
		t.Fatal(err)
	}
	for report, want := range map[string]string{report + "0": "2025-01-01", report: "2025-03-01"} {
		if got, err := added(report); err != nil || got.Format(time.DateOnly) != want {
			t.Errorf("%s: %v, %v; want %s", report, got, err, want)
		}
	}
	if _, err := added("https://r/unknown"); err == nil {
		t.Error("unknown report must be an error")
	}
	if _, err := gitFirstOnMain(filepath.Join(t.TempDir(), "pins.json")); err == nil {
		t.Error("a directory without a repository must be an error")
	}
}
