package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/wotjr1649/go-treesitter/tools/compare/harness"
)

func readRows(p string) ([]Row, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var rows []Row
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r Row
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if r.Res != nil && r.Res.Schema != harness.Schema {
			return nil, fmt.Errorf("%s: result schema %d", p, r.Res.Schema)
		}
		rows = append(rows, r)
	}
	return rows, sc.Err()
}

func okRes(r Row) *harness.Result {
	if r.Res != nil && r.Res.Status == "ok" && r.Exit == 0 && !r.TimedOut && !r.Failed && (r.Pass != "time" || len(r.Res.SamplesNs) == r.Res.Reps) {
		return r.Res
	}
	return nil
}

// gradedSet maps adapter|input|case|edit-kind to the grade level of that compared case.
func gradedSet(p string) (map[string]string, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	recs, err := csv.NewReader(f).ReadAll()
	if err != nil || len(recs) == 0 {
		return nil, fmt.Errorf("%s: %v", p, err)
	}
	col := map[string]int{}
	for i, h := range recs[0] {
		col[h] = i
	}
	g := map[string]string{}
	for _, r := range recs[1:] {
		if r[col["status"]] == "compared" {
			g[r[col["adapter"]]+"|"+r[col["input"]]+"|"+r[col["case"]]+"|"+r[col["edit_kind"]]] = r[col["level"]]
		}
	}
	return g, nil
}

// gradedEqual reports whether the case the cell's operation exercised was compared with the reference and the tree
// is the same: only sexp-exact and tree-equal qualify. tree-lossy, shape-only, mismatch and unparsed cells are
// counted but never enter a ratio, and the timed edit's own incremental case grades an edit cell.
func gradedEqual(g map[string]string, adapter string, c Cell) bool {
	key := adapter + "|" + c.Input + "|fresh|"
	if c.Op == "edit" {
		key = adapter + "|" + c.Input + "|incremental|token"
	}
	lv := g[key]
	return lv == harness.LevelExact || lv == harness.LevelTree
}

// mdEscape makes text that came from a candidate or a file name inert in Markdown (links, mentions, HTML, tables).
func mdEscape(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '<', '>', '|', '`', '@', '[', ']', '!', '~', '*', '\\', '#':
			return '_'
		}
		return r
	}, s)
}

func ms(ns float64) float64 { return ns / 1e6 }

func samplesMedian(r *harness.Result) float64 {
	f := make([]float64, len(r.SamplesNs))
	for i, s := range r.SamplesNs {
		f[i] = float64(s)
	}
	return median(f)
}

// CellStat is one input's paired result for one pass.
type CellStat struct {
	Lang, Cand, Op, Group, Input string
	Rounds                       int
	ProdMs, CandMs               float64
	Ratio                        Ratio
	Metric2                      Ratio // alloc pass: bytes per op ratio (Ratio is mallocs per op)
	ProdAlloc, CandAlloc         [2]float64
}

