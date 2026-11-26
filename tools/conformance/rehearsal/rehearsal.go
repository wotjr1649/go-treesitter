// Package rehearsal compares two per-input result tables of a runtime-only rehearsal: the same corpus, edit and large
// inputs parsed by the committed grammar modules (base) and by modules generated against another tree-sitter runtime
// (candidate). Pure text processing, no engine, no grammar. TEST ONLY: a table is a local measurement, never evidence
// of parity (docs/specs/validation.md).
package rehearsal

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Header is the first line of a table; one tab-separated Row per input follows.
const Header = "#tsg-rehearsal/1"

const none = "-"

// Row is one input's result. Tree is the sha256 of the canonical dump of a fresh parse (or ERR), Fields that of the
// Fields() dump. For edit inputs Inc is the dump hash of the incremental reparse, IncEq whether it equals the fresh
// parse, Ranges the hash and RangesN the count of ChangedRanges; the other kinds carry "-" there.
type Row struct {
	Lang, ID, Kind, Src, Tree, Nodes, Err, Fields, Inc, IncEq, Ranges, RangesN string
}

func (r Row) cols() []string {
	return []string{r.Lang, r.ID, r.Kind, r.Src, r.Tree, r.Nodes, r.Err, r.Fields, r.Inc, r.IncEq, r.Ranges, r.RangesN}
}

func (r Row) String() string { return strings.Join(r.cols(), "\t") }

// Write writes the header and rows.
func Write(w io.Writer, rows []Row) error {
	if _, err := fmt.Fprintln(w, Header); err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := fmt.Fprintln(w, r.String()); err != nil {
			return err
		}
	}
	return nil
}

// Parse reads a table written by Write.
func Parse(r io.Reader) ([]Row, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var rows []Row
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if n == 1 {
			if line != Header {
				return nil, fmt.Errorf("line 1: want %q, got %q", Header, line)
			}
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 12 {
			return nil, fmt.Errorf("line %d: %d columns, want 12", n, len(f))
		}
		rows = append(rows, Row{f[0], f[1], f[2], f[3], f[4], f[5], f[6], f[7], f[8], f[9], f[10], f[11]})
	}
	return rows, sc.Err()
}

// Clean makes an error text safe for one cell.
func Clean(s string) string { return strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace(s) }

// NoEdit is the value of the edit-only columns of a corpus or large row.
func NoEdit() (inc, incEq, ranges, rangesN string) { return none, none, none, none }

// Categories of a changed input. An input can have several; "fields" counts only when the tree is unchanged.
const (
	CatTree    = "tree"         // dump hash, node count or root error flag changed (or a parse error appeared or went away)
	CatFields  = "fields"       // same tree, Fields() hash changed
	CatInc     = "incremental"  // edit inputs: the incremental reparse dump changed
	CatRanges  = "ranges"       // edit inputs: ChangedRanges changed
	CatIncFlip = "inc-vs-fresh" // edit inputs: incremental == fresh flipped
)

// Diff is one input that changed in at least one category.
type Diff struct {
	Lang, ID, Kind string
	Cats           []string
	IncFlip        string // "equal->differs" or "differs->equal" when CatIncFlip is set
}

// Group counts one language and kind (corpus, edit, large).
type Group struct {
	Lang, Kind                                 string
	Inputs, Tree, Fields, Inc, Ranges, IncFlip int
	BaseIncDiffers, CandIncDiffers             int // edit: incremental != fresh in each table
}

// Result is the classified comparison. Problems (an input missing on one side, or its source hash differing) make the
// comparison invalid for those inputs; the caller should fail on them.
type Result struct {
	Groups   []Group
	Diffs    []Diff
	Problems []string
}

type key struct{ lang, id string }

func index(side string, rows []Row, problems *[]string) map[key]Row {
	m := make(map[key]Row, len(rows))
	for _, r := range rows {
		k := key{r.Lang, r.ID}
		if _, dup := m[k]; dup {
			*problems = append(*problems, fmt.Sprintf("%s: duplicate input %s %s", side, r.Lang, r.ID))
		}
		m[k] = r
	}
	return m
}

