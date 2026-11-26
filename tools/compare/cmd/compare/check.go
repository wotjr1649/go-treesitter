package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wotjr1649/go-treesitter/tools/compare/harness"
)

// Native reference protocol (tools/compare/native/sexp_ref.c):
//
//	<ref> sexp FILE                          line 1 "#compare-sexp/1 has_error=<0|1>", then ts_node_string of a fresh parse
//	<ref> edit-sexp OLD NEW START OE NE      the same for NEW reparsed incrementally from the edited OLD tree
const refHeader = "#compare-sexp/1 has_error="

const maxSexp = 600 << 20 // bytes of S-expression the measurer reads from one process

type checkRow struct {
	Lang, Adapter, Input, Group, Case, EditKind string
	RefKind                                     string // own | diagnostic-product | unavailable
	Status                                      string // compared | unsupported | error | ref-unavailable
	Level, FirstDiff                            string
	Missing, Extra                              int
	CandErr, RefErr                             string
	CandSHA, RefSHA                             string
	Compat                                      string // "rejects-edited-input-the-product-reference-accepts-cleanly" and similar
	Reason                                      string
}

type checker struct {
	o    checkOpts
	w    Workload
	sb   *sandbox
	mu   sync.Mutex
	rows []checkRow
	seq  atomic.Int64
}

type checkOpts struct {
	workload, bin, refs, out string
	langs, adapters          []string
	smoke                    bool
	jobs                     int
	perOp                    time.Duration
}

func cmdCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	var o checkOpts
	var langs, adapters string
	fs.StringVar(&o.workload, "workload", "", "workload directory")
	fs.StringVar(&o.bin, "bin", "", "directory of worker binaries")
	fs.StringVar(&o.refs, "refs", "", "directory of native references: <adapter>/<lang> executables or <adapter>/<lang>.unavailable reasons")
	fs.StringVar(&o.out, "out", "", "result directory (correctness.csv is written here)")
	fs.StringVar(&langs, "langs", "c,go,javascript,json,python", "languages")
	fs.StringVar(&adapters, "adapters", strings.Join(adapterOrder, ","), "adapters")
	fs.BoolVar(&o.smoke, "smoke", false, "two small inputs and the smallest large input per language")
	fs.IntVar(&o.jobs, "j", 3, "parallel checks (outside any timing window)")
	fs.DurationVar(&o.perOp, "op-watchdog", 60*time.Second, "watchdog per process (three times as long for a large input)")
	fs.Parse(args)
	o.langs, o.adapters = strings.Split(langs, ","), strings.Split(adapters, ",")
	if o.workload == "" || o.bin == "" || o.refs == "" || o.out == "" {
		return errors.New("check needs -workload, -bin, -refs and -out")
	}
	c := &checker{o: o, sb: &sandbox{}}
	if err := readJSON(filepath.Join(o.workload, "workload.json"), &c.w); err != nil {
		return err
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(1, o.jobs))
	for _, lang := range o.langs {
		small, large := c.w.inputsFor(lang, o.smoke)
		for _, in := range append(small, large...) {
			for _, ad := range o.adapters {
				wg.Add(1)
				sem <- struct{}{}
				go func(in Input, ad string) {
					defer wg.Done()
					defer func() { <-sem }()
					c.input(in, ad)
				}(in, ad)
			}
		}
	}
	wg.Wait()
	c.sb.close()
	return c.write()
}

// ref picks the native reference of an adapter: its own, else the product's as a labelled diagnostic.
func (c *checker) ref(ad, lang string) (path, kind, reason string) {
	own := filepath.Join(c.o.refs, ad, lang)
	if p := regular(own); p != "" {
		return p, "own", ""
	}
	if b, err := os.ReadFile(own + ".unavailable"); err == nil {
		reason = printable(strings.TrimSpace(string(b)), 300)
	} else {
		reason = "no native reference was built for this adapter and language"
	}
	if ad != refAdapter {
		if p := regular(filepath.Join(c.o.refs, refAdapter, lang)); p != "" {
			return p, "diagnostic-product", reason
		}
	}
	return "", "unavailable", reason
}

