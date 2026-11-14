// Command ciconf builds the data directory of the CI conformance lane (no local data needed): it takes the inputs from
// the pinned grammar repositories' own test/corpus files (extracted by `native-ref -nobuild -corpus`), derives
// deterministic seeded edits, and records the native C reference results by running the executables that native-ref
// built. tools/conformance then compares the CGO-free engine against that directory (TSG_DATA_DIR).
//
//	ciconf -pins tools/gen/pins.json -corpus <dir> -exe <native-ref out dir> -out <data dir> [-seed N] [-j N]
//
// Layout (the same as the local session-15 tree, so TestCorpus and TestEdits read both):
//
//	corpus/manifest.csv, corpus/<lang>/<name>.in, corpus/skipped.csv, native/dumps/<lang>/<name>.dump
//	edits/manifest.csv, edits/results-native.csv, edits/<lang>/<name>.{old,new,ranges}, ci.json
//
// Stdlib only; it never executes anything but the native-ref executables.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// langs maps the conformance language name to its pins.json key.
var langs = map[string]string{
	"bash": "bash", "c": "c", "cpp": "cpp", "csharp": "csharp", "css": "css", "go": "golang",
	"html": "html", "java": "java", "javascript": "javascript", "json": "json", "python": "python",
	"rust": "rust", "tsql": "tsql", "tsx": "tsx", "typescript": "typescript",
}

type entry struct {
	file, name string
	attrs      []string
	input      []byte
}

func main() {
	pinsFile := flag.String("pins", filepath.Join("tools", "gen", "pins.json"), "pins file")
	corpus := flag.String("corpus", "", "directory filled by native-ref -nobuild -corpus")
	exeDir := flag.String("exe", "", "directory holding the native-ref executables")
	out := flag.String("out", "", "data directory to create (must be empty or absent)")
	seed := flag.Uint64("seed", 1, "edit seed")
	jobs := flag.Int("j", runtime.NumCPU(), "parallel native runs")
	selected := flag.String("lang", "", "comma-separated languages (default: all)")
	flag.Parse()
	if *corpus == "" || *exeDir == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: ciconf -corpus DIR -exe DIR -out DIR [-pins F] [-seed N] [-j N] [-lang LANG,...]")
		os.Exit(2)
	}
	if err := run(*pinsFile, *corpus, *exeDir, *out, *seed, max(1, *jobs), *selected); err != nil {
		fmt.Fprintln(os.Stderr, "ciconf:", err)
		os.Exit(1)
	}
}

type job struct {
	lang, id, name string // id = lang/name
	src            []byte
	file, test     string
	attrs          string
	seq            int
}

