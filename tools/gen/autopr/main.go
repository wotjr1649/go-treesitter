// Command autopr is the trust boundary of the upstream auto-PR workflow (.github/workflows/upstream-autopr.yml).
//
// The privileged job (contents: write) never executes what earlier jobs produced; it only copies data, and only after
// this check: the proposed tools/gen/pins.json equals the checked-out one except for ONE existing grammar entry that keeps
// its repository and moves to the candidate commit, and the regenerated files are regular files under grammars/<name>/
// (go.mod and go.sum unchanged). Everything else is refused. Stdlib only.
//
//	autopr -main <checkout> -proposal <dir> [-regenerated <dir>] [-apply]
//
// <proposal> holds pins.json and candidate.json; <regenerated> is the artifact tree rooted at the repository root. On
// success it prints name=, tag= and commit= (each validated against a fixed pattern, safe for GITHUB_OUTPUT).
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
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

// checkPins refuses a proposal that is anything but one existing grammar entry moved to the candidate commit.
func checkPins(mainPins, proposal []byte, c candidate) error {
	if c.Kind != "grammar" || !nameRE.MatchString(c.Name) || !tagRE.MatchString(c.Tag) || !commitRE.MatchString(c.Commit) {
		return errors.New("candidate.json: only {kind: grammar, name, tag, commit} with fixed patterns is accepted")
	}
	a, err := asMap(mainPins)
	if err != nil {
		return err
	}
	b, err := asMap(proposal)
	if err != nil {
		return err
	}
	ag, _ := a["grammars"].(map[string]any)
	bg, _ := b["grammars"].(map[string]any)
	old, ok1 := ag[c.Name].(map[string]any)
	cur, ok2 := bg[c.Name].(map[string]any)
	if !ok1 || !ok2 {
		return fmt.Errorf("grammar %q must exist in both pins files (the bot never adds or removes one)", c.Name)
	}
	delete(ag, c.Name)
	delete(bg, c.Name)
	if !reflect.DeepEqual(a, b) {
		return errors.New("pins.json differs outside the candidate grammar entry")
	}
	if cur["repo"] != old["repo"] || cur["commit"] != c.Commit || old["commit"] == c.Commit {
		return errors.New("the entry must keep its repo and move to the candidate commit")
	}
	return checkURLs(cur)
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
func checkFiles(files []file, name string, mainRoot func(string) ([]byte, error)) error {
	var total int64
	if len(files) > maxFiles {
		return fmt.Errorf("%d files (limit %d)", len(files), maxFiles)
	}
	for _, f := range files {
		total += f.Size
		if !f.Regular || path.Clean(f.Path) != f.Path || strings.ContainsAny(f.Path, "\\:") || !strings.HasPrefix(f.Path, "grammars/"+name+"/") {
			return fmt.Errorf("refused %q: only regular files under grammars/%s/", f.Path, name)
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

func run(mainRoot, proposal, regenerated string, apply bool) (candidate, error) {
	var c candidate
	cb, err := os.ReadFile(filepath.Join(proposal, "candidate.json"))
	if err != nil {
		return c, err
	}
	dec := json.NewDecoder(bytes.NewReader(cb))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, err
	}
	pinsPath := filepath.Join(mainRoot, "tools/gen/pins.json")
	mp, err := os.ReadFile(pinsPath)
	if err != nil {
		return c, err
	}
	pp, err := os.ReadFile(filepath.Join(proposal, "pins.json"))
	if err != nil {
		return c, err
	}
	if err := checkPins(mp, pp, c); err != nil {
		return c, err
	}
	var files []file
	if regenerated != "" {
		if files, err = walk(regenerated); err != nil {
			return c, err
		}
		read := func(p string) ([]byte, error) { return os.ReadFile(filepath.Join(mainRoot, filepath.FromSlash(p))) }
		if err := checkFiles(files, c.Name, read); err != nil {
			return c, err
		}
	}
	if !apply {
		return c, nil
	}
	if err := os.WriteFile(pinsPath, pp, 0o644); err != nil {
		return c, err
	}
	for _, f := range files {
		dst := filepath.Join(mainRoot, filepath.FromSlash(f.Path))
		data, err := os.ReadFile(filepath.Join(regenerated, filepath.FromSlash(f.Path)))
		if err != nil {
			return c, err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return c, err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return c, err
		}
	}
	return c, nil
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
	c, err := run(*mainRoot, *proposal, *regenerated, *apply)
	if err != nil {
		fmt.Fprintln(os.Stderr, "autopr: refused:", err)
		os.Exit(1)
	}
	fmt.Printf("name=%s\ntag=%s\ncommit=%s\n", c.Name, c.Tag, c.Commit)
}
