// Command upstreamwatch compares the pins of tools/gen/pins.json with upstream (.github/workflows/upstream-watch.yml):
// the newest stable release of the tree-sitter runtime, wasi-sdk, binaryen and wasm2go, and the default-branch heads of
// the grammar repositories and wasi-libc. It prints a markdown table and exits 1 when a pinned tool or runtime has a
// newer stable release; a head ahead of its pinned commit is reported only. Stdlib only; GITHUB_TOKEN, when set, lifts
// the API rate limit. It reads nothing but pins.json and public GitHub API answers.
//
//	upstreamwatch [-pins tools/gen/pins.json]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type pins struct {
	Runtime struct {
		Release string `json:"release"`
		Repo    string `json:"repo"`
	} `json:"runtime"`
	Tools struct {
		WasiSDK  string `json:"wasi-sdk"`
		Binaryen string `json:"binaryen"`
		Wasm2go  string `json:"wasm2go"`
		Module   string `json:"wasm2goModule"`
	} `json:"tools"`
	WasiLibc struct{ Repo, Commit string }            `json:"wasiLibc"`
	Grammars map[string]struct{ Repo, Commit string } `json:"grammars"`
}

func main() {
	file := flag.String("pins", filepath.Join("tools", "gen", "pins.json"), "pins file")
	flag.Parse()
	raw, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "upstreamwatch:", err)
		os.Exit(2)
	}
	var p pins
	if err := json.Unmarshal(raw, &p); err != nil {
		fmt.Fprintln(os.Stderr, "upstreamwatch:", err)
		os.Exit(2)
	}
	stale, failed := report(&p, os.Stdout)
	if failed {
		os.Exit(2)
	}
	if stale {
		os.Exit(1)
	}
}

func repoOf(url string) string { return strings.TrimPrefix(url, "https://github.com/") }

func report(p *pins, w *os.File) (stale, failed bool) {
	fmt.Fprintln(w, "## Pinned tools and runtime vs newest stable release")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| component | pinned | newest stable | status |")
	fmt.Fprintln(w, "|---|---|---|---|")
	tools := []struct{ name, pinned, repo string }{
		{"tree-sitter runtime", p.Runtime.Release, repoOf(p.Runtime.Repo)},
		{"wasi-sdk", p.Tools.WasiSDK, "WebAssembly/wasi-sdk"},
		{"binaryen", p.Tools.Binaryen, "WebAssembly/binaryen"},
		{"wasm2go", p.Tools.Wasm2go, repoOf("https://" + p.Tools.Module)},
	}
	for _, t := range tools {
		var rel struct {
			Tag string `json:"tag_name"`
		}
		if err := api("repos/"+t.repo+"/releases/latest", &rel); err != nil { // "latest" is never a prerelease or draft
			fmt.Fprintf(w, "| %s | %s | ? | **ERROR** %v |\n", t.name, t.pinned, err)
			failed = true
			continue
		}
		status := "current"
		if newer(t.pinned, rel.Tag) {
			status, stale = "**NEWER STABLE RELEASE**", true
			fmt.Printf("::error::%s: pinned %s, newest stable release %s\n", t.name, t.pinned, rel.Tag)
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s |\n", t.name, t.pinned, rel.Tag, status)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Pinned sources vs default-branch head (reported, not failed)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| component | repository | pinned | head | commits ahead | head date |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|")
	type src struct{ name, repo, commit string }
	srcs := []src{{"wasi-libc", repoOf(p.WasiLibc.Repo), p.WasiLibc.Commit}}
	seen := map[string]int{}
	names := make([]string, 0, len(p.Grammars))
	for n := range p.Grammars {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		g := p.Grammars[n]
		if i, ok := seen[g.Repo+g.Commit]; ok { // typescript and tsx share one repository and commit
			srcs[i].name += "+" + n
			continue
		}
		seen[g.Repo+g.Commit] = len(srcs)
		srcs = append(srcs, src{n, repoOf(g.Repo), g.Commit})
	}
	for _, s := range srcs {
		var repo struct {
			Branch string `json:"default_branch"`
		}
		var head struct {
			SHA    string `json:"sha"`
			Commit struct {
				Committer struct{ Date string } `json:"committer"`
			} `json:"commit"`
		}
		var cmp struct {
			Ahead int `json:"ahead_by"`
		}
		err := api("repos/"+s.repo, &repo)
		if err == nil {
			err = api("repos/"+s.repo+"/commits/"+repo.Branch, &head)
		}
		if err == nil {
			err = api("repos/"+s.repo+"/compare/"+s.commit+"..."+head.SHA, &cmp)
			if se, ok := err.(*statusError); ok && se.code == http.StatusNotFound { // no common history, e.g. the branch was rewritten
				cmp.Ahead, err = -1, nil
			}
		}
		if err != nil {
			fmt.Fprintf(w, "| %s | %s | %.12s | ? | ? | **ERROR** %v |\n", s.name, s.repo, s.commit, err)
			failed = true
			continue
		}
		ahead := strconv.Itoa(cmp.Ahead)
		if cmp.Ahead < 0 {
			ahead = "pinned commit not in the head's history"
		}
		fmt.Fprintf(w, "| %s | %s | %.12s | %.12s | %s | %s |\n", s.name, s.repo, s.commit, head.SHA, ahead, head.Commit.Committer.Date)
	}
	return stale, failed
}

type statusError struct {
	code int
	error
}

func api(path string, v any) error {
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/"+path, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "upstream-watch")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := (&http.Client{Timeout: time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &statusError{resp.StatusCode, fmt.Errorf("GET %s: %s", path, resp.Status)}
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

var digits = regexp.MustCompile(`\d+`)

// newer reports whether latest is a higher version than pinned. Both are compared as the numbers they contain
// ("v0.27.2", "34.0" vs "wasi-sdk-35", "133" vs "version_134"); a missing component counts as 0.
func newer(pinned, latest string) bool {
	a, b := nums(pinned), nums(latest)
	for i := 0; i < max(len(a), len(b)); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return y > x
		}
	}
	return false
}

func nums(s string) (out []int) {
	for _, d := range digits.FindAllString(s, -1) {
		n, _ := strconv.Atoi(d)
		out = append(out, n)
	}
	return out
}