// pairStats groups the rows of a pass by cell and round and forms the per-round candidate/product ratios.
func pairStats(rows []Row, cells map[string]Cell, pass string, reps map[string]int, include func(Cell) bool) []CellStat {
	type key struct {
		cell  string
		round int
	}
	prod, cand := map[key]*harness.Result{}, map[key]*harness.Result{}
	for _, r := range rows {
		if r.Pass != pass {
			continue
		}
		// a block counts only when it is complete for its own call (clean exit, scheduled sample count, matching adapter,
		// language, operation and mode) and has the scheduled number of repetitions
		if res := okRes(r); res != nil && complete(r, pass, reps[r.Group]) {
			if r.Role == "product" {
				prod[key{r.Cell, r.Round}] = res
			} else {
				cand[key{r.Cell, r.Round}] = res
			}
		}
	}
	byCell := map[string][]int{}
	for k := range prod {
		if _, ok := cand[k]; ok {
			byCell[k.cell] = append(byCell[k.cell], k.round)
		}
	}
	var out []CellStat
	for ck, rounds := range byCell {
		sort.Ints(rounds)
		c := cells[ck]
		// only complete, graded cells enter a summary: every scheduled pair is recounted from the rows themselves rather
		// than trusting the cell's state, and the grade must allow it; the raw rows stay in measurements.jsonl
		if !include(c) || len(rounds) != c.Want || c.Want == 0 || !exactRounds(rounds) {
			continue
		}
		s := CellStat{Lang: c.Lang, Cand: c.Cand, Op: c.Op, Group: c.Group, Input: c.Input, Rounds: len(rounds)}
		var ratios, bratios, pm, cm []float64
		var pa, ca, pb, cb []float64
		for _, rd := range rounds {
			p, q := prod[key{ck, rd}], cand[key{ck, rd}]
			if pass == "time" {
				a, b := samplesMedian(p), samplesMedian(q)
				pm, cm = append(pm, a), append(cm, b)
				ratios = append(ratios, b/a)
			} else if p.Reps > 0 && q.Reps > 0 && p.Mallocs > 0 && p.TotalAlloc > 0 {
				x, y := float64(p.Mallocs)/float64(p.Reps), float64(q.Mallocs)/float64(q.Reps)
				u, v := float64(p.TotalAlloc)/float64(p.Reps), float64(q.TotalAlloc)/float64(q.Reps)
				pa, ca, pb, cb = append(pa, x), append(ca, y), append(pb, u), append(cb, v)
				ratios, bratios = append(ratios, y/x), append(bratios, v/u)
			}
		}
		s.Ratio, s.Metric2 = summarize(ratios), summarize(bratios)
		s.ProdMs, s.CandMs = ms(median(pm)), ms(median(cm))
		s.ProdAlloc, s.CandAlloc = [2]float64{median(pa), median(pb)}, [2]float64{median(ca), median(cb)}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		return strings.Join([]string{a.Lang, a.Cand, a.Op, a.Group, a.Input}, "|") < strings.Join([]string{b.Lang, b.Cand, b.Op, b.Group, b.Input}, "|")
	})
	return out
}

func f3(x float64) string {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return "n/a"
	}
	return strconv.FormatFloat(x, 'f', 3, 64)
}

func cmdReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	in := fs.String("in", "", "result directory (run output plus correctness.csv, builds.csv)")
	out := fs.String("out", "", "report directory (default: -in)")
	fs.Parse(args)
	if *in == "" {
		return fmt.Errorf("report needs -in")
	}
	if *out == "" {
		*out = *in
	}
	rows, err := readRows(filepath.Join(*in, "measurements.jsonl"))
	if err != nil {
		return err
	}
	var cellList []Cell
	if err := readJSON(filepath.Join(*in, "cells.json"), &cellList); err != nil {
		return err
	}
	var info runInfo
	if err := readJSON(filepath.Join(*in, "run.json"), &info); err != nil {
		return err
	}
	cells := map[string]Cell{}
	for _, c := range cellList {
		cells[c.Key] = c
	}
	graded, gerr := gradedSet(filepath.Join(*in, "correctness.csv"))
	if gerr != nil {
		graded = map[string]string{}
	}
	include := func(c Cell) bool {
		return c.State == "measured" && gradedEqual(graded, refAdapter, c) && gradedEqual(graded, c.Cand, c)
	}
	repsOf := info.Reps
	timing, alloc := bothPasses(pairStats(rows, cells, "time", repsOf, include), pairStats(rows, cells, "alloc", repsOf, include))

	var md strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&md, f+"\n", a...) }
	w("# Comparative benchmark summary\n")
	w("Every number below describes the identified consumer artifacts on this runner VM. Timings are at most E4 (docs/specs/validation.md), and only a pair whose")
	w("grammar bytes, runtime and API work are controlled would qualify; the libraries here ship different ones, so the rows are descriptive observations. S-expression agreement is at most E2 and nothing here is E5.\n")
	w("Only cells that finished every scheduled round of both passes, exited cleanly and whose own operation (fresh parse, or the timed incremental edit) produced a tree equal to the native reference's (sexp-exact or tree-equal) on both sides enter the ratios below.")
	w("Failed, partial, incomplete, unsupported, unavailable and ungraded cells are counted in the coverage table; their raw rows stay in measurements.jsonl.\n")
	if gerr != nil {
		w("**correctness.csv is missing or unreadable (%s): no cell is graded, so no timing row is shown.**\n", mdEscape(gerr.Error()))
	}
	writeRunInfo(&md, info)

	w("\n## Cell coverage\n")
	w("| candidate | op | measured and graded | measured, not tree-equal | partial | failed | incomplete | unsupported | unavailable |")
	w("|---|---|---:|---:|---:|---:|---:|---:|---:|")
	type k2 struct{ cand, op string }
	cov := map[k2]map[string]int{}
	for _, c := range cellList {
		m := cov[k2{c.Cand, c.Op}]
		if m == nil {
			m = map[string]int{}
			cov[k2{c.Cand, c.Op}] = m
		}
		if c.State == "measured" && !include(c) {
			m["ungraded"]++
		} else {
			m[c.State]++
		}
	}
	for _, k := range sortedKeys(cov, func(a, b k2) bool { return a.cand+a.op < b.cand+b.op }) {
		m := cov[k]
		w("| %s | %s | %d | %d | %d | %d | %d | %d | %d |", k.cand, k.op, m["measured"], m["ungraded"], m["partial"], m["failed"], m["incomplete"]+m["planned"], m["unsupported"], m["unavailable"])
	}

	w("\n## Time: candidate / product, paired per round (median of rounds; fixture ratios, not pooled)\n")
	w("Ratios above 1 mean the candidate took longer. Each fixture's ratio is the median over rounds of the paired block-median ratios;")
	w("the cross-fixture value is the median of fixture ratios, so a single large input does not dominate. Coverage is measured fixtures / expected fixtures.\n")
	w("| lang | candidate | op | group | coverage | median ratio | IQR of fixtures | inconclusive fixtures |")
	w("|---|---|---|---|---:|---:|---|---:|")
	writeGroupTable(&md, timing, cellList, func(s CellStat) (Ratio, bool) { return s.Ratio, s.Rounds > 0 })

	w("\n## Go-heap allocations per operation: candidate / product (mallocs; bytes)\n")
	w("Counters are runtime.ReadMemStats deltas of the Go heap only. C allocations and wasm linear memory are not counted, so a cgo library shows few Go allocations by construction.\n")
	w("| lang | candidate | op | group | coverage | mallocs ratio | bytes ratio |")
	w("|---|---|---|---|---:|---:|---:|")
	writeAllocTable(&md, alloc, cellList)

	w("\n## Peak RSS (large inputs, KiB, worker VmHWM): operation minus no-parse control\n")
	want := info.RSSRuns
	w("A delta is shown only when both the operation and its control have the %d scheduled fresh-process samples; otherwise the row says n/a.\n", want)
	w("| lang | input | op | adapter | samples | peak | control | delta |")
	w("|---|---|---|---|---:|---:|---:|---:|")
	writeMemory(&md, rows, want)

	for _, f := range []struct{ file, title string }{
		{"correctness.csv", "Correctness against the native reference"},
		{"builds.csv", "Builds: consumer size and the no-C-compiler build"},
	} {
		if err := appendCSVSummary(&md, filepath.Join(*in, f.file), f.title); err != nil {
			w("\n## %s\n\nnot available: %v", f.title, err)
		}
	}
	w("\n## Limits\n")
	w("- Timing and memory conclusions are Linux-only and specific to the runner architecture named above; absolute times between runs are not comparable.")
	w("- A fixture with a spread above 20%% of its median ratio, or a range crossing 1x, is inconclusive for a direction (never rerun to change it).")
	w("- Each candidate's grammar, runtime and scanner differ from the product's: the rows describe consumer artifacts, not an implementation-only change.")
	w("- Correctness levels compare parent/child structure; tree-lossy discards UNEXPECTED token text, shape-only ignores field labels and ERROR/MISSING marks, and neither is a C-agreement claim.")
	if err := writeFileAll(filepath.Join(*out, "summary.md"), []byte(md.String())); err != nil {
		return err
	}
	if err := writeStatCSV(filepath.Join(*out, "fixtures-time.csv"), timing); err != nil {
		return err
	}
	return writeStatCSV(filepath.Join(*out, "fixtures-alloc.csv"), alloc)
}

