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

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		pinned, latest string
		want           bool
	}{
		{"v0.27.2", "v0.27.2", false},
		{"v0.27.2", "v0.27.3", true},
		{"v0.27.2", "v0.28.0", true},
		{"v0.27.2", "v0.9.0", false},
		{"v0.27.2", "v1.0.0", true},
		{"34.0", "wasi-sdk-34", false},
		{"34.0", "wasi-sdk-35", true},
		{"34.0", "wasi-sdk-34.1", true},
		{"133", "version_133", false},
		{"133", "version_134", true},
		{"133", "version_99", false},
		{"v0.4.21", "v0.4.21", false},
		{"v0.4.21", "v0.4.100", true},
	} {
		if got := newer(c.pinned, c.latest); got != c.want {
			t.Errorf("newer(%q, %q) = %v, want %v", c.pinned, c.latest, got, c.want)
		}
	}
}

func TestRepositoryHealth(t *testing.T) {
	for _, tc := range []struct {
		name, comparison      string
		status                int
		archived, old, rename bool
		want                  string
		failed                bool
	}{
		{name: "identical", comparison: "identical", want: "reachable from default branch main"},
		{name: "ahead", comparison: "ahead", want: "reachable from default branch main"},
		{name: "diverged", comparison: "diverged", want: "pin unreachable from every branch and tag", failed: true},
		{name: "behind", comparison: "behind", want: "pin unreachable from every branch and tag", failed: true},
		{name: "release-tag-only", comparison: "diverged", want: "reachable from release tag v1.0.0"},
		{name: "orphan", status: 404, want: "pin unreachable from every branch and tag", failed: true},
		{name: "deleted", status: 410, want: "repository missing/deleted", failed: true},
		{name: "missing", status: 404, comparison: "missing", want: "repository missing/deleted", failed: true},
		{name: "rename", comparison: "ahead", rename: true, want: "old/repo → new/repo"},
		{name: "archived", comparison: "ahead", archived: true, want: "WARNING archived"},
		{name: "age", comparison: "ahead", old: true, want: "tier review (>12 months)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("X-RateLimit-Remaining", "123")
				switch r.URL.Path {
				case "/repos/old/repo":
					if tc.name == "missing" {
						w.WriteHeader(404)
						return
					}
					if tc.name == "deleted" {
						w.WriteHeader(410)
						return
					}
					if tc.rename {
						http.Redirect(w, r, "/repos/new/repo", http.StatusMovedPermanently)
						return
					}
					fmt.Fprintf(w, `{"full_name":"old/repo","default_branch":"main","archived":%t}`, tc.archived)
				case "/repos/new/repo":
					fmt.Fprint(w, `{"full_name":"new/repo","default_branch":"main"}`)
				case "/repos/old/repo/commits/main", "/repos/new/repo/commits/main":
					date := "2026-01-01T00:00:00Z"
					if tc.old {
						date = "2025-01-01T00:00:00Z"
					}
					fmt.Fprintf(w, `{"sha":"head","commit":{"committer":{"date":%q}}}`, date)
				case "/repos/old/repo/compare/pin...head", "/repos/new/repo/compare/pin...head":
					if tc.status != 0 {
						w.WriteHeader(tc.status)
						return
					}
					fmt.Fprintf(w, `{"status":%q}`, tc.comparison)
				case "/repos/old/repo/releases/latest", "/repos/new/repo/releases/latest":
					fmt.Fprint(w, `{"tag_name":"v1.0.0"}`)
				case "/repos/old/repo/compare/pin...v1.0.0", "/repos/new/repo/compare/pin...v1.0.0":
					if tc.name == "release-tag-only" {
						fmt.Fprint(w, `{"status":"identical"}`)
					} else {
						fmt.Fprint(w, `{"status":"behind"}`)
					}
				case "/repos/old/repo/branches", "/repos/new/repo/branches", "/repos/old/repo/tags", "/repos/new/repo/tags":
					fmt.Fprint(w, `[]`)
				default:
					t.Errorf("unexpected request %s", r.URL)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			api := newAPI()
			api.base, api.token, api.now = server.URL+"/", "", time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
			var out bytes.Buffer
			if failed := health(api, &out, source{"tsql", "old/repo", "pin", false}); failed != tc.failed {
				t.Fatalf("failed = %v, want %v: %s", failed, tc.failed, out.String())
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Errorf("missing %q: %s", tc.want, out.String())
			}
			comparison := "behind"
			if tc.name == "release-tag-only" {
				comparison = "identical"
			}
			if tc.name != "missing" && tc.name != "deleted" && !strings.Contains(out.String(), "v1.0.0 vs pin ("+comparison+")") {
				t.Error("latest release comparison missing")
			}
			before := requests
			health(api, &out, source{"shared pin", "old/repo", "pin", false})
			if requests != before {
				t.Errorf("shared pin made %d extra requests", requests-before)
			}
		})
	}
}

