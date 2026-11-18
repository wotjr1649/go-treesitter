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

func activityServer(t *testing.T, graphql func(w http.ResponseWriter, body string)) (*httptest.Server, *int) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/graphql" {
			posts++
			if r.Header.Get("Authorization") != "Bearer secret-token" {
				t.Errorf("missing token header")
			}
			buf := new(bytes.Buffer)
			buf.ReadFrom(r.Body)
			graphql(w, buf.String())
			return
		}
		fmt.Fprintf(w, `{"full_name":%q}`, strings.TrimPrefix(r.URL.Path, "/repos/"))
	}))
	t.Cleanup(server.Close)
	return server, &posts
}

func activityPins() *pins {
	var p pins
	p.Grammars = map[string]struct{ Repo, Commit string }{
		"typescript": {"https://github.com/tree-sitter/tree-sitter-typescript", "pin"},
		"tsx":        {"https://github.com/tree-sitter/tree-sitter-typescript", "pin"},
		"bash":       {"https://github.com/tree-sitter/tree-sitter-bash", "pin"},
	}
	return &p
}

func TestActivitySection(t *testing.T) {
	server, posts := activityServer(t, func(w http.ResponseWriter, body string) {
		switch {
		case strings.Contains(body, "tree-sitter-bash"):
			fmt.Fprint(w, `{"errors":[{"message":"boom\n::error::leak | pipe"}],"data":null}`)
		case !strings.Contains(body, `merged:\u003e=2025-11-18`):
			t.Errorf("12-month window missing: %s", body)
		default: // 3 issues: a maintainer comment on the first (second comment), a contributor on the second, none on the third
			fmt.Fprint(w, `{"data":{"merged":{"issueCount":7},"repository":{"issues":{"nodes":[
				{"comments":{"nodes":[{"authorAssociation":"NONE"},{"authorAssociation":"COLLABORATOR"}]}},
				{"comments":{"nodes":[{"authorAssociation":"CONTRIBUTOR"}]}},
				{"comments":{"nodes":[]}}]}}}}`)
		}
	})
	api := newAPI()
	api.base, api.token, api.now = server.URL+"/", "secret-token", time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	var out bytes.Buffer
	activitySection(activityPins(), &out, api)
	for _, want := range []string{"| tsx, typescript | tree-sitter/tree-sitter-typescript | 7 | 1/3 |", "| bash | tree-sitter/tree-sitter-bash | ? | unavailable: GraphQL error |", "GraphQL requests: 2."} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q:\n%s", want, out.String())
		}
	}
	if *posts != 2 { // the shared repository is queried once
		t.Errorf("%d GraphQL posts, want 2", *posts)
	}
	if len(api.findings) != 0 || strings.Contains(out.String(), "::") || strings.Contains(out.String(), "secret-token") || strings.Contains(out.String(), "leak") {
		t.Errorf("activity must be information only: findings=%v\n%s", api.findings, out.String())
	}
}

func TestActivitySectionWithoutToken(t *testing.T) {
	api := newAPI()
	api.token = ""
	var out bytes.Buffer
	activitySection(activityPins(), &out, api)
	if api.requests != 0 || !strings.Contains(out.String(), "Skipped") || len(api.findings) != 0 {
		t.Errorf("requests=%d findings=%v\n%s", api.requests, api.findings, out.String())
	}
}

func reposPins(names ...string) *pins {
	var p pins
	p.Grammars = map[string]struct{ Repo, Commit string }{}
	for _, n := range names {
		p.Grammars[n] = struct{ Repo, Commit string }{"https://github.com/o/" + n, "pin"}
	}
	return &p
}

func activityAPI(server *httptest.Server) *githubAPI {
	api := newAPI()
	api.base, api.token, api.now = server.URL+"/", "secret-token", time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	return api
}

