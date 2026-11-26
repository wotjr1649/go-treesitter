// Command pinbump moves ONE owner-repository grammar pin of tools/gen/pins.json to a release tag, the way the Pins
// procedure in tools/gen/README.md describes: resolve the tag to a commit (read-only GitHub API), download the API
// tarball of that commit, hash it, and rewrite exactly the entry's pin fields. It never extracts to disk, never runs
// anything from the archive, and refuses everything outside v1: a repository other than `https://github.com/wotjr1649/*`
// (ADR-0024 rule 7), an entry with patches, npm inputs, an explicit archive URL or other extras, a tag that is not a
// stable release tag ahead of the pin, a tree that lost a pinned file, and a repository without a CHANGELOG.md (AGENTS.md:
// an owner pin move cites the owner CHANGELOG). Grammars that share one repository commit (typescript and tsx) move
// together: that is one pin. Stdlib only.
//
//	pinbump -pins tools/gen/pins.json -grammar java -tag v3.0.4 [-out DIR]
//
// Without -out it rewrites -pins in place. With -out it writes pins.json, candidate.json (the input of tools/gen/autopr)
// and summary.md there and leaves -pins alone. The PR body skeleton is always printed to stdout. GITHUB_TOKEN, when set,
// is sent only to api.github.com (rate limit); the tool works without it.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	defaultAPI  = "https://api.github.com"
	maxJSON     = 1 << 20
	maxArchive  = 256 << 20 // compressed bytes
	maxUnpacked = 1 << 30   // sum of tar header sizes
	maxKept     = 256 << 20 // bytes of the files we keep in memory
	maxFile     = 64 << 20
	maxEntries  = 200000
)

var (
	nameRE   = regexp.MustCompile("^[a-z0-9]+$")
	tagRE    = regexp.MustCompile(`^v?[0-9]+(\.[0-9]+){1,3}$`)
	commitRE = regexp.MustCompile("^[0-9a-f]{40}$")
	repoRE   = regexp.MustCompile(`^https://github\.com/(wotjr1649/([A-Za-z0-9._-]+))$`)
)

// plainKeys are the fields of a grammar entry v1 understands. Anything else (patches, generateFrom, npmDependencies,
// archiveURL, reference, exceptions) changes what a pin move means and needs a human.
var plainKeys = map[string]bool{"repo": true, "commit": true, "extractDir": true, "archive": true, "archiveSHA256": true,
	"grammarDir": true, "dirs": true, "treeSHA256": true, "files": true, "licenseSHA256": true, "noticeSHA256": true,
	"cSymbol": true, "regenerate": true}

type pin struct {
	Repo          string            `json:"repo"`
	Commit        string            `json:"commit"`
	ExtractDir    string            `json:"extractDir"`
	Archive       string            `json:"archive"`
	ArchiveSHA256 string            `json:"archiveSHA256"`
	GrammarDir    string            `json:"grammarDir"`
	Dirs          []string          `json:"dirs"`
	TreeSHA256    string            `json:"treeSHA256"`
	Files         map[string]string `json:"files"`
	LicenseSHA256 string            `json:"licenseSHA256"`
	NoticeSHA256  string            `json:"noticeSHA256"`
}

type change struct {
	Name     string
	Old, New pin
}

type config struct {
	Pins    []byte
	Grammar string
	Tag     string
	API     string // base URL; tests point it at a fake server
	Client  *http.Client
	Token   string
}