func (c *checker) input(in Input, ad string) {
	refPath, refKind, refReason := c.ref(ad, in.Lang)
	abs := func(p string) string { return filepath.Join(c.o.workload, filepath.FromSlash(p)) }
	sexpOp := "sexp"
	if !has(declared[ad], "parse") {
		sexpOp = "ssexp"
	}
	timeout := c.o.perOp
	if in.Group == "large" {
		timeout *= 3
	}
	type tc struct {
		name, kind string
		old, nw    string
		e          *EditSpec
	}
	cases := []tc{{"fresh", "", abs(in.Path), "", nil}}
	if in.Group == "small" { // edits are checked on the small inputs; a large input is graded on its fresh parse
		for i := range in.Edits {
			e := &in.Edits[i]
			cases = append(cases, tc{"fresh-new", e.Kind, abs(e.NewPath), "", e}, tc{"incremental", e.Kind, abs(in.Path), abs(e.NewPath), e})
		}
	}
	for _, k := range cases {
		row := checkRow{Lang: in.Lang, Adapter: ad, Input: in.ID, Group: in.Group, Case: k.name, EditKind: k.kind, RefKind: refKind, Reason: refReason}
		c.grade(&row, in, ad, refPath, k.name, k.old, k.nw, k.e, sexpOp, timeout)
		c.mu.Lock()
		c.rows = append(c.rows, row)
		c.mu.Unlock()
	}
}

// grade runs the candidate and its reference on one case and fills row. Only a candidate that exited normally, said
// "ok" and left an output file that matches its own reported length and hash is graded; anything else is a recorded error.
func (c *checker) grade(row *checkRow, in Input, ad, refPath, name, old, nw string, e *EditSpec, sexpOp string, timeout time.Duration) {
	dir, err := c.sb.dir(ad)
	if err != nil {
		row.Status, row.Reason = "error", printable(err.Error(), 200)
		return
	}
	outPath := filepath.Join(dir, fmt.Sprintf("out-%d", c.seq.Add(1)))
	defer os.Remove(outPath)
	op := sexpOp
	wargs := []string{"-lang", in.Lang, "-mode", "out", "-src", old, "-out", outPath}
	if name == "incremental" {
		op = "esexp"
		wargs = append(wargs, "-new", nw, "-edit", fmt.Sprintf("%d,%d,%d", e.Start, e.OldEnd, e.NewEnd))
	}
	wargs = append(wargs, "-op", op)
	sp := c.sb.spawn(ad, filepath.Join(c.o.bin, ad), wargs, timeout, 64<<10)
	var res harness.Result
	line, _, _ := bytes.Cut(sp.Stdout, []byte("\n"))
	if jsonErr := json.Unmarshal(line, &res); jsonErr != nil || res.Schema != harness.Schema || !boundResult(&res) {
		row.Status, row.Reason = "error", "no valid result line: "+printable(sp.Stderr+" "+errString(sp.Err), 300)
		return
	}
	// the result must describe this very call, and the process must have exited cleanly, before any status is believed
	if sp.Failed || sp.Truncated || sp.TimedOut || sp.Exit != 0 || sp.Err != nil {
		row.Status, row.Reason = "error", printable(fmt.Sprintf("status=%s exit=%d failed=%t timeout=%t %s", res.Status, sp.Exit, sp.Failed, sp.TimedOut, res.Error), 300)
		return
	}
	if res.Adapter != ad || res.Lang != in.Lang || res.Op != op || res.Mode != "out" {
		row.Status, row.Reason = "error", "the result line does not describe this call"
		return
	}
	switch {
	case res.Status == "unsupported":
		row.Status, row.Reason = "unsupported", res.Error
		return
	case res.Status != "ok":
		row.Status, row.Reason = "error", printable(fmt.Sprintf("status=%s %s", res.Status, res.Error), 300)
		return
	case row.RefKind == "unavailable":
		row.Status = "ref-unavailable"
		return
	}
	cand, err := readBounded(outPath, maxSexp)
	if err != nil || len(cand) != res.OutBytes || sha(cand) != res.OutSHA256 {
		row.Status, row.Reason = "error", "the output file does not match the worker's reported length and hash"
		return
	}
	rargs := []string{"sexp", old}
	if name == "incremental" {
		rargs = []string{"edit-sexp", old, nw, strconv.Itoa(int(e.Start)), strconv.Itoa(int(e.OldEnd)), strconv.Itoa(int(e.NewEnd))}
	}
	rp := c.sb.spawn(ad, refPath, rargs, timeout, maxSexp)
	head, body, _ := bytes.Cut(rp.Stdout, []byte("\n"))
	if rp.Failed || rp.Truncated || rp.TimedOut || rp.Exit != 0 || rp.Err != nil || (string(head) != refHeader+"0" && string(head) != refHeader+"1") {
		row.Status, row.Reason = "error", "native reference failed: "+printable(fmt.Sprintf("exit=%d timeout=%t %s %s", rp.Exit, rp.TimedOut, errString(rp.Err), rp.Stderr), 300)
		return
	}
	d := harness.Compare(string(body), string(cand))
	row.Status, row.Level, row.FirstDiff, row.Missing, row.Extra = "compared", d.Level, printable(d.FirstDiff, 300), d.Missing, d.Extra
	row.CandErr, row.RefErr = strconv.FormatBool(res.HasError), strconv.FormatBool(strings.TrimPrefix(string(head), refHeader) == "1")
	row.CandSHA, row.RefSHA = sha(cand), sha(body)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return printable(err.Error(), 300)
}

