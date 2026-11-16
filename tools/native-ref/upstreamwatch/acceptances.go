package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

//go:embed accepted-warnings.json
var acceptedWarningsJSON []byte

type acceptance struct {
	Component string `json:"component"`
	Kind      string `json:"kind"`
	Accepted  string `json:"accepted"`
	Expires   string `json:"expires"`
	Reason    string `json:"reason"`
}

func loadAcceptances(raw []byte) ([]acceptance, error) {
	var entries []acceptance
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&entries); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("acceptances: expected one JSON list")
	}
	seen := map[[2]string]bool{}
	for _, a := range entries {
		start, startErr := time.Parse(time.DateOnly, a.Accepted)
		end, endErr := time.Parse(time.DateOnly, a.Expires)
		key := [2]string{a.Component, a.Kind}
		if a.Component == "" || a.Kind == "" || !strings.Contains(a.Reason, "](https://") ||
			startErr != nil || endErr != nil || !start.Before(end) || seen[key] {
			return nil, fmt.Errorf("invalid or duplicate acceptance for %q / %q", a.Component, a.Kind)
		}
		seen[key] = true
	}
	return entries, nil
}

func (a *githubAPI) accepted(f finding) *acceptance {
	today := a.now.UTC().Format(time.DateOnly)
	for i := range a.acceptances {
		entry := &a.acceptances[i]
		if entry.Component == f.component && entry.Kind == f.kind && entry.Accepted <= today && today < entry.Expires {
			return entry
		}
	}
	return nil
}

// warningSummary is shared by the log, issue body and the workflow's auto-close decision.
func warningSummary(w io.Writer, api *githubAPI) int {
	var warnings, accepted []finding
	for _, f := range api.findings {
		if api.accepted(f) != nil {
			accepted = append(accepted, f)
		} else {
			warnings = append(warnings, f)
		}
	}
	cell := func(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "|", `\|`), "\n", " ") }
	fmt.Fprintf(w, "**%d warning(s)**\n\n| component | warning |\n|---|---|\n", len(warnings))
	for _, f := range warnings {
		fmt.Fprintf(w, "| %s | %s |\n", cell(f.component), cell(f.text))
	}
	fmt.Fprintln(w, "\n## Accepted warnings")
	fmt.Fprintln(w, "\n| component | warning | accepted | expires (UTC, exclusive) | reason |\n|---|---|---|---|---|")
	for _, f := range accepted {
		a := api.accepted(f)
		fmt.Fprintf(w, "| %s | %s | %s | %s | %s |\n", cell(f.component), cell(f.text), a.Accepted, a.Expires, cell(a.Reason))
	}
	fmt.Fprintln(w, "\n## Stale acceptances (no current matching warning)")
	fmt.Fprintln(w, "\n| component | warning kind | accepted | expires | reason |\n|---|---|---|---|---|")
	for _, a := range api.acceptances {
		matched := false
		for _, f := range api.findings {
			if f.component == a.Component && f.kind == a.Kind {
				matched = true
				break
			}
		}
		if !matched {
			fmt.Fprintf(w, "| %s | %s | %s | %s | %s |\n", cell(a.Component), cell(a.Kind), a.Accepted, a.Expires, cell(a.Reason))
		}
	}
	return len(warnings)
}
