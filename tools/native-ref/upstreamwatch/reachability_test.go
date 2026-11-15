package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPinReachability(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		failed     bool
	}{
		{"default-branch", "reachable from default branch main", false},
		{"release-tag-only", "reachable from release tag v1.0.0", false},
		{"branch-only", "reachable from branch release/23.x", false},
		{"older-tag-only", "reachable from release tag v0.9.0", false},
		{"branches-second-page", "reachable from branch release/23.x", false},
		{"tags-second-page", "reachable from release tag v0.9.0", false},
		{"orphan", "", false},
		{"listing-error", "", true},
		{"comparison-error", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := map[string]int{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls[r.URL.String()]++
				switch r.URL.Path {
				case "/repos/org/repo/compare/pin...head":
					if tc.name == "comparison-error" {
						http.Error(w, "failure", 503)
						return
					}
					if tc.name == "orphan" {
						http.NotFound(w, r)
						return
					}
					status := "diverged"
					if tc.name == "default-branch" {
						status = "ahead"
					}
					fmt.Fprintf(w, `{"status":%q}`, status)
				case "/repos/org/repo/compare/pin...v1.0.0":
					if tc.name == "orphan" {
						http.NotFound(w, r)
						return
					}
					status := "behind"
					if tc.name == "release-tag-only" {
						status = "identical"
					}
					fmt.Fprintf(w, `{"status":%q}`, status)
				case "/repos/org/repo/compare/pin...other":
					http.NotFound(w, r)
				case "/repos/org/repo/compare/pin...descendant":
					fmt.Fprint(w, `{"status":"ahead"}`)
				case "/repos/org/repo/branches", "/repos/org/repo/tags":
					if tc.name == "listing-error" {
						http.Error(w, "failure", 403)
						return
					}
					kind := strings.TrimPrefix(r.URL.Path, "/repos/org/repo/")
					if r.URL.Query().Get("per_page") != "100" {
						t.Error("missing pagination size")
					}
					page := r.URL.Query().Get("page")
					if page == "1" && ((tc.name == "branches-second-page" && kind == "branches") || (tc.name == "tags-second-page" && kind == "tags")) {
						refs := make([]map[string]any, 100)
						for i := range refs {
							refs[i] = map[string]any{"name": fmt.Sprintf("old-%d", i), "commit": map[string]string{"sha": "other"}}
						}
						json.NewEncoder(w).Encode(refs)
					} else if kind == "branches" && (tc.name == "branch-only" || (tc.name == "branches-second-page" && page == "2")) {
						fmt.Fprint(w, `[{"name":"release/23.x","commit":{"sha":"descendant"}}]`)
					} else if kind == "tags" && (tc.name == "older-tag-only" || (tc.name == "tags-second-page" && page == "2")) {
						fmt.Fprint(w, `[{"name":"v0.9.0","commit":{"sha":"descendant"}}]`)
					} else if tc.name == "orphan" {
						fmt.Fprint(w, `[{"name":"unrelated","commit":{"sha":"other"}}]`)
					} else {
						fmt.Fprint(w, `[]`)
					}
				default:
					t.Errorf("unexpected request %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			api := newAPI()
			api.base, api.token = server.URL+"/", ""
			got, err := pinReachability(api, "repos/org/repo", "pin", "main", "head", "v1.0.0")
			if (err != nil) != tc.failed || got != tc.want {
				t.Fatalf("reachability = %q, %v; want %q, failed=%v", got, err, tc.want, tc.failed)
			}
			if !tc.failed {
				pinReachability(api, "repos/org/repo", "pin", "main", "head", "v1.0.0")
				for path, n := range calls {
					if n != 1 {
						t.Errorf("uncached request %s: %d calls", path, n)
					}
				}
			}
		})
	}
}
