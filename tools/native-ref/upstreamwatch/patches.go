package main

// Grammars that carry ADR-0023 patches: count (pins.json) and the age of each patch, measured per upstream report from
// when a patch for it first reached main. A patch older than 12 months needs the owner's re-decision (a warning).
// Unreadable patch metadata or unusable git history (no repository, shallow checkout) is a diagnostic of this section
// only: no warning, no failure. Optional metadata must never add a hard error.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	patchAlert = 3 // tools/gen -patch-alert default
	patchLimit = 5 // ADR-0023 amendment 2026-11-18, decision 3
)

type patchRec struct{ file, report string }

// loadPatches returns the patch records (file relative to the pins file, upstreamReport URL) of each grammar that has any.
// A grammar whose "patches" cannot be read is left out and named in the error (the first one, in name order); the
// rest still load. raw already parsed as pins, so only the optional patch fields can fail here.
func loadPatches(raw []byte) (map[string][]patchRec, error) {
	var p struct {
		Grammars map[string]struct {
			Patches json.RawMessage `json:"patches"`
		} `json:"grammars"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(p.Grammars))
	for n := range p.Grammars {
		names = append(names, n)
	}
	sort.Strings(names)
	out, first := map[string][]patchRec{}, error(nil)
	for _, n := range names {
		var list []struct {
			File   string `json:"file"`
			Report string `json:"upstreamReport"`
		}
		if err := json.Unmarshal(p.Grammars[n].Patches, &list); err != nil && len(p.Grammars[n].Patches) > 0 {
			if first == nil {
				first = fmt.Errorf("grammar %s: patches: %w", n, err)
			}
			continue
		}
		for _, patch := range list {
			out[n] = append(out[n], patchRec{patch.File, patch.Report})
		}
	}
	return out, first
}

// gitFirstOnMain returns a function giving, per upstreamReport URL, the date of the first commit on the first-parent
// history of HEAD (main in the workflow) whose change to the pins file added that URL: the merge that first brought a
// patch for the report, whatever the patch file is called later (ADR-0023 amendment, item 4). It errors when the
// history cannot be trusted. Limits: the URL must be the recorded upstreamReport (a bundled report listed only in a
// README is not seen), the pins file must keep its path, and on a feature branch HEAD's first parent is the branch.
func gitFirstOnMain(pinsFile string) (func(report string) (time.Time, error), error) {
	dir, base := filepath.Dir(pinsFile), filepath.Base(pinsFile)
	git := func(args ...string) (string, error) {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
		return strings.TrimSpace(string(out)), err
	}
	shallow, err := git("rev-parse", "--is-shallow-repository")
	if err != nil {
		return nil, fmt.Errorf("no git repository at %s (%v)", dir, err)
	}
	if shallow == "true" { // a shallow boundary commit would show every record as just added
		return nil, errors.New("shallow checkout; the workflow needs fetch-depth: 0")
	}
	return func(report string) (time.Time, error) {
		// The quoted URL is matched whole, so issues/35 does not match issues/350.
		out, err := git("log", "--first-parent", "--reverse", "-S"+strconv.Quote(report), "--format=%cI", "--", base)
		if err != nil {
			return time.Time{}, err
		}
		lines := strings.Fields(out)
		if len(lines) == 0 {
			return time.Time{}, errors.New("not in git history (uncommitted?)")
		}
		return time.Parse(time.RFC3339, lines[0])
	}, nil
}

func patchSection(w io.Writer, api *githubAPI, patches map[string][]patchRec, metaErr error, added func(string) (time.Time, error), historyErr error) {
	fmt.Fprintln(w, "\n## Patch-carrying grammars (ADR-0023)")
	fmt.Fprintln(w)
	if metaErr != nil {
		fmt.Fprintf(w, "Patch metadata unreadable (%v); the grammars below are the readable ones.\n\n", metaErr)
	}
	if len(patches) == 0 {
		fmt.Fprintln(w, "No grammar carries a readable patch.")
		return
	}
	if historyErr != nil {
		fmt.Fprintf(w, "Patch ages unavailable: git history unusable (%v).\n\n", historyErr)
		added = nil
	}
	names := make([]string, 0, len(patches))
	for n := range patches {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Fprintln(w, "Age: first commit on HEAD's first-parent history (main in the workflow) that added the patch's `upstreamReport` to pins.json; renaming or rewriting the patch file does not restart it.")
	fmt.Fprintln(w, "Limit: only the recorded `upstreamReport` is tracked; bundled reports listed only in READMEs are not (that needs a pins schema change: an owner decision).")
	fmt.Fprintln(w, "A warning is per report (kind `patch older than 12 months: <report URL>`), so accepting one report does not hide another.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| grammar | patches | patch | upstream report | first on main | status |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|")
	warned := map[string]bool{}
	for _, n := range names {
		count := fmt.Sprint(len(patches[n]))
		switch {
		case len(patches[n]) > patchLimit:
			count += fmt.Sprintf(" **above the ADR-0023 limit of %d**", patchLimit)
		case len(patches[n]) > patchAlert:
			count += fmt.Sprintf(" **above -patch-alert %d**", patchAlert)
		}
		for _, rec := range patches[n] {
			date, status := "?", "age not measured"
			if added != nil && rec.report == "" {
				status = "age unknown: no upstreamReport"
			} else if added != nil {
				if t, err := added(rec.report); err != nil {
					status = "age unknown: " + err.Error()
				} else {
					date, status = t.Format(time.DateOnly), "ok"
					if t.Before(api.now.AddDate(-1, 0, 0)) {
						status = "**older than 12 months: owner re-decision**"
						if kind := "patch older than 12 months: " + rec.report; !warned[n+" "+kind] { // patches sharing a report warn once
							warned[n+" "+kind] = true
							api.warn(w, n, kind, fmt.Sprintf("patch %s: report %s first reached main %s; owner re-decision (ADR-0023)", rec.file, rec.report, date))
						}
					}
				}
			}
			fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s |\n", n, count, rec.file, rec.report, date, status)
		}
	}
}
