package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/wotjr1649/go-treesitter/tools/compare/harness"
)

const (
	maxFile  = 20 << 20 // one accepted file
	maxTotal = 25 << 20 // the whole accepted set, and (seal) the whole uploaded set
	maxText  = 8 << 10  // failures/ and provenance/ text files
	maxHash  = 64 << 20 // bytes read to fingerprint an oversize file (a bounded read, never the whole file)
)

// allowed lists the file names a sandbox may leave in its result volume. Anything else (links, binaries, module
// caches, environment dumps, unexpected paths) is rejected, not copied.
var allowed = regexp.MustCompile("^(measurements\\.jsonl|cells\\.json|run\\.json|correctness\\.csv|builds\\.csv|workload\\.json|fixtures-(time|alloc)\\.csv|summary\\.md|provenance/[A-Za-z0-9._-]{1,80}\\.(json|txt|csv)|failures/[A-Za-z0-9._-]{1,80}\\.txt|go-sum-proposal/[a-z]{3,20}/go\\.(mod|sum))$")

var (
	knownPass   = map[string]bool{"preflight": true, "time": true, "alloc": true, "rss": true, "ctl": true}
	knownOp     = map[string]bool{"parse": true, "snapshot": true, "edit": true, "ctl": true}
	knownGroup  = map[string]bool{"": true, "small": true, "large": true}
	knownLang   = map[string]bool{}
	knownAdapt  = map[string]bool{}
	knownStates = map[string]bool{"planned": true, "measured": true, "partial": true, "failed": true, "incomplete": true, "unsupported": true, "unavailable": true}
)

func init() {
	for _, l := range allLangs {
		knownLang[l.name] = true
	}
	for _, a := range adapterOrder {
		knownAdapt[a] = true
	}
}

// expectation is what the measurer's own plan says may appear in the measurement rows.
type expectation struct {
	cells  map[string]Cell
	inputs map[string]bool
}

func loadExpectation(in string) (*expectation, error) {
	var cells []Cell
	if err := readJSON(filepath.Join(in, "cells.json"), &cells); err != nil {
		return nil, fmt.Errorf("cells.json: %w", err)
	}
	var w Workload
	if err := readJSON(filepath.Join(in, "workload.json"), &w); err != nil {
		return nil, fmt.Errorf("workload.json: %w", err)
	}
	e := &expectation{cells: map[string]Cell{}, inputs: map[string]bool{}}
	for _, c := range cells {
		if !knownStates[c.State] || !knownLang[c.Lang] || !knownAdapt[c.Cand] || !knownOp[c.Op] || !knownGroup[c.Group] {
			return nil, fmt.Errorf("cells.json: unexpected cell %q", c.Key)
		}
		e.cells[c.Key] = c
	}
	for _, in := range w.Inputs {
		e.inputs[in.ID] = true
	}
	return e, nil
}