func run(pinsFile, corpus, exeDir, out string, seed uint64, jobs int, selected string) error {
	pinsRaw, err := os.ReadFile(pinsFile)
	if err != nil {
		return err
	}
	var p struct {
		Grammars map[string]struct {
			ExtractDir string `json:"extractDir"`
		} `json:"grammars"`
	}
	if err := json.Unmarshal(pinsRaw, &p); err != nil {
		return err
	}
	if ents, err := os.ReadDir(out); err == nil && len(ents) > 0 {
		return fmt.Errorf("%s is not empty", out)
	}
	names, err := selectedLanguages(selected)
	if err != nil {
		return err
	}

	var inputs []job
	var skipped [][]string
	for _, lang := range names {
		g, ok := p.Grammars[langs[lang]]
		if !ok {
			return fmt.Errorf("no grammar %q in %s", langs[lang], pinsFile)
		}
		root := filepath.Join(corpus, g.ExtractDir, "test", "corpus")
		var files []string
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.Type().IsRegular() && strings.HasSuffix(d.Name(), ".txt") {
				files = append(files, path)
			}
			return err
		})
		if err != nil {
			return fmt.Errorf("%s: %w", lang, err)
		}
		slices.Sort(files)
		if len(files) == 0 {
			return fmt.Errorf("%s: no corpus files under %s", lang, root)
		}
		for _, f := range files {
			raw, err := os.ReadFile(f)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, f)
			rel = filepath.ToSlash(rel)
			ents, err := parseCorpus(raw)
			if err != nil {
				return fmt.Errorf("%s/%s: %w", lang, rel, err)
			}
			stem := strings.NewReplacer("/", "_", " ", "_").Replace(strings.TrimSuffix(rel, ".txt"))
			for i, e := range ents {
				attrs := strings.Join(e.attrs, " ")
				name := fmt.Sprintf("%s-%03d", stem, i+1)
				if reason := skipReason(e.attrs, lang); reason != "" {
					skipped = append(skipped, []string{lang + "/" + name, lang, rel, e.name, attrs, reason})
					continue
				}
				inputs = append(inputs, job{lang: lang, id: lang + "/" + name, name: name, src: e.input, file: rel, test: e.name, attrs: attrs, seq: i + 1})
			}
		}
	}

	exe := func(lang string) string {
		x := filepath.Join(exeDir, langs[lang])
		if runtime.GOOS == "windows" {
			x += ".exe"
		}
		return x
	}
	for _, l := range names {
		if _, err := os.Stat(exe(l)); err != nil {
			return err
		}
	}

	// Inputs: write, then native dump.
	var fails failures
	corpusRows := make([][]string, len(inputs))
	parallel(jobs, len(inputs), func(i int) {
		j := inputs[i]
		path := filepath.Join(out, "corpus", j.lang, j.name+".in")
		must(writeFile(path, j.src))
		corpusRows[i] = []string{j.id, j.lang, j.file + "#" + strconv.Itoa(j.seq), strconv.Itoa(len(j.src)), sha(j.src), j.file, j.test, j.attrs}
		dump, code, err := runExe(exe(j.lang), "dump", path)
		if err != nil || code != 0 {
			fails.add(j.id, "dump", code, err)
			return
		}
		must(writeFile(filepath.Join(out, "native", "dumps", j.lang, j.name+".dump"), dump))
	})
	must(writeCSV(filepath.Join(out, "corpus", "manifest.csv"), []string{"id", "lang", "source_ref", "bytes", "sha256", "corpus_file", "test_name", "attrs"}, corpusRows))
	must(writeCSV(filepath.Join(out, "corpus", "skipped.csv"), []string{"id", "lang", "corpus_file", "test_name", "attrs", "reason"}, skipped))

	// Edits: three per input (insert, delete, replace), seeded per input id.
	type edit struct {
		id, lang, name, base, kind string
		old, new                   []byte
		start, oe, ne              int
		seed                       uint64
	}
	var edits []edit
	for _, j := range inputs {
		for k, kind := range []string{"insert", "delete", "replace"} {
			eseed := idSeed(j.id+"#"+kind, seed)
			nw, s, oe, ne, actual := makeEdit(j.src, kind, rand.New(rand.NewPCG(seed, eseed)))
			name := fmt.Sprintf("%s-e%d", j.name, k+1)
			edits = append(edits, edit{j.lang + "/" + name, j.lang, name, j.id, actual, j.src, nw, s, oe, ne, eseed})
		}
	}
	editRows := make([][]string, len(edits))
	resRows := make([][]string, len(edits))
	parallel(jobs, len(edits), func(i int) {
		e := &edits[i]
		d := filepath.Join(out, "edits", e.lang)
		oldP, newP := filepath.Join(d, e.name+".old"), filepath.Join(d, e.name+".new")
		must(writeFile(oldP, e.old))
		must(writeFile(newP, e.new))
		editRows[i] = []string{e.id, e.lang, e.base, strconv.Itoa(e.start), strconv.Itoa(e.oe), strconv.Itoa(e.ne), sha(e.old), sha(e.new), e.kind, strconv.FormatUint(e.seed, 10)}
		args := []string{oldP, newP, strconv.Itoa(e.start), strconv.Itoa(e.oe), strconv.Itoa(e.ne)}
		t0 := time.Now()
		inc, ec, err1 := runExe(exe(e.lang), append([]string{"edit"}, args...)...)
		ms := float64(time.Since(t0).Microseconds()) / 1000
		fresh, fc, err2 := runExe(exe(e.lang), "dump", newP)
		cr, cc, err3 := runExe(exe(e.lang), append([]string{"changed-ranges"}, args...)...)
		if err1 != nil || ec != 0 {
			fails.add(e.id, "edit", ec, err1)
		}
		if err2 != nil || fc != 0 {
			fails.add(e.id, "dump(new)", fc, err2)
		}
		if err3 != nil || cc != 0 {
			fails.add(e.id, "changed-ranges", cc, err3)
		} else {
			must(writeFile(filepath.Join(d, e.name+".ranges"), cr))
		}
		resRows[i] = []string{e.id, strconv.Itoa(ec), sha(inc), strconv.Itoa(fc), sha(fresh), strconv.FormatFloat(ms, 'f', 1, 64)}
	})
	must(writeCSV(filepath.Join(out, "edits", "manifest.csv"), []string{"id", "lang", "base_ref", "start", "old_end", "new_end", "old_sha256", "new_sha256", "kind", "seed"}, editRows))
	must(writeCSV(filepath.Join(out, "edits", "results-native.csv"), []string{"id", "edit_exit", "edit_sha256", "fresh_exit", "fresh_sha256", "edit_ms"}, resRows))

	perLang := map[string][3]int{} // inputs, edits, skipped
	for _, j := range inputs {
		c := perLang[j.lang]
		c[0]++
		c[1] += 3
		perLang[j.lang] = c
	}
	for _, s := range skipped {
		c := perLang[s[1]]
		c[2]++
		perLang[s[1]] = c
	}
	meta, _ := json.MarshalIndent(map[string]any{
		"kind": "ciconf/1", "seed": seed, "editsPerInput": 3, "pinsSHA256": sha(pinsRaw),
		"inputs": len(inputs), "edits": len(edits), "skipped": len(skipped), "perLang": perLang,
	}, "", "  ")
	must(writeFile(filepath.Join(out, "ci.json"), append(meta, '\n')))
	for _, l := range names {
		c := perLang[l]
		fmt.Printf("%-10s inputs=%d edits=%d skipped=%d\n", l, c[0], c[1], c[2])
	}
	fmt.Printf("total      inputs=%d edits=%d skipped=%d native-failures=%d\n", len(inputs), len(edits), len(skipped), len(fails.rows))
	if len(fails.rows) > 0 {
		must(writeCSV(filepath.Join(out, "native-failures.csv"), []string{"id", "step", "exit", "error"}, fails.rows))
		return fmt.Errorf("%d native reference runs failed (see %s)", len(fails.rows), filepath.Join(out, "native-failures.csv"))
	}
	return nil
}

