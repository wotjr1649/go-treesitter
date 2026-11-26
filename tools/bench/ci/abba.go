package main

// ABBA paired regression gate (prototype). Two bench binaries, A (baseline) and B (candidate), are timed in one job on
// one runner. Each round runs A, B, B, A (a linear drift of the host cancels exactly), each run being one
// "bench speed" process whose per-rep median is the run's time. A round's log ratio is
// ((ln B1 + ln B2) - (ln A1 + ln A2)) / 2; the verdict comes from the t-interval of the per-round log ratios.
//
//	benchci abba -a <dir of A binaries> -b <dir of B binaries> -out <dir> [-langs a,b] [-mode large|corpus]
//	             [-large <dir filled by fetch>] [-data <ciconf data dir>] [-abba-rounds 8] [-abba-max-rounds 16]
//	             [-large-reps 5] [-corpus-reps 7] [-corpus-files 100] [-budget 10] [-a-args "-op parse"] [-b-args "-op parse"]
//	benchci abba-calib -calib <abba-rounds.csv> [-budget 10] [-abba-rounds 8] [-abba-max-rounds 16] [-resample N]
//	             (A=A runs: verdict rates per round count, of the sequential rule, and of seeded resamples)
//
// Modes: large = one process per large input (parse of 0.2-9 MB, throughput-dominated); corpus = the pinned grammar's
// own small inputs (<= 25 KB, evenly spaced, at most -corpus-files) in one "bench corpus" process per run, where a
// round's value is the geomean over files of the per-file ABBA ratios; with "-a-args -fresh -b-args -fresh" every rep
// also pays NewParser + Parser.Close (startup-dominated). Fixed baseline: A is any older tree (a release tag), see
// bench-abba.yml.
//
// Sequential rule (pre-declared, two looks): decide after -abba-rounds (8); only if that is INCONCLUSIVE, measure up to
// -abba-max-rounds (16) rounds and decide once more from all 16 with the same rule. A decisive first look is final.
//
// Rule (budget b, two-sided 90% interval [lo, hi] of the ratio B/A, i.e. one-sided 95% bounds):
//   PASS          hi <  1+b   the candidate is within the budget with confidence
//   REGRESS       lo >  1+b   the candidate is over the budget with confidence
//   INCONCLUSIVE  otherwise, and always with fewer than 3 rounds (the owner decides; never rerun until green)
// Error rates of the two looks (each look is a one-sided 95% bound): with the true ratio exactly at the budget, a
// false PASS needs the upper bound below it and a false REGRESS the lower bound above it, each at most ~5% per look, so
// at most ~10% over both looks (a union bound under normal noise; the second look only runs after an INCONCLUSIVE first
// look). The measured noise has heavy tails, which makes the bounds anti-conservative at the budget itself (REGRESS up to
// ~29% there in seeded resamples); with the true ratio at 0.8 of the budget or less the measured false REGRESS rate
// stayed <= 1.8% (<= 0.1% at half the budget): see REPORT.md.
// Exit status: 0 PASS, 1 REGRESS or a failed run, 3 INCONCLUSIVE.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// tCrit95[df-1] is the one-sided 95% quantile of Student's t (df 1..30). shortcut: df > 30 uses the df=30 value
// (conservative); more than 31 rounds is never needed, upgrade to an exact quantile if that changes.
var tCrit95 = [30]float64{6.314, 2.920, 2.353, 2.132, 2.015, 1.943, 1.895, 1.860, 1.833, 1.812, 1.796, 1.782, 1.771, 1.761, 1.753,
	1.746, 1.740, 1.734, 1.729, 1.725, 1.721, 1.717, 1.714, 1.711, 1.708, 1.706, 1.703, 1.701, 1.699, 1.697}

type abbaStat struct {
	N       int
	Ratio   float64 // exp(mean log ratio) = B/A
	Lo, Hi  float64 // two-sided 90% interval of the ratio
	Verdict string  // PASS | REGRESS | INCONCLUSIVE
}

// roundLog is one ABBA round's log ratio from the four run times in execution order A1 B1 B2 A2.
func roundLog(a1, b1, b2, a2 float64) float64 {
	return (math.Log(b1) + math.Log(b2) - math.Log(a1) - math.Log(a2)) / 2
}