func cmdCollect(args []string) error {
	fs := flag.NewFlagSet("collect", flag.ExitOnError)
	in := fs.String("in", "", "untrusted result directory")
	out := fs.String("out", "", "directory for the accepted files")
	fs.Parse(args)
	if *in == "" || *out == "" {
		return fmt.Errorf("collect needs -in and -out")
	}
	var accepted, rejected, oversize []string
	var total int64
	err := filepath.WalkDir(*in, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(*in, p)
		rel = filepath.ToSlash(rel)
		if rel == "." || d.IsDir() && d.Type()&os.ModeSymlink == 0 {
			return nil
		}
		info, err := os.Lstat(p)
		switch {
		case err != nil:
			return err
		case !info.Mode().IsRegular():
			rejected = append(rejected, rel+": not a regular file")
		case !allowed.MatchString(rel):
			rejected = append(rejected, rel+": unexpected path")
		case info.Size() > maxFile || total+info.Size() > maxTotal:
			// Fingerprint a bounded prefix: the status stays reproducible and the read stays bounded.
			oversize = append(oversize, fmt.Sprintf("%s %d prefix-sha256(%d)=%s", rel, info.Size(), maxHash, prefixHash(p)))
		default:
			total += info.Size()
			accepted = append(accepted, rel)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(accepted)
	var exp *expectation
	if has(accepted, "measurements.jsonl") {
		if exp, err = loadExpectation(*in); err != nil {
			rejected = append(rejected, err.Error())
		}
	}
	var kept []string
	for _, rel := range accepted {
		if verr := validate(filepath.Join(*in, filepath.FromSlash(rel)), rel, exp); verr != nil {
			rejected = append(rejected, rel+": "+verr.Error())
			continue
		}
		kept = append(kept, rel)
	}
	for _, rel := range kept {
		b, err := readBounded(filepath.Join(*in, filepath.FromSlash(rel)), maxFile)
		if err != nil {
			return err
		}
		if err := writeFileAll(filepath.Join(*out, filepath.FromSlash(rel)), b); err != nil {
			return err
		}
	}
	if err := writeSums(*out); err != nil {
		return err
	}
	if len(oversize) > 0 {
		if err := writeFileAll(filepath.Join(*out, "OVERSIZE.txt"), []byte("files over the upload budget (name size bounded-prefix-hash); the run stays incomplete:\n"+strings.Join(oversize, "\n")+"\n")); err != nil {
			return err
		}
	}
	if len(rejected) > 0 || len(oversize) > 0 {
		sort.Strings(rejected)
		_ = writeFileAll(filepath.Join(*out, "REJECTED.txt"), []byte(printable(strings.Join(rejected, "\n"), 4000)+"\n"))
		return fmt.Errorf("%d rejected, %d oversize: %s", len(rejected), len(oversize), printable(strings.Join(append(rejected, oversize...), "; "), 1500))
	}
	return nil
}

func prefixHash(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return "unreadable"
	}
	defer f.Close()
	h := sha256.New()
	io.Copy(h, io.LimitReader(f, maxHash))
	return hex.EncodeToString(h.Sum(nil))
}

// writeSums writes SHA256SUMS over every regular file under dir (except itself) and refuses links and special files.
func writeSums(dir string) error {
	var lines []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if rel == "." || rel == "SHA256SUMS" || d.IsDir() && d.Type()&os.ModeSymlink == 0 {
			return nil
		}
		st, err := os.Lstat(p)
		if err != nil || !st.Mode().IsRegular() {
			return fmt.Errorf("%s: not a regular file", rel)
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		lines = append(lines, hex.EncodeToString(h.Sum(nil))+"  "+rel)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i][66:] < lines[j][66:] })
	return writeFileAll(filepath.Join(dir, "SHA256SUMS"), []byte(strings.Join(lines, "\n")+"\n"))
}