// Compare classifies every input of base against cand.
func Compare(base, cand []Row) Result {
	var res Result
	b, c := index("base", base, &res.Problems), index("candidate", cand, &res.Problems)
	groups := map[[2]string]*Group{}
	keys := make([]key, 0, len(b))
	for k := range b {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].lang != keys[j].lang {
			return keys[i].lang < keys[j].lang
		}
		return keys[i].id < keys[j].id
	})
	for _, k := range keys {
		x := b[k]
		y, ok := c[k]
		if !ok {
			res.Problems = append(res.Problems, fmt.Sprintf("input only in base: %s %s", k.lang, k.id))
			continue
		}
		if x.Src != y.Src || x.Kind != y.Kind {
			res.Problems = append(res.Problems, fmt.Sprintf("input differs between the tables (source hash or kind): %s %s", k.lang, k.id))
			continue
		}
		g := groups[[2]string{x.Lang, x.Kind}]
		if g == nil {
			g = &Group{Lang: x.Lang, Kind: x.Kind}
			groups[[2]string{x.Lang, x.Kind}] = g
		}
		g.Inputs++
		d := Diff{Lang: x.Lang, ID: x.ID, Kind: x.Kind}
		treeChanged := x.Tree != y.Tree || x.Nodes != y.Nodes || x.Err != y.Err
		if treeChanged {
			d.Cats, g.Tree = append(d.Cats, CatTree), g.Tree+1
		} else if x.Fields != y.Fields {
			d.Cats, g.Fields = append(d.Cats, CatFields), g.Fields+1
		}
		if x.Kind == "edit" {
			if x.Inc != y.Inc {
				d.Cats, g.Inc = append(d.Cats, CatInc), g.Inc+1
			}
			if x.Ranges != y.Ranges {
				d.Cats, g.Ranges = append(d.Cats, CatRanges), g.Ranges+1
			}
			if x.IncEq == "false" {
				g.BaseIncDiffers++
			}
			if y.IncEq == "false" {
				g.CandIncDiffers++
			}
			if x.IncEq != y.IncEq {
				d.IncFlip = "equal->differs"
				if x.IncEq == "false" {
					d.IncFlip = "differs->equal"
				}
				d.Cats, g.IncFlip = append(d.Cats, CatIncFlip), g.IncFlip+1
			}
		}
		if len(d.Cats) > 0 {
			res.Diffs = append(res.Diffs, d)
		}
	}
	for k := range c {
		if _, ok := b[k]; !ok {
			res.Problems = append(res.Problems, fmt.Sprintf("input only in candidate: %s %s", k.lang, k.id))
		}
	}
	for _, g := range groups {
		res.Groups = append(res.Groups, *g)
	}
	sort.Slice(res.Groups, func(i, j int) bool {
		if res.Groups[i].Lang != res.Groups[j].Lang {
			return res.Groups[i].Lang < res.Groups[j].Lang
		}
		return res.Groups[i].Kind < res.Groups[j].Kind
	})
	sort.Strings(res.Problems)
	return res
}

// WriteDiffs writes one line per changed input: lang, id, kind, categories, incremental flip.
func (r Result) WriteDiffs(w io.Writer) error {
	for _, d := range r.Diffs {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", d.Lang, d.ID, d.Kind, strings.Join(d.Cats, ","), d.IncFlip); err != nil {
			return err
		}
	}
	return nil
}

// Summary renders the markdown report. title is free text (the rehearsal identity); idCap limits how many input IDs are
// listed per category (all of them are in the diff table).
func (r Result) Summary(title string, idCap int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Runtime-only rehearsal\n\n%s\n\n", title)
	b.WriteString("Local measurement of base (committed modules) against candidate (modules generated against another runtime), on the same inputs. Not parity evidence.\n\n")
	if len(r.Problems) > 0 {
		fmt.Fprintf(&b, "## INVALID: %d problems\n\n", len(r.Problems))
		for _, p := range r.Problems {
			fmt.Fprintf(&b, "- %s\n", p)
		}
		b.WriteString("\n")
	}
	b.WriteString("| lang | kind | inputs | tree changed | same tree, fields changed | incremental changed | ChangedRanges changed | inc vs fresh flipped | inc!=fresh base | inc!=fresh candidate |\n|---|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, g := range r.Groups {
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %d | %d | %d | %d | %d |\n", g.Lang, g.Kind, g.Inputs, g.Tree, g.Fields, g.Inc, g.Ranges, g.IncFlip, g.BaseIncDiffers, g.CandIncDiffers)
	}
	for _, cat := range []string{CatTree, CatFields, CatInc, CatRanges, CatIncFlip} {
		var ids []string
		for _, d := range r.Diffs {
			for _, c := range d.Cats {
				if c == cat {
					ids = append(ids, d.Lang+" "+d.ID)
				}
			}
		}
		if len(ids) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## %s: %d inputs\n\n", cat, len(ids))
		for i, id := range ids {
			if i == idCap {
				fmt.Fprintf(&b, "- ... %d more in diff.tsv\n", len(ids)-idCap)
				break
			}
			fmt.Fprintf(&b, "- %s\n", id)
		}
	}
	return b.String()
}