// abbaVerdict decides from per-round log ratios; budget is a fraction (0.10 = +10%).
func abbaVerdict(logs []float64, budget float64) abbaStat {
	n := len(logs)
	s := abbaStat{N: n, Ratio: math.NaN(), Lo: math.NaN(), Hi: math.NaN(), Verdict: "INCONCLUSIVE"}
	if n < 3 {
		return s
	}
	var m, ss float64
	for _, x := range logs {
		m += x
	}
	m /= float64(n)
	for _, x := range logs {
		ss += (x - m) * (x - m)
	}
	h := tCrit95[min(n-1, 30)-1] * math.Sqrt(ss/float64(n-1)/float64(n))
	s.Ratio, s.Lo, s.Hi = math.Exp(m), math.Exp(m-h), math.Exp(m+h)
	switch lim := 1 + budget; {
	case s.Hi < lim:
		s.Verdict = "PASS"
	case s.Lo > lim:
		s.Verdict = "REGRESS"
	}
	return s
}

func (s abbaStat) String() string {
	return fmt.Sprintf("n=%d ratio %.4f CI90 [%.4f, %.4f] %s", s.N, s.Ratio, s.Lo, s.Hi, s.Verdict)
}

// speedMedian is the median of the timed reps (rep >= 1) of one "bench speed" run.
func speedMedian(out string) (float64, error) {
	var xs []float64
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(line, ",")
		if strings.HasPrefix(line, "#") || len(f) < 2 {
			continue
		}
		if rep, err := strconv.Atoi(f[0]); err == nil && rep >= 1 {
			if v, err := strconv.ParseFloat(f[1], 64); err == nil && v > 0 {
				xs = append(xs, v)
			}
		}
	}
	if len(xs) == 0 {
		return 0, fmt.Errorf("no timed rep in %q", out)
	}
	return med(xs), nil
}

