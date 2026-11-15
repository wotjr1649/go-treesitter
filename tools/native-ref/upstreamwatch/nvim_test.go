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

// A trimmed excerpt of lua/nvim-treesitter/parsers.lua plus hostile lines that must be ignored.
const parsersFixture = `---@type nvim-ts.parsers
return {
  bash = {
    install_info = {
      revision = 'a06c2e4415e9bc0346c6b86d401879ffb44058f7',
      url = 'https://github.com/tree-sitter/tree-sitter-bash',
    },
    maintainers = { '@TravonteD' },
    tier = 1,
  },
  c_sharp = {
    install_info = {
      revision = 'v0.23.5',
      url = 'https://github.com/tree-sitter/tree-sitter-c-sharp',
    },
    tier = 2,
  },
  tsx = {
    install_info = {
      location = 'tsx',
      revision = '75b3874edb2dc714fb1fd77a32013d0f8699989f',
      url = 'https://github.com/tree-sitter/tree-sitter-typescript.git',
    },
    requires = { 'html' },
  },
  sql = {
    install_info = {
      branch = 'gh-pages',
      revision = '2d088525f3f986108d969dab526a0d7129281e24',
      url = 'https://github.com/derekstride/tree-sitter-sql',
      url = 'https://github.com/second/wins-not',
    },
  },
  evil = {
    install_info = {
      revision = "x'; rm -rf /",
      url = 'http://example.com/not-github',
    },
  },
  nourl = {
    install_info = {
      revision = 'abc',
    },
  },
  badrev = {
    install_info = {
      revision = 'bad rev with spaces',
      url = 'https://github.com/o/badrev',
    },
  },
}
`

func TestParseNvim(t *testing.T) {
	for _, text := range []string{parsersFixture, strings.ReplaceAll(parsersFixture, "\n", "\r\n")} {
		got, err := parseNvim(text)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]nvimEntry{
			"bash":    {"tree-sitter/tree-sitter-bash", "a06c2e4415e9bc0346c6b86d401879ffb44058f7"},
			"c_sharp": {"tree-sitter/tree-sitter-c-sharp", "v0.23.5"},
			"tsx":     {"tree-sitter/tree-sitter-typescript", "75b3874edb2dc714fb1fd77a32013d0f8699989f"},
			"sql":     {"derekstride/tree-sitter-sql", "2d088525f3f986108d969dab526a0d7129281e24"},
			"badrev":  {"o/badrev", ""},
		}
		if len(got) != len(want) {
			t.Errorf("parsed %d entries, want %d: %v", len(got), len(want), got)
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s = %v, want %v", k, got[k], v)
			}
		}
	}
	for _, text := range []string{"", "<html>not lua</html>", "return {}"} {
		if _, err := parseNvim(text); err == nil {
			t.Errorf("parseNvim(%q) succeeded", text)
		}
	}
}

func TestFetchNvim(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			fmt.Fprint(w, parsersFixture)
		case "/big":
			w.Write(bytes.Repeat([]byte("-- padding\n"), nvimMaxBytes/10))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()
	if got, err := fetchNvim(server.Client(), server.URL+"/ok"); err != nil || len(got) != 5 {
		t.Errorf("ok: %d entries, err %v", len(got), err)
	}
	for _, path := range []string{"/big", "/error"} {
		if _, err := fetchNvim(server.Client(), server.URL+path); err == nil {
			t.Errorf("%s: want an error", path)
		}
	}
}

// fakeAPI serves the repositories and commits of one scenario; compare answers by the nvim sha before "...".
func fakeAPI(t *testing.T, status map[string]string) *githubAPI {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/repos/old/name":
			fmt.Fprint(w, `{"full_name":"new/name"}`)
		case strings.Contains(path, "/commits/v1"):
			fmt.Fprint(w, `{"sha":"1111111111111111111111111111111111111111"}`)
		case strings.Contains(path, "/commits/gone"):
			w.WriteHeader(404)
		case strings.Contains(path, "/compare/"):
			from := strings.SplitN(strings.SplitN(path, "/compare/", 2)[1], "...", 2)[0]
			if s, ok := status[from]; ok {
				fmt.Fprintf(w, `{"status":%q}`, s)
				return
			}
			w.WriteHeader(404)
		case strings.Count(path, "/") == 3: // /repos/owner/name
			fmt.Fprintf(w, `{"full_name":%q}`, strings.TrimPrefix(path, "/repos/"))
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	api := newAPI()
	api.base, api.token, api.now = server.URL+"/", "", time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	return api
}

