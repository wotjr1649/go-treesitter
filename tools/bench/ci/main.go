// Command benchci is the portable driver of the speed and memory lanes (.github/workflows/bench.yml): it fetches the
// large inputs, times the product bench binaries against the native reference in the same window, applies the
// pre-declared criteria of docs/specs/validation.md and prints a markdown summary. Stdlib only.
//
//	benchci fetch -manifest large.json -data <ciconf data dir> -out <large dir>      (-print: only print the sha256 values)
//	benchci run -bench <dir of bench binaries named <lang>[.exe]> -native <dir of native-ref executables>
//	            -data <ciconf data dir> -large <large dir> -out <result dir> [-langs a,b] [-rounds N ...]
//
// Criteria (unchanged from the Windows lane): large-input geomean of product/native <= 2.5; small (<= 25 KB) files:
// per-file median <= 1 ms or <= 2.5x the same file's native median; peak resident set <= 100 x input + 64 MB; after
// Close every engine region returned and private bytes within start + 16 MB (darwin: net of the runtime's released
// heap growth, see memgate.go). A criterion the platform cannot measure
// is NOT_RUN (never PASS); a measured breach makes the exit status 1.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	mb         = 1 << 20
	smallLimit = 25 * 1024
)

var allLangs = []string{"json", "golang", "python", "typescript", "tsx", "csharp", "tsql"}

// dataLang is the directory name of a language in the ciconf data tree (it calls golang "go").
func dataLang(l string) string {
	if l == "golang" {
		return "go"
	}
	return l
}

type entry struct {
	Lang   string `json:"lang"`
	File   string `json:"file"`
	Kind   string `json:"kind"` // url | npm | concat | json
	URL    string `json:"url,omitempty"`
	Member string `json:"member,omitempty"`
	Seed   uint64 `json:"seed,omitempty"`
	SHA256 string `json:"sha256"`
}

func main() {
	if len(os.Args) < 2 {
		die("usage: benchci fetch|run ...")
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	manifest := fs.String("manifest", "large.json", "large input manifest")
	data := fs.String("data", "", "ciconf data directory (corpus/<lang>/*.in)")
	out := fs.String("out", "", "output directory")
	printOnly := fs.Bool("print", false, "fetch: print sha256 values instead of verifying them")
	benchDir := fs.String("bench", "", "directory of product bench binaries")
	nativeDir := fs.String("native", "", "directory of native-ref executables")
	large := fs.String("large", "", "directory filled by fetch")
	langsFlag := fs.String("langs", strings.Join(allLangs, ","), "languages to run")
	lr := fs.Int("large-rounds", 5, "paired rounds per large input")
	lreps := fs.Int("large-reps", 5, "timed reps per round (plus one warm-up)")
	cr := fs.Int("corpus-rounds", 3, "paired rounds over the small corpus")
	creps := fs.Int("corpus-reps", 7, "timed reps per file")
	mruns := fs.Int("mem-runs", 3, "fresh processes per memory measurement")
	fs.Parse(os.Args[2:])
	switch os.Args[1] {
	case "fetch":
		must(fetch(*manifest, *data, *out, *printOnly))
	case "run":
		ok, err := run(*benchDir, *nativeDir, *data, *large, *out, strings.Split(*langsFlag, ","), *lr, *lreps, *cr, *creps, *mruns)
		must(err)
		if !ok {
			os.Exit(1)
		}
	default:
		die("unknown command", os.Args[1])
	}
}

func die(a ...any) { fmt.Fprintln(os.Stderr, a...); os.Exit(2) }
func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "benchci:", err)
		os.Exit(1)
	}
}
func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// ---- fetch ----

