package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// aggregate is the last job: it downloads the artifacts of the measurement jobs, validates each with the strict collector
// (allowlisted names, bounded reads, schemas checked against the run's own plan), builds the report from the validated
// copy and writes one summary. It executes nothing from an artifact and writes nothing but its own output directory.
func cmdAggregate(args []string) error {
	fs := flag.NewFlagSet("aggregate", flag.ExitOnError)
	in := fs.String("in", "", "directory of downloaded artifacts (one subdirectory each)")
	out := fs.String("out", "", "output directory (summary.md and the validated copies)")
	runners := fs.String("runners", "ubuntu-24.04,ubuntu-24.04-arm", "runner names of the matrix")
	fs.Parse(args)
	if *in == "" || *out == "" {
		return errors.New("aggregate needs -in and -out")
	}
	var problems []string
	var md strings.Builder
	md.WriteString("# Comparative benchmark: all jobs\n\n")
	for _, r := range strings.Split(*runners, ",") {
		for _, l := range allLangs {
			name := "compare-" + r + "-" + l.name
			sec, err := aggregateOne(filepath.Join(*in, name), filepath.Join(*out, name), false)
			if err != nil {
				problems = append(problems, name+": "+err.Error())
				fmt.Fprintf(&md, "## %s / %s\n\n**not accepted:** %s\n\n", mdEscape(r), l.name, mdEscape(err.Error()))
				continue
			}
			fmt.Fprintf(&md, "## %s / %s\n\n%s\n\n", mdEscape(r), l.name, sec)
		}
		name := "compare-nocc-" + r
		sec, err := aggregateOne(filepath.Join(*in, name), filepath.Join(*out, name), true)
		if err != nil {
			problems = append(problems, name+": "+err.Error())
			fmt.Fprintf(&md, "## no-C-compiler build / %s\n\n**not accepted:** %s\n\n", mdEscape(r), mdEscape(err.Error()))
			continue
		}
		fmt.Fprintf(&md, "## no-C-compiler build / %s\n\n%s\n\n", mdEscape(r), sec)
	}
	if err := writeFileAll(filepath.Join(*out, "summary.md"), []byte(md.String())); err != nil {
		return err
	}
	if err := cmdSeal([]string{"-dir", *out}); err != nil {
		return err
	}
	if len(problems) > 0 {
		return fmt.Errorf("%d artifact(s) missing or rejected: %s", len(problems), printable(strings.Join(problems, "; "), 1200))
	}
	return nil
}

// aggregateOne validates one artifact directory and returns its Markdown section.
func aggregateOne(in, out string, nocc bool) (string, error) {
	if st, err := os.Lstat(in); err != nil || !st.IsDir() {
		return "", errors.New("artifact missing")
	}
	if err := cmdCollect([]string{"-in", in, "-out", out}); err != nil {
		return "", err
	}
	if nocc {
		if err := cmdGateNocc([]string{"-builds", filepath.Join(out, "builds.csv")}); err != nil {
			return "", err
		}
		var md strings.Builder
		if err := appendCSVSummary(&md, filepath.Join(out, "builds.csv"), "Build outcomes"); err != nil {
			return "", err
		}
		return md.String(), nil
	}
	if err := cmdReport([]string{"-in", out}); err != nil {
		return "", err
	}
	b, err := readBounded(filepath.Join(out, "summary.md"), maxFile)
	if err != nil {
		return "", err
	}
	// sections are nested one level down so the headings of the per-job reports stay below this document's own
	return strings.ReplaceAll(strings.ReplaceAll(string(b), "\n## ", "\n### "), "# Comparative benchmark summary", "Summary"), nil
}
