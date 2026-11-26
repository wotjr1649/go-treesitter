package main

// Comparison of the grammar pins with nvim-treesitter's registry (lua/nvim-treesitter/parsers.lua on main) and the
// tracking-issue body. parsers.lua is untrusted reference text: only url/revision per parser entry are extracted,
// the size is bounded, and a failure to fetch or parse it is reported as a warning, never as a hard error.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
)

const (
	nvimParsersURL = "https://raw.githubusercontent.com/nvim-treesitter/nvim-treesitter/main/lua/nvim-treesitter/parsers.lua"
	nvimMaxBytes   = 2 << 20
	issueMaxBytes  = 60000 // GitHub rejects issue bodies above 65536 characters
)

// nvimEntry is one parser of the registry: repo is "owner/name", rev a commit or a tag ("" if absent or malformed).
type nvimEntry struct{ repo, rev string }

var (
	nvimEntryRe = regexp.MustCompile(`^  ([A-Za-z0-9_]+) = \{\s*$`)
	nvimCloseRe = regexp.MustCompile(`^  \},?\s*$`)
	nvimFieldRe = regexp.MustCompile(`^\s+(url|revision)\s*=\s*'([^'\\]*)',?\s*$`)
	nvimURLRe   = regexp.MustCompile(`^https://github\.com/([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+?)(?:\.git)?/?$`)
	nvimRevRe   = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,100}$`)
	sha40       = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// nvimNames maps our grammar names to nvim-treesitter's where they differ.
var nvimNames = map[string]string{"golang": "go", "csharp": "c_sharp"}

func parseNvim(text string) (map[string]nvimEntry, error) {
	out := map[string]nvimEntry{}
	var name string
	var cur nvimEntry
	var haveURL, haveRev bool
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case nvimEntryRe.MatchString(line):
			name, cur, haveURL, haveRev = nvimEntryRe.FindStringSubmatch(line)[1], nvimEntry{}, false, false
		case name != "" && nvimCloseRe.MatchString(line):
			if cur.repo != "" {
				out[name] = cur
			}
			name = ""
		case name != "":
			m := nvimFieldRe.FindStringSubmatch(line)
			switch {
			case m == nil:
			case m[1] == "url" && !haveURL:
				haveURL = true
				if u := nvimURLRe.FindStringSubmatch(m[2]); u != nil {
					cur.repo = u[1]
				}
			case m[1] == "revision" && !haveRev:
				haveRev = true
				if nvimRevRe.MatchString(m[2]) {
					cur.rev = m[2]
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no parser entries recognised (did the parsers.lua layout change?)")
	}
	return out, nil
}

func fetchNvim(client *http.Client, src string) (map[string]nvimEntry, error) {
	req, err := http.NewRequest(http.MethodGet, src, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "upstream-watch") // no token: this is another host
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", src, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, nvimMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > nvimMaxBytes {
		return nil, fmt.Errorf("parsers.lua larger than %d bytes", nvimMaxBytes)
	}
	return parseNvim(string(body))
}

// canonical is the API's current owner/name of a repository, lowercase; the literal name if the lookup fails.
func canonical(api *githubAPI, repo string) string {
	var r struct {
		Name string `json:"full_name"`
	}
	if err := api.get("repos/"+repo, &r); err == nil && r.Name != "" {
		return strings.ToLower(r.Name)
	}
	return strings.ToLower(repo)
}

func resolveRev(api *githubAPI, repo, rev string) (string, error) {
	if sha40.MatchString(rev) {
		return rev, nil
	}
	var c struct {
		SHA string `json:"sha"`
	}
	if err := api.get("repos/"+repo+"/commits/"+url.PathEscape(rev), &c); err != nil {
		return "", err
	}
	if c.SHA == "" {
		return "", errors.New("no commit sha")
	}
	return c.SHA, nil
}

// nvimSection prints the comparison table. Differences that need a decision (we are behind, diverged, another
// repository, unresolved revision, parsers.lua unavailable) are warnings; "same", "ahead" and "not in" are information.
func nvimSection(p *pins, w io.Writer, api *githubAPI, entries map[string]nvimEntry, err error) {
	fmt.Fprintln(w, "\n## nvim-treesitter comparison (reference only; parsers.lua on main)")
	fmt.Fprintln(w)
	if err != nil {
		fmt.Fprintf(w, "Comparison unavailable: %v\n", err)
		api.warn(w, "nvim-treesitter", "nvim comparison unavailable", "comparison unavailable: "+err.Error())
		return
	}
	fmt.Fprintln(w, "| grammar | our pin | nvim-treesitter | result |")
	fmt.Fprintln(w, "|---|---|---|---|")
	names := make([]string, 0, len(p.Grammars))
	for n := range p.Grammars {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		g := p.Grammars[n]
		ours := fmt.Sprintf("%s@%.12s", repoOf(g.Repo), g.Commit)
		key := n
		if alias, ok := nvimNames[n]; ok {
			key = alias
		}
		e, ok := entries[key]
		if !ok {
			fmt.Fprintf(w, "| %s | %s | - | not in nvim-treesitter |\n", n, ours)
			continue
		}
		theirs := fmt.Sprintf("%s@%.12s", e.repo, e.rev)
		result, warn := compareNvim(api, repoOf(g.Repo), g.Commit, e)
		fmt.Fprintf(w, "| %s | %s | %s | %s |\n", n, ours, theirs, result)
		if warn {
			api.warn(w, n, "nvim-treesitter: "+result, "nvim-treesitter: "+result+" ("+theirs+")")
		}
	}
}

func compareNvim(api *githubAPI, repo, pin string, e nvimEntry) (result string, warn bool) {
	ours, theirs := canonical(api, repo), canonical(api, e.repo)
	if ours != theirs {
		return "**different repository**", true
	}
	if e.rev == "" {
		return "**unresolved** (no valid revision)", true
	}
	sha, err := resolveRev(api, ours, e.rev)
	if err != nil {
		return "**unresolved** revision " + e.rev + ": " + err.Error(), true
	}
	if strings.EqualFold(sha, pin) {
		return "same", false
	}
	var cmp struct {
		Status string `json:"status"`
	}
	if err := api.get("repos/"+ours+"/compare/"+sha+"..."+url.PathEscape(pin)+"?per_page=1", &cmp); err != nil {
		return "**unknown** (" + err.Error() + ")", true
	}
	switch cmp.Status { // compare nvim...ours: "ahead" means our pin is newer
	case "ahead":
		return "we are ahead", false
	case "behind":
		return "**we are behind**", true
	case "diverged":
		return "**diverged**", true
	case "identical":
		return "same", false
	}
	return "**unknown** comparison status " + cmp.Status, true
}

// writeIssue removes path, then writes the tracking-issue body when nothing failed hard (an
// incomplete report must not close or rewrite the issue).
func writeIssue(path string, report *bytes.Buffer, api *githubAPI, failed bool) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if failed {
		return nil
	}
	var b strings.Builder
	b.WriteString("<!-- upstream-watch: maintained by .github/workflows/upstream-watch.yml; edits are overwritten -->\n")
	fmt.Fprintf(&b, "Updated %s by the weekly upstream-watch workflow. Replaced on every run; closed automatically when no unaccepted warning remains.\n\n",
		api.now.Format("2006-01-02 15:04 UTC"))
	warningSummary(&b, api)
	candidatesSection(&b, api)
	var full []string
	for _, line := range strings.Split(report.String(), "\n") {
		if !strings.HasPrefix(line, "::") {
			full = append(full, line)
		}
	}
	text := strings.Join(full, "\n")
	if room := issueMaxBytes - b.Len() - 200; len(text) > room {
		text = strings.ToValidUTF8(text[:max(room, 0)], "") + "\n\n(report truncated; see the run log)\n"
	}
	fmt.Fprintf(&b, "\n<details><summary>Full report</summary>\n\n%s\n\n</details>\n", text)
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