// cmdSeal is the last step before upload: the final file set (collected files plus the report) gets its total size
// checked against the cap and a fresh SHA256SUMS.
func cmdSeal(args []string) error {
	fs := flag.NewFlagSet("seal", flag.ExitOnError)
	dir := fs.String("dir", "", "the artifact directory")
	fs.Parse(args)
	if *dir == "" {
		return errors.New("seal needs -dir")
	}
	var total int64
	err := filepath.WalkDir(*dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if st, serr := os.Lstat(p); serr == nil && st.Mode().IsRegular() {
			total += st.Size()
		} else if serr != nil || !d.IsDir() {
			return fmt.Errorf("%s: not a regular file or directory", p)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if total > maxTotal {
		return fmt.Errorf("artifact set is %d bytes, over the %d byte cap", total, maxTotal)
	}
	return writeSums(*dir)
}

// visible reports whether b holds only printable ASCII plus line breaks and tabs.
func visible(b []byte) bool {
	for _, c := range b {
		if c >= 0x20 && c < 0x7f || c == '\n' || c == '\r' || c == '\t' {
			continue
		}
		return false
	}
	return true
}

// validate checks a file's type, fields and sizes. A measurement row must belong to a cell of the measurer's own
// plan; text a candidate could influence must be printable ASCII.
func validate(p, rel string, exp *expectation) error {
	switch {
	case rel == "measurements.jsonl":
		if exp == nil {
			return errors.New("no cells.json/workload.json to check the rows against")
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		sc := bufio.NewScanner(io.LimitReader(f, maxFile))
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		per := map[string]int{}
		for n := 1; sc.Scan(); n++ {
			if !visible(sc.Bytes()) {
				return fmt.Errorf("line %d: non-printable bytes", n)
			}
			var r Row
			dec := json.NewDecoder(strings.NewReader(sc.Text()))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&r); err != nil {
				return fmt.Errorf("line %d: %v", n, err)
			}
			if err := validateRow(r, exp); err != nil {
				return fmt.Errorf("line %d: %v", n, err)
			}
			k := fmt.Sprintf("%s|%s|%s|%d", r.Cell, r.Pass, r.Role, r.Round)
			if per[k]++; per[k] > 1 && r.Pass != "preflight" {
				return fmt.Errorf("line %d: two rows for %s", n, k)
			}
		}
		return sc.Err()
	case strings.HasPrefix(rel, "failures/") || strings.HasPrefix(rel, "provenance/") && !strings.HasSuffix(rel, ".json") && !strings.HasSuffix(rel, ".csv"):
		b, err := readBounded(p, maxFile)
		if err != nil {
			return err
		}
		if len(b) > maxText || !visible(b) {
			return errors.New("text file over 8 KiB or with non-printable bytes")
		}
	case strings.HasPrefix(rel, "go-sum-proposal/"):
		st, _ := os.Stat(p)
		if st.Size() > 1<<20 {
			return errors.New("module file over 1 MiB")
		}
	case strings.HasSuffix(rel, ".json"):
		b, err := readBounded(p, maxFile)
		if err != nil {
			return err
		}
		if !visible(b) {
			return errors.New("non-printable bytes")
		}
		var v any
		return json.Unmarshal(b, &v)
	case strings.HasSuffix(rel, ".csv"):
		b, err := readBounded(p, maxFile)
		if err != nil {
			return err
		}
		if !visible(b) {
			return errors.New("non-printable bytes")
		}
		recs, err := csv.NewReader(strings.NewReader(string(b))).ReadAll()
		if err != nil {
			return err
		}
		return checkCSVShape(rel, recs)
	}
	return nil
}

// checkCSVShape pins the column count and the value sets the report relies on.
func checkCSVShape(rel string, recs [][]string) error {
	if len(recs) == 0 {
		return errors.New("empty csv")
	}
	for _, r := range recs {
		if len(r) != len(recs[0]) {
			return errors.New("ragged csv")
		}
		for _, f := range r {
			if len(f) > 1500 {
				return errors.New("field over 1500 bytes")
			}
		}
	}
	idx := func(name string) int {
		for i, h := range recs[0] {
			if h == name {
				return i
			}
		}
		return -1
	}
	switch rel {
	case "correctness.csv":
		for _, c := range []struct {
			col string
			ok  func(string) bool
		}{
			{"adapter", func(s string) bool { return knownAdapt[s] }},
			{"lang", func(s string) bool { return knownLang[s] }},
			{"status", func(s string) bool {
				return s == "compared" || s == "unsupported" || s == "error" || s == "ref-unavailable"
			}},
			{"level", func(s string) bool {
				switch s {
				case "", harness.LevelExact, harness.LevelTree, harness.LevelLossy, harness.LevelShape, harness.LevelMismatch, harness.LevelUnparsed:
					return true
				}
				return false
			}},
		} {
			i := idx(c.col)
			if i < 0 {
				return fmt.Errorf("column %s missing", c.col)
			}
			for _, r := range recs[1:] {
				if !c.ok(r[i]) {
					return fmt.Errorf("column %s: unexpected value", c.col)
				}
			}
		}
	case "builds.csv":
		if i := idx("adapter"); i < 0 {
			return errors.New("column adapter missing")
		} else {
			for _, r := range recs[1:] {
				if !knownAdapt[r[i]] && r[i] != "empty" {
					return errors.New("column adapter: unexpected value")
				}
			}
		}
	}
	return nil
}

func validateRow(r Row, exp *expectation) error {
	if !knownPass[r.Pass] || !knownAdapt[r.Adapter] || !knownOp[r.Op] || !knownGroup[r.Group] || !knownLang[r.Lang] ||
		(r.Role != "product" && r.Role != "candidate") || r.Round < 0 || r.Round > 99 || len(r.Err) > 2048 {
		return errors.New("field outside its allowed set")
	}
	if r.Pass == "preflight" {
		if r.Cell != r.Adapter+"/"+r.Lang {
			return errors.New("preflight row for an unexpected cell")
		}
	} else if c, ok := exp.cells[r.Cell]; !ok || c.Lang != r.Lang || c.Op != r.Op || c.Group != r.Group || c.Input != r.Input ||
		(r.Adapter != c.Cand && r.Adapter != refAdapter) {
		return errors.New("row does not belong to an expected cell")
	}
	if r.Input != "" && !exp.inputs[r.Input] {
		return errors.New("unknown input")
	}
	if res := r.Res; res != nil {
		if res.Schema != harness.Schema || len(res.SamplesNs) > 1000 || len(res.Error) > 1024 || len(res.Identity) > 64 || res.Reps > 1000 {
			return errors.New("result out of bounds")
		}
		switch res.Status {
		case "ok", "unsupported", "error", "timeout":
		default:
			return errors.New("unknown result status")
		}
	}
	return nil
}

// cmdGateNocc fails when the product's compiler-free build or smoke parse did not succeed, and checks the file against
// the exact expected adapter set, one row each, with consistent status, exit and outcome values.
func cmdGateNocc(args []string) error {
	fs := flag.NewFlagSet("gate-nocc", flag.ExitOnError)
	file := fs.String("builds", "", "builds.csv of the no-compiler job")
	fs.Parse(args)
	b, err := readBounded(*file, maxFile)
	if err != nil {
		return err
	}
	recs, err := csv.NewReader(strings.NewReader(string(b))).ReadAll()
	if err != nil || len(recs) < 2 {
		return fmt.Errorf("builds.csv unreadable: %v", err)
	}
	col := map[string]int{}
	for i, h := range recs[0] {
		col[h] = i
	}
	for _, need := range []string{"adapter", "status", "exit", "bytes", "smoke_parse", "outcome"} {
		if _, ok := col[need]; !ok {
			return fmt.Errorf("column %s missing", need)
		}
	}
	seen := map[string]bool{}
	for _, r := range recs[1:] {
		if len(r) != len(recs[0]) {
			return errors.New("ragged builds.csv")
		}
		a := r[col["adapter"]]
		if seen[a] || !has(noccAdapters, a) {
			return fmt.Errorf("unexpected or repeated adapter %q", a)
		}
		seen[a] = true
		st, exit, out := r[col["status"]], r[col["exit"]], r[col["outcome"]]
		switch out {
		case "ok", "expected-unsupported", "build-failure", "smoke-failure":
		default:
			return fmt.Errorf("%s: unknown outcome %q", a, out)
		}
		if (st == "ok") != (exit == "0") || (st != "ok" && st != "failed") {
			return fmt.Errorf("%s: status %q and exit %q disagree", a, st, exit)
		}
		if out == "ok" && (st != "ok" || r[col["bytes"]] == "0" || a != "empty" && r[col["smoke_parse"]] != "ok") {
			return fmt.Errorf("%s: outcome ok without a successful build and smoke parse", a)
		}
		if out == "expected-unsupported" && a != "official" && a != "smacker" {
			return fmt.Errorf("%s: only the cgo adapters may be expected-unsupported", a)
		}
		if a == refAdapter && out != "ok" {
			return fmt.Errorf("the product's compiler-free build or smoke parse failed: outcome=%s smoke=%s", out, r[col["smoke_parse"]])
		}
	}
	for _, a := range noccAdapters {
		if !seen[a] {
			return fmt.Errorf("no row for %s", a)
		}
	}
	return nil
}
