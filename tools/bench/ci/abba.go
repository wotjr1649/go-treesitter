package main

// ABBA paired regression gate (prototype). Two bench binaries, A (baseline) and B (candidate), are timed in one job on
// one runner. Each round runs A, B, B, A (a linear drift of the host cancels exactly), each run being one
// "bench speed" process whose per-rep median is the run's time. A round's log ratio is
// ((ln B1 + ln B2) - (ln A1 + ln A2)) / 2; the verdict comes from the t-interval of the per-round log ratios.
//
//	benchci abba -a <dir of A binaries> -b <dir of B binaries> -large <dir filled by fetch> -out <dir> [-langs a,b]
//	             [-abba-rounds 8] [-large-reps 5] [-budget 10] [-a-args "-op parse"] [-b-args "-op parse"]
//	benchci abba-calib -calib <abba-rounds.csv> [-budget 10]     (A=A runs: verdict rates per round count)
//
// Rule (budget b, two-sided 90% interval [lo, hi] of the ratio B/A, i.e. one-sided 95% bounds):
//   PASS          hi <  1+b   the candidate is within the budget with confidence
//   REGRESS       lo >  1+b   the candidate is over the budget with confidence
//   INCONCLUSIVE  otherwise, and always with fewer than 3 rounds (the owner decides; never rerun until green)
// Exit status: 0 PASS, 1 REGRESS or a failed run, 3 INCONCLUSIVE.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

// abba returns the exit status.
func abba(aDir, bDir, largeDir, out string, langs []string, rounds, reps int, budgetPct float64, aArgs, bArgs string) (int, error) {
	if aDir == "" || bDir == "" || largeDir == "" || out == "" || rounds < 3 {
		return 1, fmt.Errorf("abba needs -a -b -large -out and -abba-rounds >= 3")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return 1, err
	}
	mrows, err := readCSV(filepath.Join(largeDir, "manifest.csv"))
	if err != nil {
		return 1, err
	}
	var md strings.Builder
	P := func(f string, a ...any) { fmt.Fprintf(&md, f+"\n", a...); fmt.Printf(f+"\n", a...) }
	P("# ABBA paired speed gate: %s/%s", runtime.GOOS, runtime.GOARCH)
	P("")
	P("budget +%.0f%%, %d rounds x (A,B,B,A), %d timed reps per run, A args %q, B args %q, %d CPUs", budgetPct, rounds, reps, aArgs, bArgs, runtime.NumCPU())
	P("")
	P("| lang | A binary sha256 | B binary sha256 |")
	P("|---|---|---|")
	for _, l := range langs {
		P("| %s | %.12s | %.12s |", l, fileSum(exeName(aDir, l)), fileSum(exeName(bDir, l)))
	}
	P("")
	P("| lang | file | ratio B/A | CI90 | verdict |")
	P("|---|---|---|---|---|")
	var rows [][]string
	overall, failed := "PASS", false
	run1 := func(dir, args, lang, path string) (float64, error) {
		argv := append([]string{"speed", "-file", path, "-reps", strconv.Itoa(reps)}, strings.Fields(args)...)
		r := runCmd(30*time.Minute, exeName(dir, lang), argv...)
		if r.code != 0 {
			return 0, fmt.Errorf("exit %d %s", r.code, r.errOut)
		}
		return speedMedian(r.out)
	}
	for _, lang := range langs {
		var files []large
		for _, r := range mrows[1:] {
			if r[0] == lang {
				n, _ := strconv.Atoi(r[3])
				files = append(files, large{r[0], r[1], filepath.Join(largeDir, r[0], r[1]), n, r[4]})
			}
		}
		if len(files) == 0 {
			P("| %s | - | - | - | NOT_RUN (no large input) |", lang)
			continue
		}
		perFile := make([][]float64, len(files)) // [file][round]
		var langLogs []float64
		ok := true
	rounds:
		for rnd := 0; rnd < rounds; rnd++ {
			load := load1()
			sum := 0.0
			for i, in := range files {
				var t [4]float64
				for pos, side := range []string{"A", "B", "B", "A"} {
					dir, args := aDir, aArgs
					if side == "B" {
						dir, args = bDir, bArgs
					}
					v, err := run1(dir, args, in.lang, in.path)
					if err != nil {
						P("**FAIL** %s/%s round %d position %d (%s): %v", in.lang, in.file, rnd, pos, side, err)
						ok, failed = false, true
						break rounds
					}
					t[pos] = v
					rows = append(rows, []string{strconv.Itoa(rnd), in.lang, in.file, strconv.Itoa(pos), side, strconv.FormatFloat(v, 'f', 4, 64), load})
				}
				l := roundLog(t[0], t[1], t[2], t[3])
				perFile[i] = append(perFile[i], l)
				sum += l
			}
			langLogs = append(langLogs, sum/float64(len(files)))
		}
		if !ok {
			continue
		}
		v := "PASS"
		for i, in := range files {
			s := abbaVerdict(perFile[i], budgetPct/100)
			P("| %s | %s | %.4f | [%.4f, %.4f] | %s (per file, information) |", in.lang, in.file, s.Ratio, s.Lo, s.Hi, s.Verdict)
		}
		s := abbaVerdict(langLogs, budgetPct/100)
		v = s.Verdict
		P("| **%s** | geomean of %d file(s) per round | **%.4f** | [%.4f, %.4f] | **%s** |", lang, len(files), s.Ratio, s.Lo, s.Hi, v)
		overall = worse(overall, v)
	}
	P("")
	if failed {
		P("**FAIL**: a run failed; the verdict is not valid.")
	}
	P("overall: **%s**", overall)
	must(writeCSV(filepath.Join(out, "abba-rounds.csv"), []string{"round", "lang", "file", "pos", "side", "median_ms", "load1"}, rows))
	if err := os.WriteFile(filepath.Join(out, "abba-summary.md"), []byte(md.String()), 0o644); err != nil {
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

// abbaCalib reads abba-rounds.csv of A=A runs and prints, per language and round count n, how often the rule gives each
// verdict over all windows of n consecutive rounds, and the median CI half-width in percent (the windows overlap: the
// rates are a diagnostic, not independent trials).
func abbaCalib(path string, budgetPct float64) error {
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
	fmt.Println("| lang | rounds | n | windows | PASS | INCONCLUSIVE | REGRESS | CI contains 1.0 | median CI half-width % |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|")
	for _, lang := range langs {
		var logs []float64
		for rnd := 0; ; rnd++ {
			s, c := 0.0, 0
			for k, x := range t {
				if k.lang == lang && k.rnd == rnd {
					s += roundLog(x[0], x[1], x[2], x[3])
					c++
				}
			}
			if c == 0 {
				break
			}
			logs = append(logs, s/float64(c))
		}
		for _, n := range []int{3, 4, 5, 6, 8, 10, 12, 16} {
			if n > len(logs) {
				break
			}
			cnt := map[string]int{}
			cover := 0
			var hw []float64
			for i := 0; i+n <= len(logs); i++ {
				s := abbaVerdict(logs[i:i+n], budgetPct/100)
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
	return nil
}