func fetch(manifest, data, out string, printOnly bool) error {
	raw, err := os.ReadFile(manifest)
	if err != nil {
		return err
	}
	var es []entry
	if err := json.Unmarshal(raw, &es); err != nil {
		return err
	}
	var rows [][]string
	for _, e := range es {
		b, src, err := produce(e, data)
		if err != nil {
			return fmt.Errorf("%s/%s: %w", e.Lang, e.File, err)
		}
		got := sum(b)
		if printOnly {
			fmt.Printf("%s/%s %d %s\n", e.Lang, e.File, len(b), got)
		} else if got != e.SHA256 {
			return fmt.Errorf("%s/%s: sha256 %s, pinned %s", e.Lang, e.File, got, e.SHA256)
		}
		p := filepath.Join(out, e.Lang, e.File)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			return err
		}
		rows = append(rows, []string{e.Lang, e.File, src, strconv.Itoa(len(b)), got})
	}
	return writeCSV(filepath.Join(out, "manifest.csv"), []string{"lang", "file", "source", "bytes", "sha256"}, rows)
}

func produce(e entry, data string) ([]byte, string, error) {
	switch e.Kind {
	case "url":
		b, err := get(e.URL)
		return b, e.URL, err
	case "npm":
		b, err := get(e.URL)
		if err != nil {
			return nil, "", err
		}
		zr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, "", err
		}
		tr := tar.NewReader(zr)
		for {
			h, err := tr.Next()
			if err != nil {
				return nil, "", fmt.Errorf("%s not in %s: %w", e.Member, e.URL, err)
			}
			if h.Name == e.Member {
				m, err := io.ReadAll(tr)
				return m, e.URL + "#" + e.Member, err
			}
		}
	case "concat": // the pinned grammar's own corpus inputs, concatenated cyclically with GO until 1 MB
		files, _ := filepath.Glob(filepath.Join(data, "corpus", dataLang(e.Lang), "*.in"))
		sort.Strings(files)
		var parts [][]byte
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				return nil, "", err
			}
			parts = append(parts, bytes.TrimRight(b, " \t\r\n\v\f"))
		}
		if len(parts) == 0 {
			return nil, "", fmt.Errorf("no corpus inputs under %s", data)
		}
		var buf []byte
		n := 0
		for len(buf) < 1_000_000 {
			buf = append(append(buf, parts[n%len(parts)]...), "\nGO\n"...)
			n++
		}
		return buf, fmt.Sprintf("ciconf corpus %s/*.in cyclic with GO, %d pieces of %d", e.Lang, n, len(parts)), nil
	case "json": // seeded generator (splitmix64), about 5 MB
		r := rng{e.Seed}
		var gen func(d int) string
		gen = func(d int) string {
			k := r.float()
			if d > 4 || k < 0.3 {
				switch r.intn(6) {
				case 0:
					return strconv.Itoa(r.intn(2_000_001) - 1_000_000)
				case 1:
					return strconv.FormatFloat(r.float(), 'g', -1, 64)
				case 2:
					return `"str` + strconv.Itoa(r.intn(1_000_000_000)) + `"`
				case 3:
					return "true"
				case 4:
					return "false"
				}
				return "null"
			}
			var sb strings.Builder
			n := 1 + r.intn(6)
			if k < 0.65 {
				sb.WriteByte('[')
				for i := 0; i < n; i++ {
					if i > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(gen(d + 1))
				}
				sb.WriteByte(']')
			} else {
				sb.WriteByte('{')
				for i := 0; i < n; i++ {
					if i > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(`"k` + strconv.Itoa(r.intn(1000)) + `": ` + gen(d+1))
				}
				sb.WriteByte('}')
			}
			return sb.String()
		}
		var items []string
		for total := 0; total < 5_000_000; {
			s := gen(0)
			items = append(items, s)
			total += len(s) + 2
		}
		return []byte("[" + strings.Join(items, ",\n") + "]"), fmt.Sprintf("generated by benchci, splitmix64 seed=%d", e.Seed), nil
	}
	return nil, "", fmt.Errorf("unknown kind %q", e.Kind)
}

type rng struct{ s uint64 }