func TestRateLimitStopsRequests(t *testing.T) {
	api := newAPI()
	api.remaining = "0"
	if err := api.get("repos/example/repo", &struct{}{}); err == nil || api.requests != 0 {
		t.Fatalf("exhausted rate limit: err=%v requests=%d", err, api.requests)
	}
}

func TestReportCoversAllPinsAndCachesSharedRepositories(t *testing.T) {
	requests := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests[r.URL.String()]++
		switch {
		case strings.HasSuffix(r.URL.Path, "/tags"), strings.HasSuffix(r.URL.Path, "/branches"):
			fmt.Fprint(w, `[]`)
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			fmt.Fprint(w, `{"tag_name":"v1.0.0"}`)
		case strings.Contains(r.URL.Path, "/commits/"):
			fmt.Fprint(w, `{"sha":"head","commit":{"committer":{"date":"2026-11-01T00:00:00Z"}}}`)
		case strings.Contains(r.URL.Path, "/compare/"):
			fmt.Fprint(w, `{"status":"ahead","ahead_by":1}`)
		default:
			fmt.Fprintf(w, `{"full_name":%q,"default_branch":"main"}`, strings.TrimPrefix(r.URL.Path, "/repos/"))
		}
	}))
	defer server.Close()
	var p pins
	p.Runtime.Repo, p.Runtime.Release, p.Runtime.Commit = "https://github.com/tree-sitter/tree-sitter", "v1.0.0", "runtime-pin"
	p.Tools.WasiSDK, p.Tools.Binaryen, p.Tools.Wasm2go = "1.0", "1", "v1.0.0"
	p.Tools.Module, p.Tools.CLI, p.Tools.LLVM = "github.com/ncruces/wasm2go", "1.0.0", "llvm-pin"
	p.WasiLibc.Repo, p.WasiLibc.Commit = "https://github.com/WebAssembly/wasi-libc", "libc-pin"
	p.Grammars = map[string]struct{ Repo, Commit string }{
		"typescript": {"https://github.com/tree-sitter/tree-sitter-typescript", "grammar-pin"},
		"tsx":        {"https://github.com/tree-sitter/tree-sitter-typescript", "grammar-pin"},
	}
	api := newAPI()
	api.base, api.token = server.URL+"/", ""
	var out bytes.Buffer
	if stale, failed := report(&p, &out, api); stale || failed {
		t.Fatalf("stale=%v failed=%v: %s", stale, failed, out.String())
	}
	for _, name := range []string{"tree-sitter runtime", "tree-sitter CLI", "wasi-sdk", "binaryen", "wasm2go", "LLVM (wasi-sdk clang)", "wasi-libc", "typescript", "tsx"} {
		if !strings.Contains(out.String(), "| "+name+" |") {
			t.Errorf("missing %s", name)
		}
	}
	for path, count := range requests {
		if count != 1 {
			t.Errorf("%s requested %d times", path, count)
		}
	}
}
