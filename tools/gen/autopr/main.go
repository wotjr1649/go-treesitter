// Command autopr is the trust boundary of the upstream auto-PR workflow (.github/workflows/upstream-autopr.yml).
//
// The workflow's evidence job (no token, owner approval) takes what the propose job produced only as data, and only after
// this check: the proposed tools/gen/pins.json equals the checked-out one except for ONE existing grammar entry that keeps
// its repository and its set of fields and moves to the candidate commit (grammars that share that repository commit, such as typescript and tsx,
// move together: one pin), and the regenerated files are regular files under grammars/<name>/
// (go.mod and go.sum unchanged). Everything else is refused. Stdlib only.
//
//	autopr -main <checkout> -proposal <dir> [-regenerated <dir>] [-apply]
//
// <proposal> holds pins.json and candidate.json; <regenerated> is the artifact tree rooted at the repository root. On
// success it prints name=, tag=, commit= and grammars= (the moved entries, space separated; each validated against a fixed pattern, safe for GITHUB_OUTPUT).
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
)

const (
	maxFiles = 400
	maxBytes = 64 << 20
)

var (
	nameRE   = regexp.MustCompile(`^[a-z0-9]+$`)
	tagRE    = regexp.MustCompile(`^v?[0-9]+(\.[0-9]+){1,3}$`)
	commitRE = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

type candidate struct{ Kind, Name, Tag, Commit string }

type file struct {
	Path    string // slash-separated, relative to the repository root
	Regular bool
	Size    int64
	Data    []byte // read only for go.mod and go.sum
}

func asMap(b []byte) (map[string]any, error) {
	var m map[string]any
	return m, json.Unmarshal(b, &m)
}

// checkPins refuses a proposal that is anything but one existing grammar entry (with the entries that share its repository
// commit) moved to the candidate commit. It returns the names of the moved entries.
func checkPins(mainPins, proposal []byte, c candidate) ([]string, error) {
	if c.Kind != "grammar" || !nameRE.MatchString(c.Name) || !tagRE.MatchString(c.Tag) || !commitRE.MatchString(c.Commit) {
		return nil, errors.New("candidate.json: only {kind: grammar, name, tag, commit} with fixed patterns is accepted")
	}
	a, err := asMap(mainPins)
	if err != nil {
		return nil, err
	}
	b, err := asMap(proposal)
	if err != nil {
		return nil, err
	}
	ag, _ := a["grammars"].(map[string]any)
	bg, _ := b["grammars"].(map[string]any)
	target, ok := ag[c.Name].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("grammar %q must exist in the checked-out pins file (the bot never adds one)", c.Name)
	}
	var names []string
	for n, e := range ag {
		if m, _ := e.(map[string]any); m["repo"] == target["repo"] && m["commit"] == target["commit"] {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		old := ag[n].(map[string]any)
		cur, ok := bg[n].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("grammar %q must exist in both pins files (the bot never removes one)", n)
		}
		if cur["repo"] != old["repo"] || cur["commit"] != c.Commit || old["commit"] == c.Commit {
			return nil, fmt.Errorf("entry %s must keep its repo and move to the candidate commit", n)
		}
		if !slices.Equal(slices.Sorted(maps.Keys(cur)), slices.Sorted(maps.Keys(old))) { // no new archiveURL, patches, npmDependencies...
			return nil, fmt.Errorf("entry %s must keep its set of fields", n)
		}
		if err := checkURLs(cur); err != nil {
			return nil, err
		}
		delete(ag, n)
		delete(bg, n)
	}
	if !reflect.DeepEqual(a, b) {
		return nil, errors.New("pins.json differs outside the candidate's entries")
	}
	return names, nil
}