func (r *rng) next() uint64 {
	r.s += 0x9e3779b97f4a7c15
	z := r.s
	z = (z ^ z>>30) * 0xbf58476d1ce4e5b9
	z = (z ^ z>>27) * 0x94d049bb133111eb
	return z ^ z>>31
}
func (r *rng) intn(n int) int { return int(r.next() % uint64(n)) }
func (r *rng) float() float64 { return float64(r.next()>>11) / (1 << 53) }

func get(url string) (b []byte, err error) {
	for try := 0; try < 3; try++ { // transient download errors only; results are never retried
		if try > 0 {
			time.Sleep(time.Duration(try) * 5 * time.Second)
		}
		var resp *http.Response
		if resp, err = (&http.Client{Timeout: 5 * time.Minute}).Get(url); err != nil {
			continue
		}
		b, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err == nil && resp.StatusCode != http.StatusOK {
			err = fmt.Errorf("GET %s: %s", url, resp.Status)
		}
		if err == nil {
			return b, nil
		}
	}
	return nil, err
}

// ---- run ----

type result struct {
	out, errOut string
	code        int
}

func runCmd(timeout time.Duration, name string, args ...string) result {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var so, se bytes.Buffer
	c := exec.CommandContext(ctx, name, args...)
	c.Stdout, c.Stderr = &so, &se
	err := c.Run()
	r := result{out: so.String(), errOut: strings.TrimSpace(se.String())}
	if err != nil {
		r.code = -1
		if ee, ok := err.(*exec.ExitError); ok {
			r.code = ee.ExitCode()
		}
		if ctx.Err() != nil {
			r.errOut += " (timeout)"
		}
	}
	return r
}

func med(xs []float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	s := slices.Clone(xs)
	sort.Float64s(s)
	if n := len(s); n%2 == 0 {
		return (s[n/2-1] + s[n/2]) / 2
	}
	return s[len(s)/2]
}

func geo(xs []float64) float64 {
	t := 0.0
	for _, x := range xs {
		t += math.Log(x)
	}
	return math.Exp(t / float64(len(xs)))
}

func exeName(dir, lang string) string {
	p := filepath.Join(dir, lang)
	if runtime.GOOS == "windows" {
		p += ".exe"
	}
	return p
}

func load1() string { // host load noise, recorded per round (Linux only)
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		return strings.Fields(string(b))[0]
	}
	return "n/a"
}

type large struct {
	lang, file, path string
	bytes            int
	sha              string
}