type result struct {
	Pins      []byte
	Candidate []byte
	Summary   string
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// groupOf returns the entries that share the named grammar's repository and commit, plus their parsed old pins.
func groupOf(pins []byte, grammar string) ([]change, error) {
	var top struct {
		Grammars map[string]map[string]json.RawMessage `json:"grammars"`
	}
	if err := json.Unmarshal(pins, &top); err != nil {
		return nil, err
	}
	parse := func(name string) (pin, error) {
		var p pin
		raw := top.Grammars[name]
		for k := range raw {
			if !plainKeys[k] {
				return p, fmt.Errorf("grammar %s has %q: pins with extras are moved by hand in v1", name, k)
			}
		}
		b, _ := json.Marshal(raw)
		if err := json.Unmarshal(b, &p); err != nil {
			return p, err
		}
		m := repoRE.FindStringSubmatch(p.Repo)
		if m == nil {
			return p, fmt.Errorf("grammar %s: %q is not an owner repository (v1 moves https://github.com/wotjr1649/* only)", name, p.Repo)
		}
		want := m[2] + "-" + p.Commit[:min(12, len(p.Commit))]
		if !commitRE.MatchString(p.Commit) || p.ExtractDir != want || p.Archive != want+".tar.gz" {
			return p, fmt.Errorf("grammar %s: extractDir/archive do not follow <repo>-<commit12>[.tar.gz]; derive by hand", name)
		}
		return p, nil
	}
	if _, ok := top.Grammars[grammar]; !ok {
		return nil, fmt.Errorf("grammar %q is not in pins.json (pinbump never adds one)", grammar)
	}
	t, err := parse(grammar)
	if err != nil {
		return nil, err
	}
	var out []change
	for name := range top.Grammars {
		var r struct{ Repo, Commit string }
		b, _ := json.Marshal(top.Grammars[name])
		_ = json.Unmarshal(b, &r)
		if r.Repo != t.Repo || r.Commit != t.Commit {
			continue
		}
		p, err := parse(name)
		if err != nil {
			return nil, err
		}
		out = append(out, change{Name: name, Old: p})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (c config) get(rawURL string, limit int64) ([]byte, int, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, 0, err
	}
	req, _ := http.NewRequest("GET", rawURL, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "go-treesitter-pinbump")
	if c.Token != "" && c.API == defaultAPI && u.Host == "api.github.com" {
		req.Header.Set("Authorization", "Bearer "+c.Token) // never to another host; Go also drops it on cross-host redirects
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if int64(len(b)) > limit {
		return nil, resp.StatusCode, fmt.Errorf("%s: response over %d bytes", u.Path, limit)
	}
	return b, resp.StatusCode, nil
}

func (c config) getJSON(rawURL string, v any) (int, error) {
	b, code, err := c.get(rawURL, maxJSON)
	if err != nil {
		return code, err
	}
	if code != http.StatusOK {
		return code, fmt.Errorf("GET %s: HTTP %d", rawURL, code)
	}
	return code, json.Unmarshal(b, v)
}

// resolveTag returns the commit a tag names, following annotated tag objects.
func (c config) resolveTag(repoPath, tag string) (string, error) {
	var ref struct {
		Object struct{ Type, SHA string }
	}
	if _, err := c.getJSON(c.API+"/repos/"+repoPath+"/git/ref/tags/"+tag, &ref); err != nil {
		return "", err
	}
	for i := 0; i < 4; i++ {
		if !commitRE.MatchString(ref.Object.SHA) {
			return "", errors.New("tag resolves to a malformed object id")
		}
		switch ref.Object.Type {
		case "commit":
			return ref.Object.SHA, nil
		case "tag":
			sha := ref.Object.SHA
			ref.Object.SHA = ""
			if _, err := c.getJSON(c.API+"/repos/"+repoPath+"/git/tags/"+sha, &ref); err != nil {
				return "", err
			}
		default:
			return "", fmt.Errorf("tag points at a %q, not a commit", ref.Object.Type)
		}
	}
	return "", errors.New("annotated tag chain too deep")
}

// readArchive streams a gzip tar and keeps, in memory, only the regular files keep() selects (paths relative to the single
// top directory). Links and special entries inside a hashed directory refuse: gen would fail on them too.
func readArchive(r io.Reader, keep func(string) bool, hashed []string) (map[string][]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	files := map[string][]byte{}
	var top string
	var n int
	var total, kept int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return files, nil
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		if n++; n > maxEntries {
			return nil, errors.New("archive: too many entries")
		}
		if total += h.Size; total > maxUnpacked {
			return nil, errors.New("archive: unpacked size over limit")
		}
		name := strings.TrimSuffix(h.Name, "/")
		if name == "" || strings.ContainsAny(name, "\\\x00") || path.IsAbs(name) {
			return nil, fmt.Errorf("archive: refused entry name %q", h.Name)
		}
		parts := strings.SplitN(name, "/", 2)
		if top == "" {
			top = parts[0]
		} else if parts[0] != top {
			return nil, errors.New("archive: more than one top-level directory")
		}
		if len(parts) == 1 {
			continue
		}
		rel := parts[1]
		if path.Clean(rel) != rel || rel == ".." || strings.HasPrefix(rel, "../") {
			return nil, fmt.Errorf("archive: refused path %q", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
		case tar.TypeReg:
			if !keep(rel) {
				continue
			}
			if h.Size > maxFile || kept+h.Size > maxKept {
				return nil, fmt.Errorf("archive: %s over the size limit", rel)
			}
			b, err := io.ReadAll(io.LimitReader(tr, h.Size))
			if err != nil {
				return nil, err
			}
			kept += int64(len(b))
			files[rel] = b
		default:
			for _, d := range hashed {
				if rel == d || strings.HasPrefix(rel, d+"/") {
					return nil, fmt.Errorf("archive: %s is a link or special file inside hashed directory %s", rel, d)
				}
			}
		}
	}
}

// treeSHA256 is gen's treeSHA256 (tools/gen/pins.go) over in-memory files: sha256 over the sorted lines
// "<slash path>\x00<file sha256>\n" of every file under the directories.
func treeSHA256(files map[string][]byte, dirs []string) (string, error) {
	var lines []string
	for _, d := range dirs {
		for rel, b := range files {
			if strings.HasPrefix(rel, d+"/") {
				lines = append(lines, rel+"\x00"+sum(b)+"\n")
			}
		}
	}
	if len(lines) == 0 {
		return "", fmt.Errorf("no files under %v", dirs)
	}
	sort.Strings(lines)
	return sum([]byte(strings.Join(lines, ""))), nil
}

// derive computes the new pin of one entry from the archive contents.
func derive(old pin, repoName, commit, archiveSHA string, files map[string][]byte) (pin, error) {
	n := old
	n.Commit, n.ArchiveSHA256 = commit, archiveSHA
	n.ExtractDir = repoName + "-" + commit[:12]
	n.Archive = n.ExtractDir + ".tar.gz"
	var err error
	if n.TreeSHA256, err = treeSHA256(files, old.Dirs); err != nil {
		return n, err
	}
	n.Files = map[string]string{}
	for k := range old.Files {
		b, ok := files[path.Join(old.GrammarDir, k)]
		if !ok {
			return n, fmt.Errorf("pinned file %s/%s is missing from the new tree", old.GrammarDir, k)
		}
		n.Files[k] = sum(b)
	}
	if _, ok := files[path.Join(old.GrammarDir, "scanner.c")]; ok && old.Files["scanner.c"] == "" {
		return n, errors.New("the new tree has a scanner.c the old pin does not list: a structural change, pin by hand")
	}
	lic, ok := files["LICENSE"]
	if !ok {
		return n, errors.New("the new tree has no LICENSE")
	}
	n.LicenseSHA256 = sum(lic)
	notice, hasNotice := files["NOTICE"]
	switch {
	case old.NoticeSHA256 != "" && !hasNotice:
		return n, errors.New("NOTICE is pinned but absent from the new tree")
	case old.NoticeSHA256 == "" && hasNotice:
		return n, errors.New("the new tree has a NOTICE the old pin does not list: pin by hand")
	case hasNotice:
		n.NoticeSHA256 = sum(notice)
	}
	return n, nil
}

// patchEntry rewrites the changed scalar fields of one grammar entry in place, so every other byte of pins.json stays.
func patchEntry(text, name string, old, nw pin) (string, error) {
	gi := strings.Index(text, `"grammars"`)
	if gi < 0 {
		return "", errors.New("pins.json has no grammars object")
	}
	head, tail := text[:gi], text[gi:]
	start := regexp.MustCompile(`(?m)^([ \t]*)"`+regexp.QuoteMeta(name)+`":[ \t]*\{[ \t]*\r?$`).FindAllStringSubmatchIndex(tail, -1)
	if len(start) != 1 {
		return "", fmt.Errorf("entry %s: %d matches in pins.json text", name, len(start))
	}
	indent := tail[start[0][2]:start[0][3]]
	end := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(indent) + `\}`).FindStringIndex(tail[start[0][1]:])
	if end == nil {
		return "", fmt.Errorf("entry %s: no closing brace", name)
	}
	bs, be := start[0][0], start[0][1]+end[1]
	block := tail[bs:be]
	set := func(key, o, n string) error {
		if o == n {
			return nil
		}
		re := regexp.MustCompile(`(?m)^([ \t]*"` + regexp.QuoteMeta(key) + `":[ \t]*)"` + regexp.QuoteMeta(o) + `"`)
		if len(re.FindAllStringIndex(block, -1)) != 1 {
			return fmt.Errorf("entry %s: field %s not found exactly once", name, key)
		}
		block = re.ReplaceAllStringFunc(block, func(m string) string { return m[:len(m)-len(o)-2] + `"` + n + `"` }) // literal: no $ expansion
		return nil
	}
	for _, f := range []struct{ k, o, n string }{
		{"commit", old.Commit, nw.Commit}, {"extractDir", old.ExtractDir, nw.ExtractDir}, {"archive", old.Archive, nw.Archive},
		{"archiveSHA256", old.ArchiveSHA256, nw.ArchiveSHA256}, {"treeSHA256", old.TreeSHA256, nw.TreeSHA256},
		{"licenseSHA256", old.LicenseSHA256, nw.LicenseSHA256}, {"noticeSHA256", old.NoticeSHA256, nw.NoticeSHA256},
	} {
		if err := set(f.k, f.o, f.n); err != nil {
			return "", err
		}
	}
	for k, o := range old.Files {
		if err := set(k, o, nw.Files[k]); err != nil {
			return "", err
		}
	}
	return head + tail[:bs] + block + tail[be:], nil
}

func (c config) bump() (result, error) {
	var res result
	if !nameRE.MatchString(c.Grammar) || !tagRE.MatchString(c.Tag) {
		return res, errors.New("grammar or tag has an unexpected form (a stable release tag such as v1.2.3)")
	}
	group, err := groupOf(c.Pins, c.Grammar)
	if err != nil {
		return res, err
	}
	m := repoRE.FindStringSubmatch(group[0].Old.Repo)
	repoPath, repoName := m[1], m[2]
	oldCommit := group[0].Old.Commit

	commit, err := c.resolveTag(repoPath, c.Tag)
	if err != nil {
		return res, err
	}
	released := true
	var rel struct{ Draft, Prerelease bool }
	switch code, err := c.getJSON(c.API+"/repos/"+repoPath+"/releases/tags/"+c.Tag, &rel); {
	case code == http.StatusNotFound:
		released = false // a tag without a GitHub Release is allowed (S94)
	case err != nil:
		return res, err
	case rel.Draft || rel.Prerelease:
		return res, fmt.Errorf("release %s is a draft or prerelease", c.Tag)
	}
	var cmp struct{ Status string }
	if _, err := c.getJSON(c.API+"/repos/"+repoPath+"/compare/"+oldCommit+"..."+commit, &cmp); err != nil {
		return res, err
	}
	if cmp.Status != "ahead" {
		return res, fmt.Errorf("%s (%s) is %q relative to the pinned %s: v1 only moves forward", c.Tag, commit[:12], cmp.Status, oldCommit[:12])
	}

	var hashed []string
	for _, g := range group {
		hashed = append(hashed, g.Old.Dirs...)
		if !hasDir(g.Old.Dirs, g.Old.GrammarDir) {
			return res, fmt.Errorf("grammar %s: grammarDir is outside its hashed dirs", g.Name)
		}
	}
	keep := func(rel string) bool {
		if rel == "LICENSE" || rel == "NOTICE" || rel == "CHANGELOG.md" {
			return true
		}
		return hasDir(hashed, rel)
	}
	b, code, err := c.get(c.API+"/repos/"+repoPath+"/tarball/"+commit, maxArchive)
	if err != nil {
		return res, err
	}
	if code != http.StatusOK {
		return res, fmt.Errorf("tarball: HTTP %d", code)
	}
	files, err := readArchive(bytes.NewReader(b), keep, hashed)
	if err != nil {
		return res, err
	}
	changelog, ok := files["CHANGELOG.md"]
	if !ok {
		return res, errors.New("the new tree has no CHANGELOG.md: an owner pin move cites the owner CHANGELOG (AGENTS.md)")
	}

	text := string(c.Pins)
	for i := range group {
		if group[i].New, err = derive(group[i].Old, repoName, commit, sum(b), files); err != nil {
			return res, fmt.Errorf("grammar %s: %w", group[i].Name, err)
		}
		if text, err = patchEntry(text, group[i].Name, group[i].Old, group[i].New); err != nil {
			return res, err
		}
	}
	// The edited text must equal the old document with exactly the group's fields changed.
	if err := checkEdit(c.Pins, []byte(text), group); err != nil {
		return res, err
	}
	res.Pins = []byte(text)
	res.Candidate, _ = json.Marshal(map[string]string{"Kind": "grammar", "Name": c.Grammar, "Tag": c.Tag, "Commit": commit})
	res.Summary = summary(group, repoPath, c.Tag, commit, released, changelog)
	return res, nil
}

func hasDir(dirs []string, rel string) bool {
	for _, d := range dirs {
		if rel == d || strings.HasPrefix(rel, d+"/") {
			return true
		}
	}
	return false
}

func checkEdit(before, after []byte, group []change) error {
	var a, b map[string]any
	if err := json.Unmarshal(before, &a); err != nil {
		return err
	}
	if err := json.Unmarshal(after, &b); err != nil {
		return fmt.Errorf("edited pins.json is not valid JSON: %w", err)
	}
	ag := a["grammars"].(map[string]any)
	for _, g := range group {
		e := ag[g.Name].(map[string]any)
		nb, _ := json.Marshal(g.New)
		var nm map[string]any
		_ = json.Unmarshal(nb, &nm)
		for k, v := range nm {
			if k == "noticeSHA256" && g.New.NoticeSHA256 == "" {
				continue
			}
			e[k] = v
		}
	}
	if !reflect.DeepEqual(a, b) {
		return errors.New("internal check: the edited pins.json differs from the expected document")
	}
	return nil
}

func summary(group []change, repoPath, tag, commit string, released bool, changelog []byte) string {
	var s strings.Builder
	names := make([]string, len(group))
	for i, g := range group {
		names[i] = "`" + g.Name + "`"
	}
	old := group[0].Old.Commit
	web := "https://github.com/" + repoPath
	fmt.Fprintf(&s, "Move the %s pin to owner release `%s` (`%s`).\n\n", strings.Join(names, ", "), tag, commit)
	fmt.Fprintf(&s, "- Source: owner repository %s (ADR-0024 rule 7); previous pin `%s`.\n", web, old)
	if released {
		fmt.Fprintf(&s, "- Owner release: %s/releases/tag/%s\n", web, tag)
	} else {
		fmt.Fprintf(&s, "- Owner release: none; `%s` is a tag without a GitHub Release.\n", tag)
	}
	fmt.Fprintf(&s, "- Owner CHANGELOG: %s/blob/%s/CHANGELOG.md\n", web, tag)
	if v := strings.TrimPrefix(tag, "v"); !bytes.Contains(changelog, []byte(v)) {
		fmt.Fprintf(&s, "- WARNING: that CHANGELOG.md does not mention %q; check it before citing.\n", v)
	}
	fmt.Fprintf(&s, "- Compare: %s/compare/%s...%s\n", web, old[:12], commit[:12])
	fmt.Fprintf(&s, "- archiveSHA256 `%s` (GitHub API tarball of the commit)\n", group[0].New.ArchiveSHA256)
	for _, g := range group {
		lic := "unchanged"
		if g.Old.LicenseSHA256 != g.New.LicenseSHA256 {
			lic = "CHANGED " + g.Old.LicenseSHA256 + " -> " + g.New.LicenseSHA256
		}
		var fc []string
		for k := range g.New.Files {
			if g.Old.Files[k] != g.New.Files[k] {
				fc = append(fc, k)
			}
		}
		sort.Strings(fc)
		if len(fc) == 0 {
			fc = []string{"none"}
		}
		fmt.Fprintf(&s, "- %s: treeSHA256 `%s`, files changed: %s, LICENSE %s", g.Name, g.New.TreeSHA256, strings.Join(fc, ", "), lic)
		if g.Old.NoticeSHA256 != g.New.NoticeSHA256 {
			s.WriteString(", NOTICE CHANGED")
		}
		s.WriteString("\n")
	}
	s.WriteString("\nOnly tools/gen/pins.json (written by tools/gen/pinbump) and the regenerated grammars/<name>/ files change.\n")
	s.WriteString("\n## Evidence\n\n- [ ] regenerate drift check (`regen`) green\n- [ ] pin-move evidence reviewed: <link>\n- [ ] generated modules equal the autopr-regenerated artifact: <run link>\n")
	return s.String()
}

func main() {
	pinsPath := flag.String("pins", "tools/gen/pins.json", "pins.json to read (rewritten in place without -out)")
	grammar := flag.String("grammar", "", "grammar name (an owner-repository grammar)")
	tag := flag.String("tag", "", "release tag of the pinned repository, for example v1.2.3")
	out := flag.String("out", "", "write pins.json, candidate.json and summary.md here instead of rewriting -pins")
	api := flag.String("api", defaultAPI, "GitHub API base URL (tests only)")
	flag.Parse()
	if *grammar == "" || *tag == "" {
		flag.Usage()
		os.Exit(2)
	}
	b, err := os.ReadFile(*pinsPath)
	if err != nil {
		fail(err)
	}
	c := config{Pins: b, Grammar: *grammar, Tag: *tag, API: strings.TrimRight(*api, "/"),
		Client: &http.Client{Timeout: 5 * time.Minute}, Token: os.Getenv("GITHUB_TOKEN")}
	apiHost := mustHost(c.API)
	c.Client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		// The tarball endpoint redirects to codeload.github.com; follow nothing else.
		if len(via) > 5 || (r.URL.Host != apiHost && r.URL.Host != "codeload.github.com") {
			return fmt.Errorf("refused redirect to %s", r.URL.Host)
		}
		return nil
	}
	res, err := c.bump()
	if err != nil {
		fail(err)
	}
	if *out == "" {
		if err := os.WriteFile(*pinsPath, res.Pins, 0o644); err != nil {
			fail(err)
		}
	} else {
		if err := os.MkdirAll(*out, 0o755); err != nil {
			fail(err)
		}
		for name, data := range map[string][]byte{"pins.json": res.Pins, "candidate.json": res.Candidate, "summary.md": []byte(res.Summary)} {
			if err := os.WriteFile(filepath.Join(*out, name), data, 0o644); err != nil {
				fail(err)
			}
		}
	}
	fmt.Print(res.Summary)
}

func mustHost(base string) string {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		fail(fmt.Errorf("bad -api %q", base))
	}
	return u.Host
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "pinbump: refused:", err)
	os.Exit(1)
}
