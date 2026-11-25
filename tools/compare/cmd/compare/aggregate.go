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
	in := fs.String("in", "", "directory of downloaded bundles (compare-<runner>-<lang>.zip, compare-nocc-<runner>.zip)")
	out := fs.String("out", "", "output directory (summary.md and SHA256SUMS; the validated copies live in -work)")
	work := fs.String("work", "", "directory for the validated per-bundle copies (never uploaded)")
	runners := fs.String("runners", "ubuntu-24.04,ubuntu-24.04-arm", "runner names of the matrix")
	langsFlag := fs.String("langs", "c,go,javascript,json,python", "languages of the matrix")
	fs.Parse(args)
	if *in == "" || *out == "" || *work == "" {
		return errors.New("aggregate needs -in, -out and -work")
	}
	var problems []string
	var md strings.Builder
	md.WriteString("# Comparative benchmark: all jobs\n\n")
	for _, r := range strings.Split(*runners, ",") {
		for _, l := range strings.Split(*langsFlag, ",") {
			name := "compare-" + r + "-" + l
			sec, err := aggregateOne(filepath.Join(*in, name), filepath.Join(*work, name), false)
			if err != nil {
				problems = append(problems, name+": "+err.Error())
				fmt.Fprintf(&md, "## %s / %s\n\n**not accepted:** %s\n\n", mdEscape(r), mdEscape(l), mdEscape(err.Error()))
				continue
			}
			fmt.Fprintf(&md, "## %s / %s\n\n%s\n\n", mdEscape(r), mdEscape(l), sec)
		}
		name := "compare-nocc-" + r
		sec, err := aggregateOne(filepath.Join(*in, name), filepath.Join(*work, name), true)
		if err != nil {
			problems = append(problems, name+": "+err.Error())
			fmt.Fprintf(&md, "## no-C-compiler build / %s\n\n**not accepted:** %s\n\n", mdEscape(r), mdEscape(err.Error()))
			continue
		}
		fmt.Fprintf(&md, "## no-C-compiler build / %s\n\n%s\n\n", mdEscape(r), sec)
	}
	if len(problems) > 0 {
		return fmt.Errorf("%d artifact(s) missing or rejected: %s", len(problems), printable(strings.Join(problems, "; "), 1200))
	}
	// only a fully validated aggregate leaves anything for upload: the summary and the hashes of what it was built from
	if err := writeFileAll(filepath.Join(*out, "summary.md"), []byte(md.String())); err != nil {
		return err
	}
	sums, err := sumsOf(*work)
	if err != nil {
		return err
	}
	return writeFileAll(filepath.Join(*out, "SHA256SUMS"), []byte(sums))
}

// bundleFile finds the downloaded bundle of an artifact: the file itself, or the single regular file in a directory of that name.
func bundleFile(in, name string) (string, error) {
	p := filepath.Join(in, name)
	st, err := os.Lstat(p)
	if err != nil {
		return "", errors.New("artifact missing")
	}
	if st.Mode().IsRegular() {
		return p, nil
	}
	if !st.IsDir() {
		return "", errors.New("artifact is neither a file nor a directory")
	}
	ents, err := os.ReadDir(p)
	if err != nil || len(ents) != 1 {
		return "", errors.New("artifact directory must hold exactly one file")
	}
	return filepath.Join(p, ents[0].Name()), nil
}

// aggregateOne unpacks one bundle with the bounded extractor, validates the extracted copy with the strict collector and
// returns its Markdown section.
func aggregateOne(in, out string, nocc bool) (string, error) {
	file, err := bundleFile(filepath.Dir(in), filepath.Base(in)+".zip")
	if err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp("", "compare-unpack-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if _, err := unpackBounded(file, tmp); err != nil {
		return "", err
	}
	if err := cmdCollect([]string{"-in", tmp, "-out", out}); err != nil {
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
