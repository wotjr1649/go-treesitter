// Command upstreamwatch compares the pins of tools/gen/pins.json with upstream (.github/workflows/upstream-watch.yml):
// release freshness and repository health of every pinned source and tool. A newer tool/runtime release exits 1;
// a tag newer than the latest release (a tag without a GitHub Release) counts as a newer release; a missing repository,
// unreachable pin or API failure exits 2. A grammar pin behind its repository's default branch is reported for every
// grammar and warned about only for owner repositories (ADR-0024 rule 7; tags.go) whose commits since the pin change a hashed path
// (hashed.go); a range of docs/tests/dev-script commits is information. Renames, archives and age are warnings. Stdlib only;
// GITHUB_TOKEN lifts the API rate limit. Requests are cached for shared repositories and pins are never changed.
//
// Every grammar pin is also compared with nvim-treesitter's parsers.lua (information, never a failure; see nvim.go).
// -issue FILE writes a complete tracking-issue body unless a hard error occurred.
// -state FILE writes warn, clean or error; accepted warnings do not prevent auto-close.
// -candidates FILE writes the pins that could move (candidates.go, JSON with a version field) unless a hard error occurred;
// the issue body lists them with dispatch lines. Nothing here dispatches a workflow.
//
//	upstreamwatch [-pins tools/gen/pins.json] [-nvim URL|""] [-issue FILE] [-state FILE] [-candidates FILE]
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
		Commit  string `json:"commit"`
		Repo    string `json:"repo"`
	} `json:"runtime"`
	Tools struct {
		WasiSDK  string `json:"wasi-sdk"`
		Binaryen string `json:"binaryen"`
		Wasm2go  string `json:"wasm2go"`
		Module   string `json:"wasm2goModule"`
		CLI      string `json:"treeSitterCLI"`
		LLVM     string `json:"llvmCommit"`
	} `json:"tools"`
	WasiLibc struct{ Repo, Commit string }            `json:"wasiLibc"`
	Grammars map[string]struct{ Repo, Commit string } `json:"grammars"`
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, newAPI())) }

// run is main with its dependencies injected; it returns the exit code.
func run(args []string, stdout io.Writer, api *githubAPI) int {
	fs := flag.NewFlagSet("upstreamwatch", flag.ExitOnError)
	file := fs.String("pins", filepath.Join("tools", "gen", "pins.json"), "pins file")
	nvimURL := fs.String("nvim", nvimParsersURL, "nvim-treesitter parsers.lua URL (empty: skip the comparison)")
	issue := fs.String("issue", "", "write the tracking-issue body here when the report is complete")
	state := fs.String("state", "", "write warn, clean or error for the tracking-issue workflow")
	candidates := fs.String("candidates", "", "write the pin-move candidates (JSON) here when the report is complete")
	fs.Parse(args)
	fail := func(err error) int { fmt.Fprintln(os.Stderr, "upstreamwatch:", err); return 2 }
	raw, err := os.ReadFile(*file)
	if err != nil {
		return fail(err)
	}
	var p pins
	if err := json.Unmarshal(raw, &p); err != nil {
		return fail(err)
	}
	var out bytes.Buffer
	api.hashed = loadHashed(raw)
	api.acceptances, err = loadAcceptances(acceptedWarningsJSON)
	if err != nil {
		return fail(err)
	}
	both := io.MultiWriter(stdout, &out)
	stale, failed := report(&p, both, api)
	if *nvimURL != "" {
		entries, err := fetchNvim(api.client, *nvimURL)
		nvimSection(&p, both, api, entries, err)
	}
	patches, metaErr := loadPatches(raw) // optional metadata: a defect is a diagnostic, never a new exit 2
	var added func(string) (time.Time, error)
	var historyErr error
	if len(patches) > 0 {
		added, historyErr = gitFirstOnMain(*file)
	}
	patchSection(both, api, patches, metaErr, added, historyErr)
	activitySection(&p, both, api) // last and bounded: it cannot keep the issue body or state from being written
	warnings := warningSummary(stdout, api)
	if *issue != "" {
		if err := writeIssue(*issue, &out, api, failed); err != nil {
			return fail(err)
		}
	}
	if *candidates != "" {
		if err := writeCandidates(*candidates, api, failed); err != nil {
			return fail(err)
		}
	}
	if *state != "" {
		value := "clean"
		if failed {
			value = "error"
		} else if warnings > 0 {
			value = "warn"
		}
		if err := os.WriteFile(*state, []byte(value+"\n"), 0o644); err != nil {
			return fail(err)
		}
	}
	if failed {
		return 2
	}
	if stale {
		return 1
	}
	return 0
}

