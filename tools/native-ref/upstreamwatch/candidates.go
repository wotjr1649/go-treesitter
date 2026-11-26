package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strings"
)

// candidatesVersion is the schema version of the -candidates file; change it with any incompatible field change.
const candidatesVersion = 1

// autoPRWorkflow is the placeholder name of the auto-PR workflow whose dispatch lines the issue body prints (the
// workflow is not merged yet). This command never dispatches anything.
const autoPRWorkflow = "upstream-autopr.yml"

// candidate is a pin the owner could move: the machine-readable form of a newer-release, newer-tag or behind-head finding.
// Kind is "release" (Newest is a release or, with TagOnly, a tag that has no GitHub Release) or "head" (Newest is the
// default-branch head commit of an owner repository). Grammar is false for tool and runtime pins. Accepted marks a
// candidate whose warning is covered by accepted-warnings.json.
type candidate struct {
	Component string `json:"component"`
	Pinned    string `json:"pinned"`
	Newest    string `json:"newest"`
	Kind      string `json:"kind"`
	Repo      string `json:"repo"`
	Owner     bool   `json:"owner"`
	TagOnly   bool   `json:"tagOnly"`
	Grammar   bool   `json:"grammar"`
	Accepted  bool   `json:"accepted"`
}

// propose records c. warnKind is the kind of the warning that reported it ("" for a failure, which is never accepted).
func (a *githubAPI) propose(c candidate, warnKind string) {
	c.Owner = isOwner(c.Repo)
	c.Accepted = warnKind != "" && a.accepted(finding{component: c.Component, kind: warnKind}) != nil
	a.candidates = append(a.candidates, c)
}

func (a *githubAPI) sortedCandidates() []candidate {
	list := slices.Clone(a.candidates)
	slices.SortFunc(list, func(x, y candidate) int {
		return strings.Compare(x.Component+"\x00"+x.Kind+"\x00"+x.Newest, y.Component+"\x00"+y.Kind+"\x00"+y.Newest)
	})
	return list
}

// writeCandidates writes the sorted candidates, one per line; after a hard error it writes nothing (as the issue body).
func writeCandidates(path string, api *githubAPI, failed bool) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if failed {
		return nil
	}
	lines := []string{}
	for _, c := range api.sortedCandidates() {
		b, err := json.Marshal(c)
		if err != nil {
			return err
		}
		lines = append(lines, "  "+string(b))
	}
	body := fmt.Sprintf("{\"version\":%d,\"candidates\":[", candidatesVersion)
	if len(lines) > 0 {
		body += "\n" + strings.Join(lines, ",\n") + "\n"
	}
	return os.WriteFile(path, []byte(body+"]}\n"), 0o644)
}

// safeName limits what reaches a shell command or markdown: tag names come from the GitHub API and may hold
// metacharacters (git allows ; $ and backticks in tag names).
var safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// candidatesSection prints one line per unaccepted candidate (text only). Only a grammar release or tag has a dispatch
// command; everything else is a manual pin move.
func candidatesSection(w io.Writer, api *githubAPI) {
	var lines []string
	for _, c := range api.sortedCandidates() {
		if c.Accepted {
			continue
		}
		what := "release"
		if c.TagOnly {
			what = "tag without a release"
		} else if c.Kind == "head" {
			what = "default-branch head"
		}
		if !c.Owner {
			what += ", third party"
		}
		if !safeName.MatchString(c.Component) || !safeName.MatchString(c.Newest) {
			lines = append(lines, "- (name not shown: unexpected characters) — no dispatch")
			continue
		}
		line := fmt.Sprintf("- `%s` %s `%s`", c.Component, what, c.Newest)
		if c.Grammar && c.Kind == "release" {
			line += fmt.Sprintf(": `gh workflow run %s -f grammar=%s -f tag=%s`", autoPRWorkflow, c.Component, c.Newest)
		} else {
			line += " — no dispatch (the pin moves by hand)"
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return
	}
	fmt.Fprintln(w, "\n## Pin-move candidates")
	fmt.Fprint(w, "\nThe watch job dispatches nothing; run a line below to start the auto-PR workflow for that candidate. The same list is the `upstream-watch-candidates` artifact of the run.\n\n")
	fmt.Fprintln(w, strings.Join(lines, "\n"))
}
