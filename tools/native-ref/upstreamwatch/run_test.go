package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const runPins = `{"runtime":{"release":"v1.0.0","commit":"rp","repo":"https://github.com/tree-sitter/tree-sitter"},
"tools":{"wasi-sdk":"1.0","binaryen":"1","wasm2go":"v1.0.0","wasm2goModule":"github.com/ncruces/wasm2go","treeSitterCLI":"1.0.0","llvmCommit":"lp"},
"wasiLibc":{"repo":"https://github.com/WebAssembly/wasi-libc","commit":"cp"},
"grammars":{"bash":{"repo":"https://github.com/tree-sitter/tree-sitter-bash","commit":"gp"%s}}}`

// runWatch runs the command against a fake API where every pin is healthy and current.
func runWatch(t *testing.T, pinsJSON string, graphql func(w http.ResponseWriter)) (code int, state, issue, stdout string, api *githubAPI) {
	t.Helper()
	isolateGit(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/graphql":
			graphql(w)
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			fmt.Fprint(w, `{"tag_name":"v1.0.0"}`)
		case strings.Contains(r.URL.Path, "/commits/"):
			fmt.Fprint(w, `{"sha":"head","commit":{"committer":{"date":"2026-11-01T00:00:00Z"}}}`)
		case strings.Contains(r.URL.Path, "/compare/"):
			fmt.Fprint(w, `{"status":"identical"}`)
		default:
			fmt.Fprintf(w, `{"full_name":%q,"default_branch":"main"}`, strings.TrimPrefix(r.URL.Path, "/repos/"))
		}
	}))
	t.Cleanup(server.Close)
	dir := t.TempDir()
	pinsFile, stateFile, issueFile := filepath.Join(dir, "pins.json"), filepath.Join(dir, "state"), filepath.Join(dir, "body.md")
	if err := os.WriteFile(pinsFile, []byte(pinsJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	api = newAPI()
	api.base, api.token, api.now, api.activityBudget = server.URL+"/", "secret-token", time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC), 30*time.Millisecond
	var out bytes.Buffer
	code = run([]string{"-pins", pinsFile, "-nvim", "", "-issue", issueFile, "-state", stateFile}, &out, api)
	read := func(p string) string { b, _ := os.ReadFile(p); return string(b) }
	return code, strings.TrimSpace(read(stateFile)), read(issueFile), out.String(), api
}

func okGraphQL(w http.ResponseWriter) {
	fmt.Fprint(w, `{"data":{"merged":{"issueCount":3},"repository":{"issues":{"nodes":[]}}}}`)
}

// Whatever the activity lookups do, the state file and the issue body are written and the outcome is unchanged.
func TestRunActivityCannotPreventOutputs(t *testing.T) {
	for name, graphql := range map[string]func(http.ResponseWriter){
		"slow": func(w http.ResponseWriter) { time.Sleep(300 * time.Millisecond) },
		"rate limit": func(w http.ResponseWriter) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			http.Error(w, "limit", http.StatusForbidden)
		},
		"server error": func(w http.ResponseWriter) { http.Error(w, "boom", http.StatusBadGateway) },
		"fine":         okGraphQL,
	} {
		t.Run(name, func(t *testing.T) {
			code, state, issue, stdout, _ := runWatch(t, fmt.Sprintf(runPins, ""), graphql)
			if code != 0 || state != "clean" || !strings.Contains(issue, "Upstream activity") {
				t.Fatalf("code=%d state=%q issue=%q\n%s", code, state, issue, stdout)
			}
			if strings.Index(stdout, "Repository health") > strings.Index(stdout, "Upstream activity") {
				t.Error("activity must come after the health checks")
			}
			if strings.Contains(stdout, "No grammar carries") == false {
				t.Errorf("a pins file without patches:\n%s", stdout)
			}
		})
	}
}

func TestRunPatchMetadata(t *testing.T) {
	// patches: {} used to be a new exit 2; now only a diagnostic, with the flow (issue body, state) unchanged.
	code, state, issue, _, api := runWatch(t, fmt.Sprintf(runPins, `,"patches":{}`), okGraphQL)
	if code != 0 || state != "clean" || !strings.Contains(issue, "Patch metadata unreadable") || len(api.findings) != 0 {
		t.Errorf("code=%d state=%q findings=%v\n%s", code, state, api.findings, issue)
	}
	// The existing pins parse error path stays a hard error that writes nothing.
	code, state, issue, _, _ = runWatch(t, `{"grammars":`, okGraphQL)
	if code != 2 || state != "" || issue != "" {
		t.Errorf("code=%d state=%q issue=%q", code, state, issue)
	}
	// Unusable git history (here: no repository) is a section diagnostic, not a warning that changes the state.
	code, state, issue, _, api = runWatch(t, fmt.Sprintf(runPins, `,"patches":[{"file":"patches/bash/1.patch","upstreamReport":"https://r/1"}]`), okGraphQL)
	if code != 0 || state != "clean" || !strings.Contains(issue, "Patch ages unavailable") || len(api.findings) != 0 {
		t.Errorf("code=%d state=%q findings=%v\n%s", code, state, api.findings, issue)
	}
}