// write sorts the rows, derives the compatibility flag from the product reference for the same bytes and operation,
// and writes correctness.csv.
func (c *checker) write() error {
	key := func(r checkRow) string {
		return strings.Join([]string{r.Lang, r.Input, r.Case, r.EditKind}, "|")
	}
	prod := map[string]checkRow{}
	for _, r := range c.rows {
		if r.Adapter == refAdapter && r.Status == "compared" {
			prod[key(r)] = r
		}
	}
	for i := range c.rows {
		r := &c.rows[i]
		if r.Adapter == refAdapter || r.Status != "compared" || r.CandErr != "true" {
			continue
		}
		if p, ok := prod[key(*r)]; ok && p.RefKind == "own" && p.RefErr == "false" {
			r.Compat = "rejects-input-the-product-reference-accepts-cleanly"
		}
	}
	sort.Slice(c.rows, func(i, j int) bool {
		a, b := c.rows[i], c.rows[j]
		return strings.Join([]string{a.Lang, a.Adapter, a.Input, a.Case, a.EditKind}, "|") < strings.Join([]string{b.Lang, b.Adapter, b.Input, b.Case, b.EditKind}, "|")
	})
	if err := os.MkdirAll(c.o.out, 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(c.o.out, "correctness.csv"))
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Write([]string{"lang", "adapter", "input", "group", "case", "edit_kind", "ref_kind", "status", "level", "first_diff", "missing", "extra", "cand_has_error", "ref_has_error", "cand_sha256", "ref_sha256", "compat", "reason"})
	for _, r := range c.rows {
		w.Write([]string{r.Lang, r.Adapter, r.Input, r.Group, r.Case, r.EditKind, r.RefKind, r.Status, r.Level, r.FirstDiff, strconv.Itoa(r.Missing), strconv.Itoa(r.Extra), r.CandErr, r.RefErr, r.CandSHA, r.RefSHA, r.Compat, r.Reason})
	}
	w.Flush()
	return w.Error()
}

// regular returns p (or p+".exe" on Windows) when it names a regular file.
func regular(p string) string {
	for _, c := range []string{p, p + ".exe"} {
		if st, err := os.Stat(c); err == nil && st.Mode().IsRegular() {
			return c
		}
	}
	return ""
}