func TestNvimSection(t *testing.T) {
	sha := func(c string) string { return strings.Repeat(c, 40) }
	var p pins
	p.Grammars = map[string]struct{ Repo, Commit string }{
		"same":       {"https://github.com/o/same", sha("a")},
		"golang":     {"https://github.com/o/go", sha("b")}, // nvim name: go
		"other":      {"https://github.com/o/ours", sha("f")},
		"moved":      {"https://github.com/old/name", sha("a")},
		"tag":        {"https://github.com/o/tag", sha("1")}, // v1 resolves to sha("1")
		"gone":       {"https://github.com/o/gone", sha("a")},
		"norev":      {"https://github.com/o/norev", sha("a")},
		"tsql":       {"https://github.com/o/tsql", sha("a")}, // not in nvim-treesitter
		"unknowncmp": {"https://github.com/o/unknowncmp", sha("a")},
	}
	entries := map[string]nvimEntry{
		"same":       {"o/same", sha("a")},
		"go":         {"o/go", sha("b")},
		"other":      {"o/theirs", sha("f")},
		"moved":      {"new/name", sha("a")}, // we pin the old name of a transferred repository
		"tag":        {"o/tag", "v1"},
		"gone":       {"o/gone", "gone"},
		"norev":      {"o/norev", ""},
		"unknowncmp": {"o/unknowncmp", sha("8")},
	}
	api := fakeAPI(t, map[string]string{sha("8"): "weird"})
	var out bytes.Buffer
	nvimSection(&p, &out, api, entries, nil)
	got := map[string]string{}
	for _, line := range strings.Split(out.String(), "\n") {
		if cells := strings.Split(line, " | "); len(cells) == 4 {
			got[strings.TrimPrefix(cells[0], "| ")] = strings.TrimSuffix(cells[3], " |")
		}
	}
	warned := map[string]bool{}
	for _, f := range api.findings {
		warned[f.component] = true
	}
	for name, want := range map[string]struct {
		result string
		warn   bool
	}{
		"same":       {"same", false},
		"golang":     {"same", false},
		"moved":      {"same", false},
		"tag":        {"same", false},
		"tsql":       {"not in nvim-treesitter", false},
		"other":      {"different repository", true},
		"gone":       {"unresolved", true},
		"norev":      {"unresolved", true},
		"unknowncmp": {"unknown** comparison status weird", true},
	} {
		if !strings.Contains(got[name], want.result) || warned[name] != want.warn {
			t.Errorf("%s: result %q warned=%v, want %q warned=%v", name, got[name], warned[name], want.result, want.warn)
		}
	}
	if !strings.Contains(out.String(), "::warning::other: nvim-treesitter: **different repository**") {
		t.Errorf("warning annotation missing: %s", out.String())
	}
}

func TestNvimComparisonDirections(t *testing.T) {
	api := fakeAPI(t, map[string]string{"9999999999999999999999999999999999999999": "ahead", "8888888888888888888888888888888888888888": "behind", "7777777777777777777777777777777777777777": "diverged"})
	pin := strings.Repeat("a", 40)
	for _, tc := range []struct {
		nvim, want string
		warn       bool
	}{
		{strings.Repeat("9", 40), "we are ahead", false},
		{strings.Repeat("8", 40), "we are behind", true},
		{strings.Repeat("7", 40), "diverged", true},
	} {
		got, warn := compareNvim(api, "o/r", pin, nvimEntry{"o/r", tc.nvim})
		if !strings.Contains(got, tc.want) || warn != tc.warn {
			t.Errorf("nvim %.4s: %q warn=%v, want %q warn=%v", tc.nvim, got, warn, tc.want, tc.warn)
		}
	}
}

func TestNvimSectionUnavailableIsAWarning(t *testing.T) {
	api := newAPI()
	var out bytes.Buffer
	nvimSection(&pins{}, &out, api, nil, fmt.Errorf("boom"))
	if len(api.findings) != 1 || !strings.Contains(out.String(), "::warning::nvim-treesitter: comparison unavailable: boom") {
		t.Errorf("findings %v, output %q", api.findings, out.String())
	}
}

func TestGrammarNewerStableReleaseWarns(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			fmt.Fprint(w, `{"tag_name":"v2.0.0"}`)
		case strings.Contains(r.URL.Path, "/commits/"):
			fmt.Fprint(w, `{"sha":"head","commit":{"committer":{"date":"2026-11-01T00:00:00Z"}}}`)
		case strings.Contains(r.URL.Path, "/compare/"):
			fmt.Fprint(w, `{"status":"ahead"}`) // the release is ahead of the pin (and the head of the pin is reachable)
		case strings.Contains(r.URL.Path, "/branches"), strings.Contains(r.URL.Path, "/tags"):
			fmt.Fprint(w, `[]`)
		default:
			fmt.Fprint(w, `{"full_name":"o/r","default_branch":"main"}`)
		}
	}))
	defer server.Close()
	for _, grammar := range []bool{true, false} {
		api := newAPI()
		api.base, api.now = server.URL+"/", time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
		var out bytes.Buffer
		health(api, &out, source{"g", "o/r", "pinpinpinpin1234", grammar})
		if has := strings.Contains(out.String(), "::warning::g: newer stable release v2.0.0 than pin pinpinpinpin"); has != grammar || (len(api.findings) == 1) != grammar {
			t.Errorf("grammar=%v: findings %v, output %s", grammar, api.findings, out.String())
		}
	}
}

func TestWriteIssue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "body.md")
	report := bytes.NewBufferString("## Report\n::warning::x: y\n| a | b |\n")
	write := func(failed bool, findings ...finding) bool {
		os.WriteFile(path, []byte("stale"), 0o644)
		api := newAPI()
		api.now = time.Date(2026, 11, 16, 1, 2, 0, 0, time.UTC)
		api.findings = findings
		if err := writeIssue(path, report, api, failed); err != nil {
			t.Fatal(err)
		}
		_, err := os.Stat(path)
		return err == nil
	}
	if write(false) || write(true, finding{"x", "y"}) {
		t.Error("no warnings, or a hard error, must leave no body (and remove a stale one)")
	}
	if !write(false, finding{"py|thon", "behind\nnvim"}) {
		t.Fatal("warnings must write a body")
	}
	body, _ := os.ReadFile(path)
	for _, want := range []string{"upstream-watch", "**1 warning(s)**", `| py\|thon | behind nvim |`, "2026-11-16 01:02 UTC", "<details>", "| a | b |"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("body lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(string(body), "::warning::") {
		t.Error("annotations must not reach the issue")
	}
	report.WriteString(strings.Repeat("filler line\n", 10000))
	if write(false, finding{"x", "y"}); func() int { b, _ := os.ReadFile(path); return len(b) }() > 65536 {
		t.Error("body exceeds the GitHub limit")
	}
}