// Missing data is not a zero, and unread comments make the reply count a lower bound.
func TestActivityMissingDataAndLowerBound(t *testing.T) {
	for _, tc := range []struct{ name, data, want string }{
		{"null repository", `{"merged":{"issueCount":7},"repository":null}`, "| 7 | ? |"},
		{"missing issueCount", `{"merged":{},"repository":{"issues":{"nodes":[]}}}`, "| ? | 0/0 |"},
		{"unread comments", `{"merged":{"issueCount":0},"repository":{"issues":{"nodes":[
			{"comments":{"pageInfo":{"hasNextPage":true},"nodes":[{"authorAssociation":"NONE"}]}},
			{"comments":{"pageInfo":{"hasNextPage":true},"nodes":[{"authorAssociation":"OWNER"}]}}]}}}`, "| 0 | >=1/2 |"},
		{"all read", `{"merged":{"issueCount":0},"repository":{"issues":{"nodes":[
			{"comments":{"pageInfo":{"hasNextPage":false},"nodes":[{"authorAssociation":"NONE"}]}}]}}}`, "| 0 | 0/1 |"},
		{"nothing usable", `{"merged":{},"repository":null}`, "| ? | unavailable: incomplete data |"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := activityServer(t, func(w http.ResponseWriter, _ string) { fmt.Fprintf(w, `{"data":%s}`, tc.data) })
			var out bytes.Buffer
			activitySection(reposPins("a"), &out, activityAPI(server))
			if !strings.Contains(out.String(), tc.want) {
				t.Errorf("want %q:\n%s", tc.want, out.String())
			}
		})
	}
}

// After a GraphQL limit the remaining rows are skipped without a request; the REST limit is a separate state.
func TestActivityStopsAtGraphQLLimit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reply  func(w http.ResponseWriter)
		first  string
		reason string
	}{
		{"remaining 0", func(w http.ResponseWriter) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			fmt.Fprint(w, `{"data":{"merged":{"issueCount":1},"repository":null}}`)
		}, "| a | o/a | 1 | ? |", "skipped, GraphQL rate limit reached"},
		{"retry-after", func(w http.ResponseWriter) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "secondary limit: do not echo", http.StatusForbidden)
		}, "| a | o/a | ? | unavailable: HTTP 403 |", "skipped, GraphQL asked to retry later"},
		{"403 without headers", func(w http.ResponseWriter) {
			http.Error(w, "secondary limit: do not echo", http.StatusForbidden)
		}, "| a | o/a | ? | unavailable: HTTP 403 |", "skipped, GraphQL asked to retry later"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, posts := activityServer(t, func(w http.ResponseWriter, _ string) { tc.reply(w) })
			api := activityAPI(server)
			api.remaining = "0" // exhausted REST quota: the canonical names fall back to the pinned ones; GraphQL is unaffected
			var out bytes.Buffer
			activitySection(reposPins("a", "b", "c"), &out, api)
			for _, want := range []string{tc.first, "| b | o/b | ? | unavailable: " + tc.reason + " |", "| c | o/c | ? | unavailable: " + tc.reason + " |"} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("missing %q:\n%s", want, out.String())
				}
			}
			if *posts != 1 || strings.Contains(out.String(), "do not echo") {
				t.Errorf("%d posts; remote text must not be printed:\n%s", *posts, out.String())
			}
		})
	}
}

// A slow GraphQL endpoint costs at most the section's time budget; the rest is marked unavailable.
func TestActivityTimeBudget(t *testing.T) {
	server, posts := activityServer(t, func(w http.ResponseWriter, _ string) { time.Sleep(300 * time.Millisecond) })
	api := activityAPI(server)
	api.activityBudget = 30 * time.Millisecond
	var out bytes.Buffer
	start := time.Now()
	activitySection(reposPins("a", "b"), &out, api)
	if time.Since(start) > 250*time.Millisecond {
		t.Errorf("section took %v, budget 30ms", time.Since(start))
	}
	for _, want := range []string{"| a | o/a | ? | unavailable: time budget used |", "| b | o/b | ? | unavailable: skipped, time budget used |"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q:\n%s", want, out.String())
		}
	}
	if *posts != 1 {
		t.Errorf("%d posts, want 1", *posts)
	}
}