func run(benchDir, nativeDir, data, largeDir, out string, langs []string, lrounds, lreps, crounds, creps, mruns int) (bool, error) {
	if benchDir == "" || nativeDir == "" || data == "" || largeDir == "" || out == "" {
		return false, fmt.Errorf("run needs -bench -native -data -large -out")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return false, err
	}
	var md strings.Builder
	P := func(f string, a ...any) { fmt.Fprintf(&md, f+"\n", a...); fmt.Printf(f+"\n", a...) }
	ok := true
	fail := func(f string, a ...any) { ok = false; P("**FAIL** "+f, a...) }

	// Inputs: the manifest written by fetch.
	mrows, err := readCSV(filepath.Join(largeDir, "manifest.csv"))
	if err != nil {
		return false, err
	}
	var larges []large
	for _, r := range mrows[1:] {
		n, _ := strconv.Atoi(r[3])
		if slices.Contains(langs, r[0]) {
			larges = append(larges, large{r[0], r[1], filepath.Join(largeDir, r[0], r[1]), n, r[4]})
		}
	}
	P("# Speed and memory lane: %s/%s", runtime.GOOS, runtime.GOARCH)
	P("")
	P("commit `%s`, pins.json sha256 `%s`, %d CPUs, rounds large=%d x %d reps, corpus=%d x %d reps, mem runs=%d",
		os.Getenv("GITHUB_SHA"), fileSum(filepath.Join("tools", "gen", "pins.json")), runtime.NumCPU(), lrounds, lreps, crounds, creps, mruns)
	P("")
	P("| lang | product binary sha256 | native binary sha256 |")
	P("|---|---|---|")
	for _, l := range langs {
		P("| %s | `%.12s` | `%.12s` |", l, fileSum(exeName(benchDir, l)), fileSum(exeName(nativeDir, l)))
	}
	P("")

	// ---- large inputs: paired rounds, order alternating ----
	type key struct {
		round      int
		lang, file string
	}
	nat, mine := map[key][]float64{}, map[key][]float64{}
	var lcsv [][]string
	for rnd := 0; rnd < lrounds; rnd++ {
		load := load1()
		for _, in := range larges {
			order := []string{"native", "mine"}
			if rnd%2 == 1 {
				order = []string{"mine", "native"}
			}
			for _, who := range order {
				var r result
				if who == "native" {
					r = runCmd(30*time.Minute, exeName(nativeDir, in.lang), "speed", in.path, strconv.Itoa(lreps))
				} else {
					r = runCmd(30*time.Minute, exeName(benchDir, in.lang), "speed", "-file", in.path, "-reps", strconv.Itoa(lreps), "-op", "parse")
				}
				if r.code != 0 {
					fail("%s %s/%s round %d: exit %d %s", who, in.lang, in.file, rnd, r.code, r.errOut)
					continue
				}
				k := key{rnd, in.lang, in.file}
				for i, line := range strings.Split(strings.TrimSpace(r.out), "\n") {
					if strings.HasPrefix(line, "#") {
						continue
					}
					f := strings.Split(line, ",")
					v, err := strconv.ParseFloat(f[0], 64) // native: "ms"
					rep := i
					if who == "mine" {
						if len(f) < 2 {
							continue
						}
						rep, _ = strconv.Atoi(f[0])
						v, err = strconv.ParseFloat(f[1], 64) // bench: "rep,total_ms,parse_ms,info"
					} else {
						rep = i
					}
					if err != nil {
						continue
					}
					lcsv = append(lcsv, []string{strconv.Itoa(rnd), in.lang, in.file, strconv.Itoa(in.bytes), who, strconv.Itoa(rep), strconv.FormatFloat(v, 'f', 4, 64), load})
					if rep >= 1 {
						if who == "native" {
							nat[k] = append(nat[k], v)
						} else {
							mine[k] = append(mine[k], v)
						}
					}
				}
			}
		}
	}
	must(writeCSV(filepath.Join(out, "large.csv"), []string{"round", "lang", "file", "bytes", "who", "rep", "ms", "load1"}, lcsv))
	P("## Large inputs: Parse+Close vs native parse+delete, paired per round (criterion: geomean <= 2.5)")
	P("")
	P("| lang | file | bytes | sha256 | native ms | product ms | ratio (median of rounds) |")
	P("|---|---|---|---|---|---|---|")
	var perFile, all []float64
	for _, in := range larges {
		var rs, nm, mm []float64
		for rnd := 0; rnd < lrounds; rnd++ {
			k := key{rnd, in.lang, in.file}
			if len(nat[k]) > 0 && len(mine[k]) > 0 {
				rs = append(rs, med(mine[k])/med(nat[k]))
				nm, mm = append(nm, med(nat[k])), append(mm, med(mine[k]))
			}
		}
		if len(rs) == 0 {
			fail("%s/%s: no paired measurement", in.lang, in.file)
			continue
		}
		perFile, all = append(perFile, med(rs)), append(all, rs...)
		P("| %s | %s | %d | `%.12s` | %.1f | %.1f | %.2f |", in.lang, in.file, in.bytes, in.sha, med(nm), med(mm), med(rs))
	}
	if len(all) > 0 {
		gAll, gFile := geo(all), geo(perFile)
		P("")
		P("geomean product/native: all paired round x file %.3f, of per-file medians %.3f (criterion <= 2.5)", gAll, gFile)
		if gAll > 2.5 || gFile > 2.5 {
			fail("large-input geomean above 2.5")
		}
	}
	P("")

	// ---- small corpus ----
	type cs struct{ mine, nat map[int]float64 } // round -> median of timed reps
	P("## Small corpus (pinned grammars' own test/corpus inputs; criterion per file <= 25 KB: median <= 1 ms or <= 2.5x native)")
	P("")
	P("| lang | files <= 25 KB | median product ms | p95 product ms | passing | failing |")
	P("|---|---|---|---|---|---|")
	var ccsv [][]string
	for _, lang := range langs {
		files, _ := filepath.Glob(filepath.Join(data, "corpus", dataLang(lang), "*.in"))
		sort.Strings(files)
		var small []string
		for _, f := range files {
			if st, err := os.Stat(f); err == nil && st.Size() <= smallLimit {
				small = append(small, f)
			}
		}
		if len(small) == 0 {
			continue
		}
		list := filepath.Join(out, "list-"+lang+".txt")
		os.WriteFile(list, []byte(strings.Join(small, "\n")+"\n"), 0o644)
		per := map[string]*cs{}
		for rnd := 0; rnd < crounds; rnd++ {
			order := []string{"native", "mine"}
			if rnd%2 == 1 {
				order = []string{"mine", "native"}
			}
			for _, who := range order {
				var r result
				if who == "native" {
					r = runCmd(time.Hour, exeName(nativeDir, lang), "corpus", list, strconv.Itoa(creps))
				} else {
					r = runCmd(time.Hour, exeName(benchDir, lang), "corpus", "-list", list, "-reps", strconv.Itoa(creps))
				}
				if r.code != 0 {
					fail("%s corpus %s round %d: exit %d %s", who, lang, rnd, r.code, r.errOut)
					continue
				}
				for _, line := range strings.Split(strings.TrimSpace(r.out), "\n") {
					f := strings.SplitN(line, ",", 4)
					if len(f) != 4 {
						continue
					}
					var xs []float64
					for _, s := range strings.Split(f[3], ";")[1:] { // drop the warm-up
						v, _ := strconv.ParseFloat(s, 64)
						xs = append(xs, v)
					}
					if len(xs) == 0 || slices.Contains(xs, -1) {
						fail("%s corpus %s/%s: no valid timing (%s)", who, lang, f[0], f[3])
						continue
					}
					c := per[f[0]]
					if c == nil {
						c = &cs{map[int]float64{}, map[int]float64{}}
						per[f[0]] = c
					}
					if who == "native" {
						c.nat[rnd] = med(xs)
					} else {
						c.mine[rnd] = med(xs)
					}
					ccsv = append(ccsv, []string{strconv.Itoa(rnd), lang, f[0], f[1], who, f[2], strconv.FormatFloat(med(xs), 'f', 4, 64)})
				}
			}
		}
		var ms []float64
		pass, failing := 0, []string{}
		names := make([]string, 0, len(per))
		for n := range per {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			c := per[n]
			var m, v []float64
			for rnd, x := range c.mine {
				if y, ok := c.nat[rnd]; ok {
					m, v = append(m, x), append(v, y)
				}
			}
			if len(m) == 0 {
				continue
			}
			pm, pn := med(m), med(v)
			ms = append(ms, pm)
			if pm <= 1.0 || pm <= 2.5*pn {
				pass++
			} else {
				failing = append(failing, fmt.Sprintf("%s product %.3f ms native %.3f ms", n, pm, pn))
			}
		}
		sort.Float64s(ms)
		if len(ms) == 0 {
			fail("corpus %s: nothing measured", lang)
			continue
		}
		P("| %s | %d | %.3f | %.3f | %d | %d |", lang, len(ms), med(ms), ms[int(0.95*float64(len(ms)-1))], pass, len(failing))
		for _, f := range failing[:min(len(failing), 20)] {
			fail("corpus %s: %s (> 1 ms and > 2.5x native)", lang, f)
		}
		if len(failing) > 20 {
			fail("corpus %s: %d more files above the criterion", lang, len(failing)-20)
		}
	}
	must(writeCSV(filepath.Join(out, "corpus.csv"), []string{"round", "lang", "file", "bytes", "who", "outcome", "median_ms"}, ccsv))
	P("")

	// ---- memory: one fresh process per run ----
	P("## Memory (large inputs; peak <= 100 x input + 64 MB; after Close all engine regions returned and private bytes <= start + 16 MB)")
	P("")
	P("Private-bytes gate on **%s/%s**: %s", runtime.GOOS, runtime.GOARCH, map[bool]string{
		true:  "adjusted (ADR-0022 §3 macOS amendment): (footprint delta - growth of runtime heap/released bytes) <= 16 MB, and a snapshot case must have been collected; a missing field is a FAIL. The raw delta is shown too.",
		false: "raw: private delta <= 16 MB (adjusted = raw; the macOS adjustment does not apply)",
	}[runtime.GOOS == "darwin"])
	P("")
	P("| file | op | peak MB (max) | limit MB | private delta MB (raw, max) | private delta MB (adjusted, max) | regions parsed/after | end==bytes | peak | private | regions |")
	P("|---|---|---|---|---|---|---|---|---|---|---|")
	var mcsv [][]string
	type diagRun struct {
		in large
		op string
	}
	var privFailed []diagRun
	for _, in := range larges {
		for _, op := range []string{"parse", "snapshot"} {
			var peak, delta, adj, endBad int64
			privWhy := ""
			regParsed, regAfter, runs := int64(math.MaxInt64), int64(0), 0
			regStartBad := false
			for i := 0; i < mruns; i++ {
				if i == 0 { // darwin: keep the raw vmmap output of one run per input as evidence/fixtures
					os.Setenv("BENCH_VMMAP_DUMP", filepath.Join(out, "vmmap-raw", in.lang+"-"+in.file+"-"+op))
				} else {
					os.Unsetenv("BENCH_VMMAP_DUMP")
				}
				r := runCmd(15*time.Minute, exeName(benchDir, in.lang), "mem", "-file", in.path, "-op", op)
				if r.code != 0 {
					fail("mem %s/%s %s run %d: exit %d %s", in.lang, in.file, op, i, r.code, r.errOut)
					continue
				}
				kv := map[string]int64{}
				for _, tok := range strings.Fields(r.out) {
					if k, v, ok := strings.Cut(tok, "="); ok {
						if n, err := strconv.ParseInt(v, 10, 64); err == nil {
							kv[k] = n
						}
					}
				}
				if _, ok := kv["priv_after"]; !ok { // a missing number must not read as a zero delta
					fail("mem %s/%s %s run %d: no priv_after in %q", in.lang, in.file, op, i, r.out)
					continue
				}
				runs++
				peak = max(peak, kv["peak_ws"])
				rawD, adjD, ok, why := privGate(runtime.GOOS, op, kv)
				delta, adj = max(delta, rawD), max(adj, adjD)
				if !ok && privWhy == "" {
					privWhy = fmt.Sprintf("run %d: %s", i, why)
				}
				if kv["end"] != int64(in.bytes) {
					endBad++
				}
				regParsed, regAfter = min(regParsed, kv["regions_parsed"]), max(regAfter, kv["regions_after"])
				regStartBad = regStartBad || kv["regions_start"] > 0
				mcsv = append(mcsv, []string{in.lang, in.file, op, strconv.Itoa(i), strconv.FormatInt(kv["peak_ws"], 10), strconv.FormatInt(kv["priv_start"], 10), strconv.FormatInt(kv["priv_after"], 10),
					strconv.FormatInt(kv["regions_parsed"], 10), strconv.FormatInt(kv["regions_after"], 10), strconv.FormatInt(kv["end"], 10),
					strconv.FormatInt(kv["rel_start"], 10), strconv.FormatInt(kv["rel_after"], 10), strconv.FormatInt(kv["snap_collected"], 10), strconv.FormatInt(adjD, 10)})
			}
			if runs == 0 {
				continue
			}
			limit := int64(100*in.bytes + 64*mb)
			pk, pv, rg := "PASS", "PASS", "PASS"
			if peak > limit {
				pk = "FAIL"
			}
			if privWhy != "" {
				pv = "FAIL"
				privFailed = append(privFailed, diagRun{in, op})
			}
			switch {
			case regParsed < 0:
				rg = "NOT_RUN (not measurable on " + runtime.GOOS + ")"
			case regParsed == 0 || regStartBad:
				rg = "NOT_RUN (detector saw no engine region while the parser was open)"
			case regAfter != 0:
				rg = "FAIL"
			}
			P("| %s | %s | %.1f | %.1f | %.1f | %d/%d | %v | %s | %s | %s |", in.file, op, float64(peak)/mb, float64(limit)/mb, float64(delta)/mb, float64(adj)/mb, regParsed, regAfter, endBad == 0, pk, pv, rg)
			if pk == "FAIL" || pv == "FAIL" || rg == "FAIL" || endBad > 0 {
				fail("memory %s/%s %s: peak %s, private %s (%s), regions %s, end mismatches %d", in.lang, in.file, op, pk, pv, privWhy, rg, endBad)
			}
			if strings.HasPrefix(rg, "NOT_RUN") {
				fmt.Printf("::warning::regions NOT_RUN for %s/%s %s\n", in.lang, in.file, op)
			}
		}
	}
	// Evidence only, never a gate: where a failed private-bytes check keeps its memory (see tools/bench/diag.go). The
	// GODEBUG variants are separate runs; madvdontneed is a no-op on darwin (the runtime uses MADV_FREE_REUSABLE there)
	// and is listed so that the log says so.
	for _, d := range privFailed {
		for _, v := range []struct{ name, godebug string }{{"default", ""}, {"scavtrace", "scavtrace=1"}, {"madvdontneed", "madvdontneed=1"}} {
			base := filepath.Join(out, "mem-diag", d.in.lang+"-"+d.in.file+"-"+d.op+"-"+v.name)
			os.Setenv("BENCH_MEM_DIAG", "1")
			os.Setenv("GODEBUG", v.godebug)
			if v.name == "default" {
				os.Setenv("BENCH_VMMAP_DUMP", base+"-vmmap")
			}
			r := runCmd(15*time.Minute, exeName(benchDir, d.in.lang), "mem", "-file", d.in.path, "-op", d.op)
			os.Unsetenv("BENCH_MEM_DIAG")
			os.Unsetenv("GODEBUG")
			os.Unsetenv("BENCH_VMMAP_DUMP")
			must(os.MkdirAll(filepath.Dir(base), 0o755))
			rec := "exit " + strconv.Itoa(r.code) + "\n" + r.out + "\n--- stderr ---\n" + r.errOut + "\n"
			must(os.WriteFile(base+".txt", []byte(rec), 0o644))
		}
	}
	must(writeCSV(filepath.Join(out, "mem.csv"), []string{"lang", "file", "op", "run", "peak_ws", "priv_start", "priv_after", "regions_parsed", "regions_after", "end", "rel_start", "rel_after", "snap_collected", "priv_delta_adjusted"}, mcsv))
	P("")
	if ok {
		P("**All measured criteria passed** (rows marked NOT_RUN were not measured).")
	}
	return ok, os.WriteFile(filepath.Join(out, "summary.md"), []byte(md.String()), 0o644)
}

func fileSum(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return "missing"
	}
	return sum(b)
}

func readCSV(p string) ([][]string, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return csv.NewReader(f).ReadAll()
}

func writeCSV(p string, header []string, rows [][]string) error {
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	w.Write(header)
	w.WriteAll(rows)
	if err := w.Error(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