func readJSON(p string, v any) error {
	b, err := readBounded(p, maxFile)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func sortedKeys[K comparable, V any](m map[K]V, less func(a, b K) bool) []K {
	ks := make([]K, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool { return less(ks[i], ks[j]) })
	return ks
}

func writeRunInfo(md *strings.Builder, info runInfo) {
	b, _ := json.MarshalIndent(info, "", "  ")
	md.WriteString("<details><summary>run.json (identities, parameters, host)</summary>\n\n~~~json\n")
	md.WriteString(strings.ReplaceAll(string(b), "~~~", "~_~"))
	md.WriteString("\n~~~\n\n</details>\n")
}

type gkey struct{ lang, cand, op, group string }

func writeGroupTable(md *strings.Builder, stats []CellStat, cells []Cell, pick func(CellStat) (Ratio, bool)) {
	expected := map[gkey]int{}
	for _, c := range cells {
		if c.State == "measured" || c.State == "failed" || c.State == "partial" || c.State == "incomplete" || c.State == "planned" {
			expected[gkey{c.Lang, c.Cand, c.Op, c.Group}]++
		}
	}
	got := map[gkey][]Ratio{}
	for _, s := range stats {
		if r, ok := pick(s); ok {
			k := gkey{s.Lang, s.Cand, s.Op, s.Group}
			got[k] = append(got[k], r)
		}
	}
	for _, k := range sortedKeys(expected, func(a, b gkey) bool {
		return strings.Join([]string{a.lang, a.cand, a.op, a.group}, "|") < strings.Join([]string{b.lang, b.cand, b.op, b.group}, "|")
	}) {
		rs := got[k]
		var med []float64
		bad := 0
		for _, r := range rs {
			med = append(med, r.Median)
			if r.Inconclusive {
				bad++
			}
		}
		fmt.Fprintf(md, "| %s | %s | %s | %s | %d/%d | %s | %s-%s | %d |\n", k.lang, k.cand, k.op, k.group, len(rs), expected[k],
			f3(median(med)), f3(quantile(med, .25)), f3(quantile(med, .75)), bad)
	}
}

func writeAllocTable(md *strings.Builder, stats []CellStat, cells []Cell) {
	expected := map[gkey]int{}
	for _, c := range cells {
		if c.State == "measured" || c.State == "failed" || c.State == "partial" || c.State == "incomplete" || c.State == "planned" {
			expected[gkey{c.Lang, c.Cand, c.Op, c.Group}]++
		}
	}
	type pair struct{ a, b []float64 }
	got := map[gkey]*pair{}
	for _, s := range stats {
		if s.Ratio.N == 0 {
			continue
		}
		k := gkey{s.Lang, s.Cand, s.Op, s.Group}
		if got[k] == nil {
			got[k] = &pair{}
		}
		got[k].a, got[k].b = append(got[k].a, s.Ratio.Median), append(got[k].b, s.Metric2.Median)
	}
	for _, k := range sortedKeys(expected, func(a, b gkey) bool {
		return strings.Join([]string{a.lang, a.cand, a.op, a.group}, "|") < strings.Join([]string{b.lang, b.cand, b.op, b.group}, "|")
	}) {
		p := got[k]
		if p == nil {
			p = &pair{}
		}
		fmt.Fprintf(md, "| %s | %s | %s | %s | %d/%d | %s | %s |\n", k.lang, k.cand, k.op, k.group, len(p.a), expected[k], f3(median(p.a)), f3(median(p.b)))
	}
}

func writeMemory(md *strings.Builder, rows []Row, want int) {
	type mk struct{ lang, input, op, adapter string }
	rss, ctl := map[mk][]float64{}, map[mk][]float64{}
	for _, r := range rows {
		res := okRes(r)
		if res == nil || (r.Pass != "rss" && r.Pass != "ctl") {
			continue
		}
		k := mk{r.Lang, r.Input, r.Op, r.Adapter}
		if r.Pass == "rss" {
			rss[k] = append(rss[k], float64(res.PeakRSSKiB))
		} else {
			ctl[k] = append(ctl[k], float64(res.PeakRSSKiB))
		}
	}
	for _, k := range sortedKeys(rss, func(a, b mk) bool {
		return strings.Join([]string{a.lang, a.input, a.op, a.adapter}, "|") < strings.Join([]string{b.lang, b.input, b.op, b.adapter}, "|")
	}) {
		p, c := median(rss[k]), median(ctl[k])
		if len(rss[k]) < want || len(ctl[k]) < want {
			fmt.Fprintf(md, "| %s | %s | %s | %s | %d/%d | n/a | n/a | n/a |\n", mdEscape(k.lang), mdEscape(k.input), k.op, k.adapter, len(rss[k]), len(ctl[k]))
			continue
		}
		fmt.Fprintf(md, "| %s | %s | %s | %s | %d/%d | %.0f | %.0f | %.0f |\n", mdEscape(k.lang), mdEscape(k.input), k.op, k.adapter, len(rss[k]), len(ctl[k]), p, c, p-c)
	}
}

// appendCSVSummary embeds a (small, bounded) CSV as a Markdown table so the summary alone carries the evidence.
func appendCSVSummary(md *strings.Builder, p, title string) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	recs, err := csv.NewReader(f).ReadAll()
	if err != nil || len(recs) == 0 {
		return fmt.Errorf("%s: %v", p, err)
	}
	fmt.Fprintf(md, "\n## %s\n\n", title)
	if len(recs) > 400 { // the CSV stays the complete record; the summary shows a bounded head
		fmt.Fprintf(md, "(first 400 of %d rows; the CSV artifact is complete)\n\n", len(recs)-1)
		recs = recs[:401]
	}
	fmt.Fprintf(md, "| %s |\n|%s\n", strings.Join(recs[0], " | "), strings.Repeat("---|", len(recs[0])))
	for _, r := range recs[1:] {
		for i := range r {
			r[i] = mdEscape(strings.ReplaceAll(r[i], "\n", " "))
		}
		fmt.Fprintf(md, "| %s |\n", strings.Join(r, " | "))
	}
	return nil
}