// checkURLs allows only GitHub hosts in the new entry, at any depth.
func checkURLs(v any) error {
	switch x := v.(type) {
	case string:
		if strings.Contains(x, "://") && !strings.HasPrefix(x, "https://github.com/") && !strings.HasPrefix(x, "https://codeload.github.com/") {
			return fmt.Errorf("URL outside github.com: %q", x)
		}
	case map[string]any:
		for _, e := range x {
			if err := checkURLs(e); err != nil {
				return err
			}
		}
	case []any:
		for _, e := range x {
			if err := checkURLs(e); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkFiles refuses anything but regular files of the candidate's own module directory.
func checkFiles(files []file, names []string, mainRoot func(string) ([]byte, error)) error {
	var total int64
	if len(files) > maxFiles {
		return fmt.Errorf("%d files (limit %d)", len(files), maxFiles)
	}
	for _, f := range files {
		total += f.Size
		if !f.Regular || path.Clean(f.Path) != f.Path || strings.ContainsAny(f.Path, "\\:") || !inModules(f.Path, names) {
			return fmt.Errorf("refused %q: only regular files under grammars/%s/", f.Path, strings.Join(names, "/ or grammars/"))
		}
		if b := path.Base(f.Path); b == "go.mod" || b == "go.sum" {
			old, err := mainRoot(f.Path) // a pin move must not change module requirements
			if err != nil || !bytes.Equal(old, f.Data) {
				return fmt.Errorf("refused %q: differs from the checked-out file", f.Path)
			}
		}
	}
	if total > maxBytes {
		return fmt.Errorf("%d bytes (limit %d)", total, maxBytes)
	}
	return nil
}

func inModules(p string, names []string) bool {
	for _, n := range names {
		if strings.HasPrefix(p, "grammars/"+n+"/") {
			return true
		}
	}
	return false
}

func walk(root string) ([]file, error) {
	var out []file
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info() // WalkDir does not follow links: a link has no regular mode
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		f := file{Path: filepath.ToSlash(rel), Regular: info.Mode().IsRegular(), Size: info.Size()}
		if b := path.Base(f.Path); f.Regular && (b == "go.mod" || b == "go.sum") {
			f.Data, err = os.ReadFile(p)
		}
		out = append(out, f)
		return err
	})
	return out, err
}

func run(mainRoot, proposal, regenerated string, apply bool) (candidate, []string, error) {
	var c candidate
	cb, err := os.ReadFile(filepath.Join(proposal, "candidate.json"))
	if err != nil {
		return c, nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(cb))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, nil, err
	}
	pinsPath := filepath.Join(mainRoot, "tools/gen/pins.json")
	mp, err := os.ReadFile(pinsPath)
	if err != nil {
		return c, nil, err
	}
	pp, err := os.ReadFile(filepath.Join(proposal, "pins.json"))
	if err != nil {
		return c, nil, err
	}
	names, err := checkPins(mp, pp, c)
	if err != nil {
		return c, nil, err
	}
	var files []file
	if regenerated != "" {
		if files, err = walk(regenerated); err != nil {
			return c, nil, err
		}
		read := func(p string) ([]byte, error) { return os.ReadFile(filepath.Join(mainRoot, filepath.FromSlash(p))) }
		if err := checkFiles(files, names, read); err != nil {
			return c, nil, err
		}
	}
	if !apply {
		return c, names, nil
	}
	if err := os.WriteFile(pinsPath, pp, 0o644); err != nil {
		return c, nil, err
	}
	for _, f := range files {
		dst := filepath.Join(mainRoot, filepath.FromSlash(f.Path))
		data, err := os.ReadFile(filepath.Join(regenerated, filepath.FromSlash(f.Path)))
		if err != nil {
			return c, nil, err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return c, nil, err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return c, nil, err
		}
	}
	return c, names, nil
}

func main() {
	mainRoot := flag.String("main", "", "checkout of the reviewed main branch")
	proposal := flag.String("proposal", "", "directory with pins.json and candidate.json")
	regenerated := flag.String("regenerated", "", "artifact tree rooted at the repository root (optional)")
	apply := flag.Bool("apply", false, "copy the validated files into -main")
	flag.Parse()
	if *mainRoot == "" || *proposal == "" {
		flag.Usage()
		os.Exit(2)
	}
	c, names, err := run(*mainRoot, *proposal, *regenerated, *apply)
	if err != nil {
		fmt.Fprintln(os.Stderr, "autopr: refused:", err)
		os.Exit(1)
	}
	fmt.Printf("name=%s\ntag=%s\ncommit=%s\ngrammars=%s\n", c.Name, c.Tag, c.Commit, strings.Join(names, " "))
}