func selectedLanguages(selected string) ([]string, error) {
	var names []string
	if selected == "" {
		for l := range langs {
			names = append(names, l)
		}
	} else {
		for _, l := range strings.Split(selected, ",") {
			l = strings.TrimSpace(l)
			if _, ok := langs[l]; !ok {
				return nil, fmt.Errorf("unknown language %q", l)
			}
			names = append(names, l)
		}
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}

func skipReason(attrs []string, lang string) string {
	for _, a := range attrs {
		switch {
		case a == ":skip":
			return "skip (marked :skip upstream)"
		case strings.HasPrefix(a, ":language(") && strings.TrimSuffix(strings.TrimPrefix(a, ":language("), ")") != lang:
			return "other language (" + a + ")"
		}
	}
	return ""
}

// parseCorpus reads tree-sitter's corpus format: a "===" line, one or more name/attribute lines, a "===" line, the
// input, a "---" line, then the expected tree up to the next header. The input keeps its bytes except that the one
// blank line that separates it from "---" is dropped, as in the tree-sitter CLI.
func parseCorpus(data []byte) ([]entry, error) {
	var lines [][]byte
	var offs []int
	for off := 0; off < len(data); {
		end := bytes.IndexByte(data[off:], '\n')
		if end < 0 {
			end = len(data) - off
		} else {
			end++
		}
		offs = append(offs, off)
		lines = append(lines, bytes.TrimRight(data[off:off+end], "\r\n"))
		off += end
	}
	offs = append(offs, len(data))
	isRun := func(l []byte, c byte) bool { // 3+ c, optionally followed by a non-c suffix
		n := 0
		for n < len(l) && l[n] == c {
			n++
		}
		return n >= 3 && (n == len(l) || l[n] != c)
	}
	// header returns the index of the closing "===" line when a header starts at i.
	header := func(i int) int {
		if !isRun(lines[i], '=') {
			return -1
		}
		for k := i + 1; k < len(lines); k++ {
			if isRun(lines[k], '=') {
				if k == i+1 {
					return -1
				}
				return k
			}
			if len(lines[k]) > 0 && lines[k][0] == '=' {
				return -1
			}
		}
		return -1
	}
	var out []entry
	for i := 0; i < len(lines); {
		k := header(i)
		if k < 0 {
			if len(bytes.TrimSpace(lines[i])) == 0 {
				i++
				continue
			}
			return nil, fmt.Errorf("line %d: expected a test header, got %q", i+1, lines[i])
		}
		var e entry
		var nameLines []string
		for _, l := range lines[i+1 : k] {
			if len(l) > 0 && l[0] == ':' {
				e.attrs = append(e.attrs, strings.TrimSpace(string(l)))
			} else if s := strings.TrimSpace(string(l)); s != "" {
				nameLines = append(nameLines, s)
			}
		}
		e.name = strings.Join(nameLines, " ")
		d := -1
		for m := k + 1; m < len(lines); m++ {
			if isRun(lines[m], '-') {
				d = m
				break
			}
		}
		if d < 0 {
			return nil, fmt.Errorf("line %d (%s): no --- divider", i+1, e.name)
		}
		in := data[offs[k+1]:offs[d]]
		if bytes.HasSuffix(in, []byte("\n\n")) {
			in = in[:len(in)-1]
		}
		e.input = in
		out = append(out, e)
		i = d + 1
		for i < len(lines) && header(i) < 0 {
			i++
		}
	}
	return out, nil
}

var insertPool = []string{" ", "\n", "x", ";", "}", ")", "{", "0", "\"", "/*", "//", "a b", "\t", "\n\n"}

// makeEdit applies one seeded edit to src and returns the new text and the byte offsets (start, old_end, new_end) in
// the form Tree.Edit takes. Positions are moved to UTF-8 boundaries and the inserted text is ASCII or a slice of src.
func makeEdit(src []byte, kind string, r *rand.Rand) (nw []byte, start, oe, ne int, actual string) {
	n := len(src)
	snap := func(i int) int {
		for i > 0 && i < n && !utf8.RuneStart(src[i]) {
			i--
		}
		return i
	}
	var ins []byte
	pick := func() []byte {
		if n > 0 && r.IntN(len(insertPool)+1) == len(insertPool) {
			a := snap(r.IntN(n))
			b := snap(min(n, a+1+r.IntN(16)))
			return src[a:b]
		}
		return []byte(insertPool[r.IntN(len(insertPool))])
	}
	switch {
	case kind == "insert" || n == 0:
		start = snap(r.IntN(n + 1))
		oe = start
		ins, actual = pick(), "insert"
	default:
		start = snap(r.IntN(n))
		oe = start + 1 + r.IntN(min(16, n-start))
		for oe < n && !utf8.RuneStart(src[oe]) {
			oe++
		}
		actual = kind
		if kind == "replace" {
			ins = pick()
		}
	}
	nw = append(append(append([]byte{}, src[:start]...), ins...), src[oe:]...)
	return nw, start, oe, start + len(ins), actual
}

func idSeed(s string, seed uint64) uint64 {
	h := sha256.Sum256([]byte(strconv.FormatUint(seed, 10) + ":" + s))
	return uint64(h[0])<<56 | uint64(h[1])<<48 | uint64(h[2])<<40 | uint64(h[3])<<32 | uint64(h[4])<<24 | uint64(h[5])<<16 | uint64(h[6])<<8 | uint64(h[7])
}

func runExe(exe string, args ...string) (out []byte, code int, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		return stdout.Bytes(), ee.ExitCode(), fmt.Errorf("%s", bytes.TrimSpace(stderr.Bytes()))
	}
	return stdout.Bytes(), 0, err
}

type failures struct {
	mu   sync.Mutex
	rows [][]string
}

func (f *failures) add(id, step string, code int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	f.rows = append(f.rows, []string{id, step, strconv.Itoa(code), msg})
}

func parallel(workers, n int, fn func(i int)) {
	var wg sync.WaitGroup
	ch := make(chan int)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range ch {
				fn(i)
			}
		}()
	}
	for i := 0; i < n; i++ {
		ch <- i
	}
	close(ch)
	wg.Wait()
}

func sha(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "ciconf:", err)
		os.Exit(1)
	}
}

func writeFile(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func writeCSV(path string, header []string, rows [][]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
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