func writeStatCSV(p string, stats []CellStat) error {
	var b strings.Builder
	b.WriteString("lang,candidate,op,group,input,rounds,product_ms,candidate_ms,ratio_median,q1,q3,min,max,inconclusive,reason,metric2_median,product_mallocs,product_bytes,candidate_mallocs,candidate_bytes\n")
	for _, s := range stats {
		fmt.Fprintf(&b, "%s,%s,%s,%s,%q,%d,%s,%s,%s,%s,%s,%s,%s,%t,%q,%s,%s,%s,%s,%s\n", s.Lang, s.Cand, s.Op, s.Group, s.Input, s.Rounds,
			f3(s.ProdMs), f3(s.CandMs), f3(s.Ratio.Median), f3(s.Ratio.Q1), f3(s.Ratio.Q3), f3(s.Ratio.Min), f3(s.Ratio.Max),
			s.Ratio.Inconclusive, s.Ratio.InconclusiveReason, f3(s.Metric2.Median),
			f3(s.ProdAlloc[0]), f3(s.ProdAlloc[1]), f3(s.CandAlloc[0]), f3(s.CandAlloc[1]))
	}
	return writeFileAll(p, []byte(b.String()))
}

var _ = slices.Sort[[]int]

// exactRounds reports whether the sorted round numbers are exactly 0..n-1 (the scheduled set, not just its count).
func exactRounds(rounds []int) bool {
	for i, r := range rounds {
		if r != i {
			return false
		}
	}
	return true
}

// bothPasses keeps only the cells that have complete rows in both passes, so a cell whose allocation rows are missing
// cannot leave a time ratio behind (and the reverse).
func bothPasses(a, b []CellStat) ([]CellStat, []CellStat) {
	key := func(s CellStat) string { return strings.Join([]string{s.Lang, s.Cand, s.Op, s.Group, s.Input}, "|") }
	inA, inB := map[string]bool{}, map[string]bool{}
	for _, s := range a {
		inA[key(s)] = true
	}
	for _, s := range b {
		inB[key(s)] = true
	}
	keep := func(in []CellStat, other map[string]bool) (out []CellStat) {
		for _, s := range in {
			if other[key(s)] {
				out = append(out, s)
			}
		}
		return out
	}
	return keep(a, inB), keep(b, inA)
}
