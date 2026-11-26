package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// tagServer answers like a healthy repository: release v1.0.0 and a head the pin trails by 3 commits unless the
// repository name says otherwise; tags maps a repository path to its tags JSON.
func tagServer(t *testing.T, release string, tags map[string]string, compare func(path string) string) *githubAPI {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			fmt.Fprintf(w, `{"tag_name":%q}`, release)
		case strings.HasSuffix(r.URL.Path, "/tags"):
			if body, ok := tags[strings.TrimSuffix(r.URL.Path, "/tags")]; ok {
				fmt.Fprint(w, body)
			} else {
				fmt.Fprint(w, `[]`)
			}
		case strings.HasSuffix(r.URL.Path, "/branches"):
			fmt.Fprint(w, `[]`)
		case strings.Contains(r.URL.Path, "/commits/"):
			fmt.Fprint(w, `{"sha":"head","commit":{"committer":{"date":"2026-11-01T00:00:00Z"}}}`)
		case strings.Contains(r.URL.Path, "/compare/"):
			fmt.Fprint(w, compare(r.URL.Path))
		default:
			fmt.Fprintf(w, `{"full_name":%q,"default_branch":"main"}`, strings.TrimPrefix(r.URL.Path, "/repos/"))
		}
	}))
	t.Cleanup(server.Close)
	api := newAPI()
	api.base, api.token, api.now = server.URL+"/", "", time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	return api
}

func tagPins(grammars map[string]struct{ Repo, Commit string }) *pins {
	var p pins
	p.Runtime.Repo, p.Runtime.Release, p.Runtime.Commit = "https://github.com/tree-sitter/tree-sitter", "v2.0.0", "runtime-pin"
	p.Tools.WasiSDK, p.Tools.Binaryen, p.Tools.Wasm2go, p.Tools.Module = "2.0", "2", "v2.0.0", "github.com/ncruces/wasm2go"
	p.Tools.CLI = "2.0.0"
	p.WasiLibc.Repo, p.WasiLibc.Commit = "https://github.com/WebAssembly/wasi-libc", "libc-pin"
	p.Grammars = grammars
	return &p
}

func TestNewestTag(t *testing.T) {
	api := tagServer(t, "", map[string]string{"/repos/o/r": `[{"name":"v0.4.9"},{"name":"v0.4.22"},{"name":"v0.4.23-rc1"},{"name":"v0.5.0-beta.2"},{"name":"nightly"},{"name":"v0.4.21"},{"name":"other-9.9.9"}]`}, nil)
	if tag, err := newestTag(api, "o/r", "v0.4.21"); err != nil || tag != "v0.4.22" {
		t.Errorf("tag=%q err=%v, want v0.4.22 (pre-releases, other prefixes and numeric order)", tag, err)
	}
	if tag, err := newestTag(api, "o/none", "v1.0.0"); err != nil || tag != "" {
		t.Errorf("no tags: tag=%q err=%v", tag, err)
	}
}

