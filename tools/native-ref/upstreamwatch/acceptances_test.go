package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAcceptedWarnings(t *testing.T) {
	entry := acceptance{"tsql", "newer stable release v1.0.0 than pin", "2026-11-16", "2027-05-16", "[decision](https://example.test/decision)"}
	current := finding{"tsql", "newer stable release v1.0.0 than pin 123456789abc", entry.Kind}
	for _, tc := range []struct {
		name, now string
		findings  []finding
		warnings  int
		accepted  bool
		stale     bool
	}{
		{"accepted", "2026-11-16", []finding{current}, 0, true, false},
		{"last-day", "2027-05-15", []finding{current}, 0, true, false},
		{"expired", "2027-05-16", []finding{current}, 1, false, false},
		{"after-expiry", "2027-05-17", []finding{current}, 1, false, false},
		{"future-acceptance", "2026-11-15", []finding{current}, 1, false, false},
		{"stale", "2026-11-16", nil, 0, false, true},
		{"unaccepted-kind", "2026-11-16", []finding{{"tsql", "repository archived", "repository archived"}}, 1, false, true},
		{"unaccepted-component", "2026-11-16", []finding{{"python", current.text, current.kind}}, 1, false, true},
		{"new-release", "2026-11-16", []finding{{"tsql", "newer stable release v1.1.0 than pin 123456789abc", "newer stable release v1.1.0 than pin"}}, 1, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := newAPI()
			api.now, _ = time.Parse(time.DateOnly, tc.now)
			api.acceptances, api.findings = []acceptance{entry}, tc.findings
			var out bytes.Buffer
			if n := warningSummary(&out, api); n != tc.warnings {
				t.Fatalf("warnings=%d, want %d: %s", n, tc.warnings, &out)
			}
			body := out.String()
			accepted, stale, _ := strings.Cut(strings.SplitN(body, "## Accepted warnings", 2)[1], "## Stale acceptances")
			if strings.Contains(accepted, current.text) != tc.accepted || strings.Contains(stale, entry.Kind) != tc.stale {
				t.Fatalf("accepted=%v stale=%v: %s", tc.accepted, tc.stale, body)
			}
			if tc.accepted && !strings.Contains(accepted, entry.Expires) {
				t.Fatal("accepted warning lost its expiry")
			}
			if len(tc.findings) > 0 {
				var annotations bytes.Buffer
				f := tc.findings[0]
				api.warn(&annotations, f.component, f.kind, f.text)
				if strings.HasPrefix(annotations.String(), "::notice::") != tc.accepted {
					t.Fatalf("wrong annotation: %s", &annotations)
				}
			}
		})
	}
}

func TestCheckedInAcceptances(t *testing.T) {
	entries, err := loadAcceptances(acceptedWarningsJSON)
	if err != nil || len(entries) != 9 {
		t.Fatalf("checked-in list: %d entries, %v", len(entries), err)
	}
	for _, a := range entries {
		if a.Accepted != "2026-11-16" || a.Expires != "2027-05-16" {
			t.Fatalf("unexpected initial dates: %+v", a)
		}
	}
	for _, name := range []string{"component", "kind", "accepted", "expires", "reason", "duplicate", "unknown", "trailing"} {
		t.Run(name, func(t *testing.T) {
			var data []map[string]any
			if err := json.Unmarshal(acceptedWarningsJSON, &data); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "duplicate":
				data = append(data, data[0])
			case "unknown":
				data[0]["typo"] = "field"
			case "trailing":
			default:
				data[0][name] = ""
			}
			raw, _ := json.Marshal(data)
			if name == "trailing" {
				raw = append(raw, []byte(" []")...)
			}
			if _, err := loadAcceptances(raw); err == nil {
				t.Fatalf("invalid %s accepted", name)
			}
		})
	}
}

func TestAcceptedWarningIssueBody(t *testing.T) {
	api := newAPI()
	api.now = time.Date(2026, 11, 16, 1, 2, 0, 0, time.UTC)
	api.acceptances, _ = loadAcceptances(acceptedWarningsJSON)
	api.findings = []finding{{"css", "last commit 2025-09-28; tier review (>12 months)", "tier review (>12 months)"}}
	path := filepath.Join(t.TempDir(), "body.md")
	report := bytes.NewBufferString("## Fixture report\n::notice::css: accepted\n")
	if err := writeIssue(path, report, api, false); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"**0 warning(s)**", "## Accepted warnings", "| css | last commit 2025-09-28;", "| 2026-11-16 | 2027-05-16 |", "## Stale acceptances", "| tsql | newer stable release v1.0.0 than pin |"} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("issue body lacks %q: %s", want, body)
		}
	}
	if bytes.Contains(body, []byte("::notice::")) {
		t.Fatal("annotation leaked into issue body")
	}
}

func TestWatchReportWithAcceptances(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			fmt.Fprint(w, `{"tag_name":"v1.0.0"}`)
		case strings.Contains(r.URL.Path, "/commits/"):
			date := "2026-11-01T00:00:00Z"
			if strings.HasPrefix(r.URL.Path, "/repos/fixture/") && !strings.HasPrefix(r.URL.Path, "/repos/fixture/tsql/") {
				date = "2025-01-01T00:00:00Z"
			}
			fmt.Fprintf(w, `{"sha":"head","commit":{"committer":{"date":%q}}}`, date)
		case strings.Contains(r.URL.Path, "/compare/"):
			status := "identical"
			if strings.HasPrefix(r.URL.Path, "/repos/fixture/tsql/") {
				status = "ahead"
			}
			fmt.Fprintf(w, `{"status":%q}`, status)
		default:
			fmt.Fprintf(w, `{"full_name":%q,"default_branch":"main"}`, strings.TrimPrefix(r.URL.Path, "/repos/"))
		}
	}))
	defer server.Close()
	var p pins
	p.Runtime.Repo, p.Runtime.Release, p.Runtime.Commit = "https://github.com/tree-sitter/tree-sitter", "v1.0.0", "runtime-pin"
	p.Tools.WasiSDK, p.Tools.Binaryen, p.Tools.Wasm2go, p.Tools.Module = "1.0", "1", "v1.0.0", "github.com/ncruces/wasm2go"
	p.WasiLibc.Repo, p.WasiLibc.Commit = "https://github.com/WebAssembly/wasi-libc", "libc-pin"
	p.Grammars = map[string]struct{ Repo, Commit string }{}
	api := newAPI()
	api.base, api.token, api.now = server.URL+"/", "", time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	var err error
	api.acceptances, err = loadAcceptances(acceptedWarningsJSON)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range api.acceptances {
		p.Grammars[a.Component] = struct{ Repo, Commit string }{"https://github.com/fixture/" + a.Component, "grammar-pin"}
	}
	var out, summary bytes.Buffer
	stale, failed := report(&p, &out, api)
	if stale || failed || len(api.findings) != 9 || warningSummary(&summary, api) != 0 {
		t.Fatalf("stale=%v failed=%v findings=%v\n%s\n%s", stale, failed, api.findings, &out, &summary)
	}
	path := filepath.Join(t.TempDir(), "body.md")
	if err := writeIssue(path, &out, api, failed); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Offline fixture: 9 current warnings accepted, 0 unaccepted; auto-close eligible.\n%s", body)
	api.warn(&out, "tsql", "repository archived", "repository archived")
	summary.Reset()
	if warningSummary(&summary, api) != 1 {
		t.Fatalf("an unrelated warning was suppressed: %s", &summary)
	}
}