func repoOf(url string) string { return strings.TrimPrefix(url, "https://github.com/") }

type source struct {
	name, repo, pin string
	grammar         bool // a grammar pin: a newer stable release than the pin is a warning
}

type finding struct{ component, text, kind string }

// warn records a warning for the tracking issue and prints its annotation.
func (a *githubAPI) warn(w io.Writer, name, kind, text string) finding {
	f := finding{name, text, kind}
	a.findings = append(a.findings, f)
	level := "warning"
	if a.accepted(f) != nil {
		level = "notice"
	}
	fmt.Fprintf(w, "::%s::%s: %s\n", level, name, text)
	return f
}

func report(p *pins, w io.Writer, api *githubAPI) (stale, failed bool) {
	fmt.Fprintln(w, "## Pinned tools and runtime vs newest stable release")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| component | pinned | newest stable | status |")
	fmt.Fprintln(w, "|---|---|---|---|")
	// tag is an example of the repository's tag naming for the pinned version (its prefix selects the tags to compare).
	tools := []struct{ name, pinned, repo, ref, tag string }{
		{"tree-sitter runtime", p.Runtime.Release, repoOf(p.Runtime.Repo), p.Runtime.Commit, p.Runtime.Release},
		{"wasi-sdk", p.Tools.WasiSDK, "WebAssembly/wasi-sdk", "wasi-sdk-" + strings.Split(p.Tools.WasiSDK, ".")[0], "wasi-sdk-" + strings.Split(p.Tools.WasiSDK, ".")[0]},
		{"binaryen", p.Tools.Binaryen, "WebAssembly/binaryen", "version_" + p.Tools.Binaryen, "version_" + p.Tools.Binaryen},
		{"wasm2go", p.Tools.Wasm2go, repoOf("https://" + p.Tools.Module), p.Tools.Wasm2go, p.Tools.Wasm2go},
	}
	if p.Tools.CLI != "" {
		v := "v" + strings.TrimPrefix(p.Tools.CLI, "v")
		tools = append(tools, struct{ name, pinned, repo, ref, tag string }{"tree-sitter CLI", p.Tools.CLI, repoOf(p.Runtime.Repo), v, v})
	}
	srcs := []source{{"wasi-libc", repoOf(p.WasiLibc.Repo), p.WasiLibc.Commit, false}}
	if p.Tools.LLVM != "" {
		srcs = append(srcs, source{"LLVM (wasi-sdk clang)", "llvm/llvm-project", p.Tools.LLVM, false})
	}
	for _, t := range tools {
		ref := t.ref
		if ref == "" {
			ref = t.pinned
		}
		srcs = append(srcs, source{t.name, t.repo, ref, false})
		var rel struct {
			Tag string `json:"tag_name"`
		}
		if err := api.get("repos/"+t.repo+"/releases/latest", &rel); err != nil {
			fmt.Fprintf(w, "| %s | %s | ? | **ERROR** %v |\n", t.name, t.pinned, err)
			failed = true
			continue
		}
		// A tag without a GitHub Release (wasm2go v0.4.22) is invisible to releases/latest.
		latest, what := rel.Tag, "release"
		tag, err := newestTag(api, t.repo, t.tag)
		if err != nil {
			fmt.Fprintf(w, "| %s | %s | %s | **ERROR** tags: %v |\n", t.name, t.pinned, rel.Tag, err)
			failed = true
			continue
		}
		if newer(latest, tag) {
			latest, what = tag, "tag without a release"
		}
		status := "current"
		if newer(t.pinned, latest) {
			status, stale = "**NEWER STABLE "+strings.ToUpper(what)+"**", true
			fmt.Fprintf(w, "::error::%s: pinned %s, newest stable %s %s\n", t.name, t.pinned, what, latest)
			api.findings = append(api.findings, finding{component: t.name, text: "newer stable " + what + " " + latest + " than pinned " + t.pinned})
			api.propose(candidate{Component: t.name, Pinned: t.pinned, Newest: latest, Kind: "release", Repo: t.repo, TagOnly: what != "release"}, "")
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s |\n", t.name, t.pinned, latest, status)
	}
	names := make([]string, 0, len(p.Grammars))
	for n := range p.Grammars {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		g := p.Grammars[n]
		srcs = append(srcs, source{n, repoOf(g.Repo), g.Commit, true})
	}

	fmt.Fprintln(w, "\n## Repository health (all pins)")
	fmt.Fprintln(w, "\n| component | pinned repository → current repository | pin | default head / date | latest release vs pin | health |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|")
	for _, s := range srcs {
		if health(api, w, s) {
			failed = true
		}
	}
	fmt.Fprintf(w, "\nGitHub API requests: %d (cached by path); rate limit remaining: %s.\n", api.requests, api.remaining)
	return stale, failed
}

func health(api *githubAPI, w io.Writer, s source) (failed bool) {
	var repo struct {
		Name     string `json:"full_name"`
		Branch   string `json:"default_branch"`
		Archived bool   `json:"archived"`
	}
	repoPath := "repos/" + s.repo
	err := api.get(repoPath, &repo)
	if err != nil {
		state := "repository lookup failed"
		if isStatus(err, http.StatusNotFound) || isStatus(err, http.StatusGone) {
			state = "repository missing/deleted (or inaccessible)"
		}
		fmt.Fprintf(w, "| %s | %s | %.12s | ? | ? | **ERROR** %s: %v |\n", s.name, s.repo, s.pin, state, err)
		fmt.Fprintf(w, "::error::%s: %s\n", s.name, state)
		return true
	}
	identity := s.repo
	health := []string{}
	if !strings.EqualFold(repo.Name, s.repo) || api.redirected[repoPath] {
		identity += " → " + repo.Name
		health = append(health, "WARNING renamed/transferred (API redirect or full_name differs)")
		api.warn(w, s.name, "repository renamed/transferred", fmt.Sprintf("repository %s → %s", s.repo, repo.Name))
	}
	if repo.Archived {
		health = append(health, "WARNING archived")
		api.warn(w, s.name, "repository archived", "repository archived")
	}
	// Use the canonical name after a transfer, avoiding a redirect on every following request.
	base := "repos/" + repo.Name
	var head struct {
		SHA    string `json:"sha"`
		Commit struct {
			Committer struct {
				Date time.Time `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	err = api.get(base+"/commits/"+url.PathEscape(repo.Branch), &head)
	headOK := err == nil
	headText := "?"
	if err != nil {
		health = append(health, "ERROR default head: "+err.Error())
		failed = true
	} else {
		date := head.Commit.Committer.Date
		headText = fmt.Sprintf("%.12s / %s", head.SHA, date.Format("2006-01-02"))
		if date.IsZero() {
			health = append(health, "ERROR missing head date")
			failed = true
		} else if date.Before(api.now.AddDate(-1, 0, 0)) {
			health = append(health, "WARNING last commit >12 months; tier review")
			api.warn(w, s.name, "tier review (>12 months)", fmt.Sprintf("last commit %s; tier review (>12 months)", date.Format("2006-01-02")))
		}

	}
	var release struct {
		Tag string `json:"tag_name"`
	}
	err = api.get(base+"/releases/latest", &release)
	releaseText := release.Tag + " vs " + s.pin
	if isStatus(err, http.StatusNotFound) {
		releaseText = "no stable release"
	} else if err != nil {
		releaseText = "ERROR " + err.Error()
		failed = true
	} else {
		var cmp struct {
			Status string `json:"status"`
		}
		err = api.get(base+"/compare/"+url.PathEscape(s.pin)+"..."+url.PathEscape(release.Tag)+"?per_page=1", &cmp)
		if isStatus(err, http.StatusNotFound) {
			releaseText += " (no comparison/common ancestor)"
		} else if err != nil {
			releaseText += " (ERROR " + err.Error() + ")"
			failed = true
		} else {
			releaseText += " (" + cmp.Status + ")"
			if s.grammar && cmp.Status == "ahead" { // the release contains the pin and is newer
				health = append(health, "WARNING newer stable release "+release.Tag)
				f := api.warn(w, s.name, "newer stable release "+release.Tag+" than pin", fmt.Sprintf("newer stable release %s than pin %.12s", release.Tag, s.pin))
				api.propose(candidate{Component: s.name, Pinned: s.pin, Newest: release.Tag, Kind: "release", Repo: s.repo, Grammar: true}, f.kind)
			}
		}
	}
	if headOK && s.grammar {
		pinBehind(api, w, base, s, repo.Branch, head.SHA, &health)
	}
	if s.grammar && isOwner(s.repo) {
		tagOnlyRelease(api, w, base, s, release.Tag, &health, &failed)
	}
	if headOK {
		where, err := pinReachability(api, base, s.pin, repo.Branch, head.SHA, release.Tag)
		switch {
		case err != nil:
			health = append(health, "ERROR pin reachability: "+err.Error())
			failed = true
		case where == "":
			health = append(health, "ERROR pin unreachable from every branch and tag")
			failed = true
		default:
			health = append(health, where)
		}
	}

	if failed {
		fmt.Fprintf(w, "::error::%s: repository health failed\n", s.name)
	}
	fmt.Fprintf(w, "| %s | %s | %.12s | %s | %s | %s |\n", s.name, identity, s.pin, headText, releaseText, strings.Join(health, "; "))
	return failed
}

type statusError struct {
	code int
	error
}

func isStatus(err error, code int) bool { se, ok := err.(*statusError); return ok && se.code == code }

type apiReply struct {
	body []byte
	err  error
}
type githubAPI struct {
	base           string
	client         *http.Client
	token          string
	now            time.Time
	cache          map[string]apiReply
	redirected     map[string]bool
	candidates     []candidate
	hashed         map[string]hashedPaths // per grammar; nil: no docs-only filter
	acceptances    []acceptance
	findings       []finding
	requests       int
	remaining      string
	gqlStop        string        // set once a GraphQL limit is reached (separate from the REST remaining)
	activityBudget time.Duration // time budget of the whole activity section
}

func newAPI() *githubAPI {
	return &githubAPI{base: "https://api.github.com/", client: &http.Client{Timeout: time.Minute}, token: os.Getenv("GITHUB_TOKEN"), now: time.Now().UTC(), cache: map[string]apiReply{}, redirected: map[string]bool{}, remaining: "unknown", activityBudget: 2 * time.Minute}
}
func (a *githubAPI) get(path string, v any) error {
	if reply, ok := a.cache[path]; ok {
		if reply.err != nil {
			return reply.err
		}
		return json.Unmarshal(reply.body, v)
	}
	if a.remaining == "0" {
		return fmt.Errorf("GitHub API rate limit exhausted; use GITHUB_TOKEN or wait for reset")
	}
	req, err := http.NewRequest(http.MethodGet, a.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "upstream-watch")
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
	a.requests++
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if value := resp.Header.Get("X-RateLimit-Remaining"); value != "" {
		a.remaining = value
	}
	a.redirected[path] = resp.Request.URL.String() != req.URL.String()
	if resp.StatusCode != http.StatusOK {
		err := &statusError{resp.StatusCode, fmt.Errorf("GET %s: %s", path, resp.Status)}
		a.cache[path] = apiReply{err: err}
		return err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	a.cache[path] = apiReply{body: body}
	return json.Unmarshal(body, v)
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