// wasm2go v0.4.22 had a tag and no GitHub Release: releases/latest still said v0.4.21.
func TestReportTagWithoutRelease(t *testing.T) {
	for _, tc := range []struct {
		name, tags string
		stale      bool
	}{
		{"tag-only", `[{"name":"v0.4.22"},{"name":"v0.4.21"}]`, true},
		{"pinned-tag-is-newest", `[{"name":"v0.4.21"},{"name":"v0.4.20"}]`, false},
		{"only-a-prerelease-tag", `[{"name":"v0.4.22-rc1"}]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := tagServer(t, "v0.4.21", map[string]string{"/repos/ncruces/wasm2go": tc.tags}, func(string) string { return `{"status":"identical"}` })
			p := tagPins(nil)
			p.Runtime.Release, p.Tools.Wasm2go, p.Tools.CLI = "v0.4.21", "v0.4.21", "0.4.21" // the fake answers v0.4.21 for every tool
			var out bytes.Buffer
			stale, failed := report(p, &out, api)
			if failed || stale != tc.stale {
				t.Fatalf("stale=%v failed=%v\n%s", stale, failed, &out)
			}
			if got := strings.Contains(out.String(), "| wasm2go | v0.4.21 | v0.4.22 | **NEWER STABLE TAG WITHOUT A RELEASE** |"); got != tc.stale {
				t.Errorf("table row: %v\n%s", got, &out)
			}
		})
	}
}

func TestReportTagsAPIFailureFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/tags") {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		fmt.Fprint(w, `{"tag_name":"v1.0.0"}`)
	}))
	defer server.Close()
	api := newAPI()
	api.base = server.URL + "/"
	var out bytes.Buffer
	if _, failed := report(tagPins(nil), &out, api); !failed || !strings.Contains(out.String(), "**ERROR** tags:") {
		t.Fatalf("failed=%v\n%s", failed, &out)
	}
}

func TestGrammarPinBehindDefaultBranch(t *testing.T) {
	compare := func(string) string { return `{"status":"ahead","ahead_by":3}` }
	api := tagServer(t, "v1.0.0", nil, func(path string) string {
		if strings.HasSuffix(path, "...v1.0.0") {
			return `{"status":"behind"}`
		}
		return compare(path)
	})
	p := tagPins(map[string]struct{ Repo, Commit string }{
		"owned": {"https://github.com/wotjr1649/tree-sitter-owned", "pin"},
		"third": {"https://github.com/someone/tree-sitter-third", "pin"},
	})
	var out bytes.Buffer
	if stale, failed := report(p, &out, api); stale || failed {
		t.Fatalf("stale=%v failed=%v\n%s", stale, failed, &out)
	}
	if len(api.findings) != 1 || api.findings[0].component != "owned" || api.findings[0].kind != "pin behind default branch" ||
		!strings.Contains(api.findings[0].text, "3 commit(s) behind main") {
		t.Fatalf("only the owner repository warns: %v", api.findings)
	}
	if !strings.Contains(out.String(), "| third | someone/tree-sitter-third |") || !strings.Contains(out.String(), "pin is 3 commit(s) behind main") {
		t.Errorf("the third-party distance is reported without a warning:\n%s", &out)
	}
}

func TestOwnerGrammarTagWithoutRelease(t *testing.T) {
	for _, tc := range []struct {
		name, repo, release, status string
		warn                        bool
	}{
		{"owner, newer tag", "wotjr1649/tree-sitter-owned", "v1.0.0", "ahead", true},
		{"owner, tag does not contain the pin", "wotjr1649/tree-sitter-owned", "v1.0.0", "diverged", false},
		{"owner, release is the newest tag", "wotjr1649/tree-sitter-owned", "v2.0.0", "ahead", false},
		{"third party stays silent", "someone/tree-sitter-third", "v1.0.0", "ahead", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := tagServer(t, tc.release, map[string]string{"/repos/" + tc.repo: `[{"name":"v2.0.0"},{"name":"v1.0.0"}]`}, func(path string) string {
				if strings.HasSuffix(path, "...v2.0.0") {
					return fmt.Sprintf(`{"status":%q}`, tc.status)
				}
				return `{"status":"identical"}`
			})
			p := tagPins(map[string]struct{ Repo, Commit string }{"g": {"https://github.com/" + tc.repo, "pin"}})
			var out bytes.Buffer
			if stale, failed := report(p, &out, api); stale || failed {
				t.Fatalf("stale=%v failed=%v\n%s", stale, failed, &out)
			}
			warned := 0
			for _, f := range api.findings {
				if strings.HasPrefix(f.kind, "newer tag ") {
					warned++
				}
			}
			if (warned == 1) != tc.warn || warned > 1 {
				t.Errorf("tag warnings=%d, want warn=%v: %v", warned, tc.warn, api.findings)
			}
		})
	}
}
