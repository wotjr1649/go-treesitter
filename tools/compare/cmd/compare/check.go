package main

import (
	"bytes"
	"context"
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
	"time"

	"github.com/wotjr1649/go-treesitter/tools/bench/proc"
	"github.com/wotjr1649/go-treesitter/tools/compare/harness"
)

// Native reference protocol (tools/compare/native/sexp_ref.c):
//
//	<ref> sexp FILE                          line 1 "#compare-sexp/1 has_error=<0|1>", then ts_node_string of a fresh parse
//	<ref> edit-sexp OLD NEW START OE NE      the same for NEW reparsed incrementally from the edited OLD tree
const refHeader = "#compare-sexp/1 has_error="

type checkRow struct {
	Lang, Adapter, Input, Case, EditKind string
	RefKind                              string // own | diagnostic-product | unavailable
	Status                               string // compared | unsupported | error | ref-unavailable
	Level, FirstDiff                     string
	Missing, Extra                       int
	CandErr, RefErr, Clean               string
	CandSHA, RefSHA                      string
	Compat                               string // "rejects a fixture the product reference accepts cleanly"
	Reason                               string
}

type checker struct {
	o       checkOpts
	w       Workload
	mu      sync.Mutex
	rows    []checkRow
	scratch string
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
	fs.BoolVar(&o.smoke, "smoke", false, "two small inputs per language")
	fs.IntVar(&o.jobs, "j", 3, "parallel checks (outside any timing window)")
	fs.DurationVar(&o.perOp, "op-watchdog", 60*time.Second, "watchdog per process")
	fs.Parse(args)
	o.langs, o.adapters = strings.Split(langs, ","), strings.Split(adapters, ",")
	if o.workload == "" || o.bin == "" || o.refs == "" || o.out == "" {
		return errors.New("check needs -workload, -bin, -refs and -out")
	}
	c := &checker{o: o}
	if err := readJSON(filepath.Join(o.workload, "workload.json"), &c.w); err != nil {
		return err
	}
	var err error
	if c.scratch, err = os.MkdirTemp("", "compare-check-"); err != nil {
		return err
	}
	defer os.RemoveAll(c.scratch)
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(1, o.jobs))
	for _, lang := range o.langs {
		n := 0
		for _, in := range c.w.Inputs {
			if in.Lang != lang || in.Group != "small" || o.smoke && n >= 2 {
				continue
			}
			n++
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
	return c.write()
}

// ref picks the native reference of an adapter: its own, else the product's as a labelled diagnostic.
func (c *checker) ref(ad, lang string) (path, kind, reason string) {
	own := filepath.Join(c.o.refs, ad, lang)
	if p := regular(own); p != "" {
		return p, "own", ""
	}
	if b, err := os.ReadFile(own + ".unavailable"); err == nil {
		reason = strings.TrimSpace(string(b))
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

func (c *checker) run(bin string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.o.perOp)
	defer cancel()
	cmd := proc.CommandContext(ctx, bin, args...)
	cmd.Env = []string{"GOMAXPROCS=1", "GOGC=100", "GOENV=off", "LC_ALL=C"}
	out, errb := &limited{max: 256 << 20}, &limited{max: 4 << 10}
	cmd.Stdout, cmd.Stderr = out, errb
	err := cmd.Run()
	if err != nil {
		return out.buf.Bytes(), fmt.Errorf("%v: %s", err, strings.TrimSpace(errb.buf.String()))
	}
	return out.buf.Bytes(), nil
}

func (c *checker) input(in Input, ad string) {
	refPath, refKind, refReason := c.ref(ad, in.Lang)
	abs := func(p string) string { return filepath.Join(c.o.workload, filepath.FromSlash(p)) }
	live := has(declared[ad], "parse")
	sexpOp := "sexp"
	if !live {
		sexpOp = "ssexp"
	}
	type tc struct {
		name, kind string
		old, nw    string
		e          *EditSpec
	}
	cases := []tc{{"fresh", "", abs(in.Path), "", nil}}
	for i := range in.Edits {
		e := &in.Edits[i]
		cases = append(cases, tc{"fresh-new", e.Kind, abs(e.NewPath), "", e}, tc{"incremental", e.Kind, abs(in.Path), abs(e.NewPath), e})
	}
	for _, k := range cases {
		row := checkRow{Lang: in.Lang, Adapter: ad, Input: in.ID, Case: k.name, EditKind: k.kind, RefKind: refKind, Reason: refReason}
		if in.Clean != nil {
			row.Clean = strconv.FormatBool(*in.Clean)
		}
		tmp, _ := os.CreateTemp(c.scratch, "cand-*")
		tmp.Close()
		op := sexpOp
		wargs := []string{"-lang", in.Lang, "-mode", "out", "-src", k.old, "-out", tmp.Name()}
		if k.name == "incremental" {
			op = "esexp"
			wargs = append(wargs, "-new", k.nw, "-edit", fmt.Sprintf("%d,%d,%d", k.e.Start, k.e.OldEnd, k.e.NewEnd))
		}
		wargs = append(wargs, "-op", op)
		out, err := c.run(filepath.Join(c.o.bin, ad), wargs...)
		var res harness.Result
		if err == nil || len(out) > 0 {
			res = parseResult(out)
		}
		switch {
		case res.Status == "unsupported":
			row.Status, row.Reason = "unsupported", res.Error
		case res.Status != "ok":
			row.Status, row.Reason = "error", firstNonEmptyStr(res.Error, errString(err))
		case refKind == "unavailable":
			row.Status = "ref-unavailable"
		default:
			cand, _ := os.ReadFile(tmp.Name())
			var refOut []byte
			if k.name == "incremental" {
				refOut, err = c.run(refPath, "edit-sexp", k.old, k.nw, strconv.Itoa(int(k.e.Start)), strconv.Itoa(int(k.e.OldEnd)), strconv.Itoa(int(k.e.NewEnd)))
			} else {
				refOut, err = c.run(refPath, "sexp", k.old)
			}
			head, body, _ := bytes.Cut(refOut, []byte("\n"))
			if err != nil || !bytes.HasPrefix(head, []byte(refHeader)) {
				row.Status, row.Reason = "error", "native reference failed: "+errString(err)
				break
			}
			d := harness.Compare(string(body), string(cand))
			row.Status, row.Level, row.FirstDiff, row.Missing, row.Extra = "compared", d.Level, d.FirstDiff, d.Missing, d.Extra
			row.CandErr, row.RefErr = strconv.FormatBool(res.HasError), strconv.FormatBool(strings.TrimPrefix(string(head), refHeader) == "1")
			row.CandSHA, row.RefSHA = sha(cand), sha(body)
			if in.Clean != nil && *in.Clean && res.HasError && ad != refAdapter {
				row.Compat = "rejects-fixture-accepted-cleanly-by-product-reference"
			}
		}
		os.Remove(tmp.Name())
		c.mu.Lock()
		c.rows = append(c.rows, row)
		c.mu.Unlock()
	}
}

func parseResult(out []byte) harness.Result {
	var r harness.Result
	line, _, _ := bytes.Cut(out, []byte("\n"))
	if err := json.Unmarshal(line, &r); err != nil || r.Schema != harness.Schema {
		return harness.Result{Status: "error", Error: "no result line"}
	}
	return r
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	return s[:min(len(s), 300)]
}

func firstNonEmptyStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (c *checker) write() error {
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
	w.Write([]string{"lang", "adapter", "input", "case", "edit_kind", "ref_kind", "status", "level", "first_diff", "missing", "extra", "cand_has_error", "ref_has_error", "product_ref_clean", "cand_sha256", "ref_sha256", "compat", "reason"})
	for _, r := range c.rows {
		w.Write([]string{r.Lang, r.Adapter, r.Input, r.Case, r.EditKind, r.RefKind, r.Status, r.Level, r.FirstDiff, strconv.Itoa(r.Missing), strconv.Itoa(r.Extra), r.CandErr, r.RefErr, r.Clean, r.CandSHA, r.RefSHA, r.Compat, r.Reason})
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