func worse(a, b string) string { // REGRESS > INCONCLUSIVE > PASS
	rank := map[string]int{"PASS": 0, "INCONCLUSIVE": 1, "REGRESS": 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

// abbaSequential is the two-look rule on the log ratios of consecutive rounds: the verdict at minR rounds, and only if
// that is INCONCLUSIVE and more rounds exist, the verdict at min(len, maxR) rounds.
func abbaSequential(logs []float64, minR, maxR int, budget float64) abbaStat {
	first := abbaVerdict(logs[:min(len(logs), minR)], budget)
	if first.Verdict != "INCONCLUSIVE" || len(logs) <= minR {
		return first
	}
	return abbaVerdict(logs[:min(len(logs), maxR)], budget)
}

// parseCorpus reads "bench corpus" output (name,bytes,outcome,ms0;ms1;...) into the per-file median of the timed reps.
func parseCorpus(out string) (map[string]float64, error) {
	m := map[string]float64{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.SplitN(line, ",", 4)
		if len(f) != 4 {
			continue
		}
		var xs []float64
		for _, s := range strings.Split(f[3], ";")[1:] { // drop the warm-up
			v, err := strconv.ParseFloat(s, 64)
			if err != nil || v < 0 {
				return nil, fmt.Errorf("%s: no valid timing (%s)", f[0], f[3])
			}
			xs = append(xs, max(v, 1e-4)) // the clock prints 4 decimals; a log needs > 0
		}
		if len(xs) == 0 {
			return nil, fmt.Errorf("%s: no timed rep", f[0])
		}
		m[f[0]] = med(xs)
	}
	if len(m) == 0 {
		return nil, fmt.Errorf("no file in %q", out)
	}
	return m, nil
}

// pickEvenly returns at most n of the sorted names, evenly spaced (deterministic).
func pickEvenly(names []string, n int) []string {
	if n <= 0 || len(names) <= n {
		return names
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, names[i*len(names)/n])
	}
	return out
}

type abbaCfg struct {
	aDir, bDir, largeDir, dataDir, out, mode string
	langs                                    []string
	rounds, maxRounds, reps, corpusFiles     int
	budgetPct                                float64
	aArgs, bArgs                             string
}

// unit is one process run per position: a large file, or the whole small-file list.
type unit struct{ lang, name, path string }

// abba returns the exit status.
func abba(c abbaCfg) (int, error) {
	if c.aDir == "" || c.bDir == "" || c.out == "" || c.rounds < 3 || c.maxRounds < c.rounds || (c.mode != "large" && c.mode != "corpus") {
		return 1, fmt.Errorf("abba needs -a -b -out, -mode large|corpus and 3 <= -abba-rounds <= -abba-max-rounds")
	}
	if err := os.MkdirAll(c.out, 0o755); err != nil {
		return 1, err
	}
	var mrows [][]string
	if c.mode == "large" {
		var err error
		if mrows, err = readCSV(filepath.Join(c.largeDir, "manifest.csv")); err != nil {
			return 1, err
		}
	} else if c.dataDir == "" {
		return 1, fmt.Errorf("-mode corpus needs -data")
	}
	var md strings.Builder
	P := func(f string, a ...any) { fmt.Fprintf(&md, f+"\n", a...); fmt.Printf(f+"\n", a...) }
	P("# ABBA paired speed gate (%s): %s/%s", c.mode, runtime.GOOS, runtime.GOARCH)
	P("")
	P("budget +%.0f%%, A,B,B,A rounds: decide at %d, only if INCONCLUSIVE continue to %d and decide again; %d timed reps per run, A args %q, B args %q, %d CPUs",
		c.budgetPct, c.rounds, c.maxRounds, c.reps, c.aArgs, c.bArgs, runtime.NumCPU())
	P("")
	P("| lang | A binary sha256 | B binary sha256 |")
	P("|---|---|---|")
	for _, l := range c.langs {
		P("| %s | %.12s | %.12s |", l, fileSum(exeName(c.aDir, l)), fileSum(exeName(c.bDir, l)))
	}
	P("")
	P("| lang | what | ratio B/A | CI90 | rounds | verdict |")
	P("|---|---|---|---|---|---|")
	var rows [][]string
	overall, failed := "PASS", false
	run1 := func(dir, args string, u unit) (map[string]float64, error) {
		argv := []string{"speed", "-file", u.path, "-reps", strconv.Itoa(c.reps)}
		if c.mode == "corpus" {
			argv = []string{"corpus", "-list", u.path, "-reps", strconv.Itoa(c.reps)}
		}
		r := runCmd(30*time.Minute, exeName(dir, u.lang), append(argv, strings.Fields(args)...)...)
		if r.code != 0 {
			return nil, fmt.Errorf("exit %d %s", r.code, r.errOut)
		}
		if c.mode == "corpus" {
			return parseCorpus(r.out)
		}
		v, err := speedMedian(r.out)
		return map[string]float64{u.name: v}, err
	}
	budget := c.budgetPct / 100
	for _, lang := range c.langs {
		var units []unit
		what := ""
		if c.mode == "large" {
			for _, r := range mrows[1:] {
				if r[0] == lang {
					units = append(units, unit{lang, r[1], filepath.Join(c.largeDir, r[0], r[1])})
				}
			}
		} else {
			files, _ := filepath.Glob(filepath.Join(c.dataDir, "corpus", dataLang(lang), "*.in"))
			sort.Strings(files)
			var small []string
			for _, f := range files {
				if st, err := os.Stat(f); err == nil && st.Size() <= smallLimit {
					small = append(small, f)
				}
			}
			if small = pickEvenly(small, c.corpusFiles); len(small) > 0 {
				list := filepath.Join(c.out, "list-"+lang+".txt")
				if err := os.WriteFile(list, []byte(strings.Join(small, "\n")+"\n"), 0o644); err != nil {
					return 1, err
				}
				units = []unit{{lang, "corpus", list}}
				what = fmt.Sprintf("%d small files (<= 25 KB), geomean per round", len(small))
			}
		}
		if len(units) == 0 {
			P("| %s | - | - | - | - | NOT_RUN (no input) |", lang)
			continue
		}
		if c.mode == "large" {
			what = fmt.Sprintf("geomean of %d large file(s) per round", len(units))
		}
		perFile := map[string][]float64{} // large mode: the per-file information table
		var langLogs []float64
		ok := true
		target := c.rounds
	rounds:
		for rnd := 0; rnd < target; rnd++ {
			load := load1()
			sum, cnt := 0.0, 0
			for _, in := range units {
				var t [4]map[string]float64
				for pos, side := range []string{"A", "B", "B", "A"} {
					dir, args := c.aDir, c.aArgs
					if side == "B" {
						dir, args = c.bDir, c.bArgs
					}
					v, err := run1(dir, args, in)
					if err != nil {
						P("**FAIL** %s/%s round %d position %d (%s): %v", in.lang, in.name, rnd, pos, side, err)
						ok, failed = false, true
						break rounds
					}
					t[pos] = v
					for name, ms := range v {
						rows = append(rows, []string{strconv.Itoa(rnd), in.lang, name, strconv.Itoa(pos), side, strconv.FormatFloat(ms, 'f', 4, 64), load})
					}
				}
				for name := range t[0] {
					x := [4]float64{t[0][name], t[1][name], t[2][name], t[3][name]}
					if slices.Contains(x[:], 0) { // a file missing from one run
						P("**FAIL** %s/%s round %d: %s missing from a run", in.lang, in.name, rnd, name)
						ok, failed = false, true
						break rounds
					}
					l := roundLog(x[0], x[1], x[2], x[3])
					sum, cnt = sum+l, cnt+1
					if c.mode == "large" {
						perFile[name] = append(perFile[name], l)
					}
				}
			}
			langLogs = append(langLogs, sum/float64(cnt))
			if len(langLogs) == c.rounds && c.maxRounds > c.rounds && abbaVerdict(langLogs, budget).Verdict == "INCONCLUSIVE" {
				target = c.maxRounds // the pre-declared second look
				P("%s: INCONCLUSIVE after %d rounds, continuing to %d", lang, c.rounds, c.maxRounds)
			}
		}
		if !ok {
			continue
		}
		if c.mode == "large" {
			for _, in := range units {
				s := abbaVerdict(perFile[in.name], budget)
				P("| %s | %s | %.4f | [%.4f, %.4f] | %d | %s (per file, information) |", in.lang, in.name, s.Ratio, s.Lo, s.Hi, s.N, s.Verdict)
			}
		}
		s := abbaVerdict(langLogs, budget)
		P("| **%s** | %s | **%.4f** | [%.4f, %.4f] | %d | **%s** |", lang, what, s.Ratio, s.Lo, s.Hi, s.N, s.Verdict)
		overall = worse(overall, s.Verdict)
	}
	P("")
	if failed {
		P("**FAIL**: a run failed; the verdict is not valid.")
	}
	P("overall: **%s**", overall)
	must(writeCSV(filepath.Join(c.out, "abba-rounds.csv"), []string{"round", "lang", "file", "pos", "side", "median_ms", "load1"}, rows))
	if err := os.WriteFile(filepath.Join(c.out, "abba-summary.md"), []byte(md.String()), 0o644); err != nil {
		return 1, err
	}
	switch {
	case failed || overall == "REGRESS":
		return 1, nil
	case overall == "INCONCLUSIVE":
		return 3, nil
	}
	return 0, nil
}

// abbaCalib reads abba-rounds.csv of A=A runs. Per language it prints (1) for each round count n the verdict rates over
// all windows of n consecutive rounds (the windows overlap: a diagnostic, not independent trials), (2) the same for the
// sequential rule over windows of maxR rounds, with the mean number of rounds used, and (3) with resample > 0, the
// rates of fixed minR, fixed maxR and the sequential rule over seeded i.i.d. resamples of the measured round values,
// at true ratios 1.0, +0.5, +0.8, +1.0 and +1.5 times the budget (the values are centred first; resampling drops
// the autocorrelation of host load).
func abbaCalib(path string, budgetPct float64, minR, maxR, resample int) error {
	rows, err := readCSV(path)
	if err != nil {
		return err
	}
	type key struct {
		lang, file string
		rnd        int
	}
	t := map[key][4]float64{}
	var langs []string
	for _, r := range rows[1:] {
		rnd, _ := strconv.Atoi(r[0])
		pos, _ := strconv.Atoi(r[3])
		v, _ := strconv.ParseFloat(r[5], 64)
		k := key{r[1], r[2], rnd}
		x := t[k]
		x[pos] = v
		t[k] = x
		if !slices.Contains(langs, r[1]) {
			langs = append(langs, r[1])
		}
	}
	budget := budgetPct / 100
	logsOf := func(lang string) (logs []float64) {
		for rnd := 0; ; rnd++ {
			s, c := 0.0, 0
			for k, x := range t {
				if k.lang == lang && k.rnd == rnd {
					s += roundLog(x[0], x[1], x[2], x[3])
					c++
				}
			}
			if c == 0 {
				return logs
			}
			logs = append(logs, s/float64(c))
		}
	}
	fmt.Println("| lang | rounds | n | windows | PASS | INCONCLUSIVE | REGRESS | CI contains 1.0 | median CI half-width % |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|")
	all := map[string][]float64{}
	for _, lang := range langs {
		logs := logsOf(lang)
		all[lang] = logs
		for _, n := range []int{3, 4, 5, 6, 8, 10, 12, 16} {
			if n > len(logs) {
				break
			}
			cnt := map[string]int{}
			cover := 0
			var hw []float64
			for i := 0; i+n <= len(logs); i++ {
				s := abbaVerdict(logs[i:i+n], budget)
				cnt[s.Verdict]++
				if s.Lo <= 1 && 1 <= s.Hi {
					cover++
				}
				hw = append(hw, (s.Hi-s.Lo)/2*100)
			}
			w := len(logs) - n + 1
			fmt.Printf("| %s | %d | %d | %d | %d | %d | %d | %d/%d | %.2f |\n", lang, len(logs), n, w, cnt["PASS"], cnt["INCONCLUSIVE"], cnt["REGRESS"], cover, w, med(hw))
		}
	}
	fmt.Println()
	fmt.Printf("Sequential rule %d -> %d on sliding windows of %d rounds (overlapping)\n\n", minR, maxR, maxR)
	fmt.Println("| lang | windows | fixed-" + strconv.Itoa(minR) + " P/I/R | sequential P/I/R | mean rounds used |")
	fmt.Println("|---|---|---|---|---|")
	for _, lang := range langs {
		logs := all[lang]
		var f, s [3]int
		used, w := 0, 0
		idx := map[string]int{"PASS": 0, "INCONCLUSIVE": 1, "REGRESS": 2}
		for i := 0; i+maxR <= len(logs); i++ {
			win := logs[i : i+maxR]
			f[idx[abbaVerdict(win[:minR], budget).Verdict]]++
			st := abbaSequential(win, minR, maxR, budget)
			s[idx[st.Verdict]]++
			used += st.N
			w++
		}
		if w > 0 {
			fmt.Printf("| %s | %d | %d/%d/%d | %d/%d/%d | %.1f |\n", lang, w, f[0], f[1], f[2], s[0], s[1], s[2], float64(used)/float64(w))
		}
	}
	if resample <= 0 {
		return nil
	}
	fmt.Println()
	fmt.Printf("Seeded i.i.d. resamples (%d per row) of the measured round values, rule at budget %.0f%%\n\n", resample, budgetPct)
	fmt.Println("| lang | true ratio | fixed-" + strconv.Itoa(minR) + " P/I/R % | fixed-" + strconv.Itoa(maxR) + " P/I/R % | sequential P/I/R % | mean rounds |")
	fmt.Println("|---|---|---|---|---|---|")
	for _, lang := range langs {
		mean := 0.0 // centre the empirical values so that the true ratio of a row is exactly its shift
		for _, x := range all[lang] {
			mean += x / float64(len(all[lang]))
		}
		for _, shift := range []float64{1.0, 1 + budget/2, 1 + 0.8*budget, 1 + budget, 1 + 1.5*budget} {
			r := rng{uint64(len(lang))*7919 + 12345}
			var f, g, s [3]int
			used := 0
			idx := map[string]int{"PASS": 0, "INCONCLUSIVE": 1, "REGRESS": 2}
			for k := 0; k < resample; k++ {
				seq := make([]float64, maxR)
				for i := range seq {
					seq[i] = all[lang][r.intn(len(all[lang]))] - mean + math.Log(shift)
				}
				f[idx[abbaVerdict(seq[:minR], budget).Verdict]]++
				g[idx[abbaVerdict(seq, budget).Verdict]]++
				st := abbaSequential(seq, minR, maxR, budget)
				s[idx[st.Verdict]]++
				used += st.N
			}
			p := func(x [3]int) string {
				return fmt.Sprintf("%.1f/%.1f/%.1f", 100*float64(x[0])/float64(resample), 100*float64(x[1])/float64(resample), 100*float64(x[2])/float64(resample))
			}
			fmt.Printf("| %s | %.2f | %s | %s | %s | %.1f |\n", lang, shift, p(f), p(g), p(s), float64(used)/float64(resample))
		}
	}
	return nil
}
