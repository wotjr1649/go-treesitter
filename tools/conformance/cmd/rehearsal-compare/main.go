// Command rehearsal-compare classifies two rehearsal tables (base, candidate) and writes summary.md and diff.tsv.
// Report-only: the exit status is 1 only when the tables cannot be compared (inputs missing or different).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wotjr1649/go-treesitter/tools/conformance/rehearsal"
)

func read(path string) []rehearsal.Row {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer f.Close()
	rows, err := rehearsal.Parse(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, path+":", err)
		os.Exit(2)
	}
	return rows
}

func main() {
	base := flag.String("base", "", "base table")
	cand := flag.String("cand", "", "candidate table")
	out := flag.String("out", "", "output directory (created; summary.md and diff.tsv)")
	title := flag.String("title", "", "identity text for the summary (runtime, commit, hashes)")
	idCap := flag.Int("id-cap", 50, "input IDs listed per category in the summary")
	flag.Parse()
	if *base == "" || *cand == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: rehearsal-compare -base b.tsv -cand c.tsv -out dir [-title text]")
		os.Exit(2)
	}
	res := rehearsal.Compare(read(*base), read(*cand))
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	d, err := os.Create(filepath.Join(*out, "diff.tsv"))
	if err == nil {
		err = res.WriteDiffs(d)
		if cerr := d.Close(); err == nil {
			err = cerr
		}
	}
	summary := res.Summary(*title, *idCap)
	if err == nil {
		err = os.WriteFile(filepath.Join(*out, "summary.md"), []byte(summary), 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Print(summary)
	if len(res.Problems) > 0 {
		os.Exit(1)
	}
}
