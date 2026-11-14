package conformance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	treesitter "github.com/wotjr1649/go-treesitter"
)

// Raw-match gate for queries (docs/design/query-abi.md, "Verification"): for every query file of the pinned grammar
// repositories (plus a few structural probes) and every corpus input, the matches the CGO-free engine returns must be
// byte-identical to the matches `<native exe> query` prints for the same bytes: pattern, capture index, node preorder
// index, symbol and byte range of every capture, in the same order. The C engine never evaluates text predicates, so
// "raw" means before the Go-side predicate filter.
//
// Test hook: the public Matches API always filters. The only seam this module has is the query text, so the raw Go side
// runs the query with every `(#...)` predicate list overwritten by spaces (stripPredicates): same offsets, same
// patterns, same captures, same C matching. The unmodified query must still compile and its filtered matches must be a
// subsequence of the raw ones (checked below).

// stripPredicates blanks every list whose first element starts with '#' (a predicate or directive) with spaces. Strings
// ("..." with backslash escapes) and ;-comments are skipped, so a "(#" inside them is left alone.
func stripPredicates(q string) string {
	b := []byte(q)
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case '"':
			for i++; i < len(b) && b[i] != '"'; i++ {
				if b[i] == '\\' {
					i++
				}
			}
		case ';':
			for i < len(b) && b[i] != '\n' {
				i++
			}
		case '(':
			j := i + 1
			for j < len(b) && (b[j] == ' ' || b[j] == '\t' || b[j] == '\n' || b[j] == '\r') {
				j++
			}
			if j >= len(b) || b[j] != '#' {
				continue
			}
			depth := 0
			k := i
			for ; k < len(b); k++ {
				switch b[k] {
				case '"':
					for k++; k < len(b) && b[k] != '"'; k++ {
						if b[k] == '\\' {
							k++
						}
					}
				case '(':
					depth++
				case ')':
					depth--
				}
				if depth == 0 {
					break
				}
			}
			for m := i; m <= k && m < len(b); m++ {
				if b[m] != '\n' {
					b[m] = ' '
				}
			}
			i = k
		}
	}
	return string(b)
}

func TestStripPredicates(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`(a) @x`, `(a) @x`},
		{`((a) @x (#eq? @x "y"))`, `((a) @x              )`},
		{`((a) @x (#match? @x "\")(#")) ; (#c)`, "((a) @x " + strings.Repeat(" ", 20) + ") ; (#c)"},
		{"((a) @x ( #set! k \"v\" ) (b))", "((a) @x " + strings.Repeat(" ", 15) + " (b))"},
		{`"(#eq?)" @s`, `"(#eq?)" @s`},
	} {
		if got := stripPredicates(c.in); got != c.want || len(got) != len(c.in) {
			t.Errorf("stripPredicates(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// queryProbes are structural queries added to every language: wildcard, error and missing nodes, anchors and
// quantifiers exercise zero-width captures, repeated captures and the capture-to-node mapping beyond what the upstream
// highlight files reach. `_` is the only construct valid in every grammar.
var queryProbes = map[string]string{
	"probe-named":   "(_) @n",
	"probe-any":     "_ @a",
	"probe-error":   "(ERROR) @e",
	"probe-missing": "(MISSING) @m",
	"probe-anchor":  "((_) @a . (_) @b)",
	"probe-quant":   "(_ (_)+ @kids) @p",
	"probe-first":   "(_ . (_) @first)",
	"probe-last":    "(_ (_) @last .)",
	"probe-alt":     "[(_) @x (ERROR) @y]",
	"probe-opt":     "(_ (_)? @o) @p",
}

type queryFile struct{ name, source string }

func (q queryFile) probe() bool {
	return !strings.HasSuffix(q.name, ".scm") && strings.HasPrefix(q.name, "probe-")
}

// probeStride is TSG_QUERY_PROBE_STRIDE (default 1): the structural probes run on every stride-th input only; the
// grammar's own query files always run on every input. CI sets it to bound the number of native processes.
func probeStride() int {
	n, err := strconv.Atoi(os.Getenv("TSG_QUERY_PROBE_STRIDE"))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// queryDirFor is the directory holding <extractDir>/queries: TSG_QUERY_DIR (native-ref -queries) or, for the local
// session-15 tree, <data>/src.
func queryDirFor(root string) string {
	if d := os.Getenv("TSG_QUERY_DIR"); d != "" {
		return d
	}
	return filepath.Join(root, "src")
}

// queryFilesFor returns the pinned grammar repository's own queries/*.scm for a corpus language (sorted) and the
// structural probes.
func queryFilesFor(t *testing.T, root, lang string) []queryFile {
	t.Helper()
	var pins struct {
		Grammars map[string]struct {
			ExtractDir string `json:"extractDir"`
		} `json:"grammars"`
	}
	if err := json.Unmarshal(readFile(t, filepath.Join("..", "gen", "pins.json")), &pins); err != nil {
		t.Fatal(err)
	}
	pin := lang
	if pin == "go" {
		pin = "golang"
	}
	files, err := filepath.Glob(filepath.Join(queryDirFor(root), pins.Grammars[pin].ExtractDir, "queries", "*.scm"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no pinned upstream queries for %s under %s (native-ref -queries, TSG_QUERY_DIR)", lang, queryDirFor(root))
	}
	slices.Sort(files)
	var out []queryFile
	for _, f := range files {
		out = append(out, queryFile{filepath.Base(f), string(readFile(t, f))})
	}
	var names []string
	for k := range queryProbes {
		names = append(names, k)
	}
	slices.Sort(names)
	for _, k := range names {
		out = append(out, queryFile{k, queryProbes[k]})
	}
	return out
}

// goRawQuery renders the raw matches in the native "#tsg-query/1" text.
func goRawQuery(t *testing.T, tree *treesitter.Tree, q *treesitter.Query) []byte {
	t.Helper()
	var b bytes.Buffer
	fmt.Fprintf(&b, "#tsg-query/1 patterns=%d captures=%d\n", q.PatternCount(), len(q.CaptureNames()))
	for m, err := range tree.Matches(bg, q, nil) {
		if err != nil {
			t.Fatalf("Matches: %v", err)
		}
		fmt.Fprintf(&b, "m\t%d\t%d\n", m.Pattern, len(m.Captures))
		for _, c := range m.Captures {
			fmt.Fprintf(&b, "c\t%d\t%d\t%d\t%d\t%d\n", c.Index, c.Node.Index(), c.Node.Symbol(), c.Node.StartByte(), c.Node.EndByte())
		}
	}
	return b.Bytes()
}

// rawMatchKeys splits the rendered text into one comparable string per match.
func rawMatchKeys(raw []byte) []string {
	var keys []string
	for _, line := range strings.Split(string(raw), "\n")[1:] {
		switch {
		case strings.HasPrefix(line, "m\t"):
			keys = append(keys, line)
		case strings.HasPrefix(line, "c\t"):
			keys[len(keys)-1] += "|" + line
		}
	}
	return keys
}

func isSubsequence(sub, all []string) bool {
	i := 0
	for _, k := range all {
		if i < len(sub) && sub[i] == k {
			i++
		}
	}
	return i == len(sub)
}

type nativeJob struct {
	qi, input int
	out       []byte
	err       error
}

func TestQueryNative(t *testing.T) {
	root := dataRoot(t)
	rows := readCSV(t, filepath.Join(root, "corpus", "manifest.csv"))
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			exe, ok := nativeExe(lang)
			if !ok {
				t.Skip("TSG_NATIVE_EXE_DIR has no native executable for " + lang)
			}
			qfiles := queryFilesFor(t, root, lang)
			dir := t.TempDir()
			language := languages[lang]()
			var raws, full []*treesitter.Query
			var qpaths []string
			for _, qf := range qfiles {
				p := filepath.Join(dir, qf.name)
				if err := os.WriteFile(p, []byte(qf.source), 0o644); err != nil {
					t.Fatal(err)
				}
				qpaths = append(qpaths, p)
				orig, err := treesitter.NewQuery(language, qf.source)
				if err != nil {
					t.Fatalf("%s: NewQuery(original): %v", qf.name, err)
				}
				raw, err := treesitter.NewQuery(language, stripPredicates(qf.source))
				if err != nil {
					t.Fatalf("%s: NewQuery(stripped): %v", qf.name, err)
				}
				if raw.PatternCount() != orig.PatternCount() || strings.Join(raw.CaptureNames(), ",") != strings.Join(orig.CaptureNames(), ",") {
					t.Fatalf("%s: stripped query differs in patterns/captures", qf.name)
				}
				full, raws = append(full, orig), append(raws, raw)
			}
			type input struct {
				id, name string
				src      []byte
				sha      string
			}
			var inputs []input
			for _, r := range rows {
				if r[1] != lang {
					continue
				}
				name := filepath.Base(r[0])
				src := readFile(t, corpusFile(t, root, lang, name))
				if sha(src) != r[4] {
					t.Fatalf("%s: input sha256 differs from the manifest (identity drift)", r[0])
				}
				inputs = append(inputs, input{r[0], name, src, r[4]})
			}
			if len(inputs) == 0 {
				t.Skip("no corpus rows for " + lang)
			}
			// Native runs: independent processes, parallel.
			stride := probeStride()
			jobs := make([]nativeJob, 0, len(inputs)*len(qfiles))
			for i := range inputs {
				for qi := range qfiles {
					if qfiles[qi].probe() && i%stride != 0 {
						continue
					}
					jobs = append(jobs, nativeJob{qi: qi, input: i})
				}
			}
			files := make([]string, len(inputs))
			for i, in := range inputs {
				files[i] = filepath.Join(dir, "in-"+in.name)
				if err := os.WriteFile(files[i], in.src, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			start := time.Now()
			var wg sync.WaitGroup
			sem := make(chan struct{}, max(2, runtime.NumCPU()))
			for i := range jobs {
				wg.Add(1)
				sem <- struct{}{}
				go func(j *nativeJob) {
					defer wg.Done()
					defer func() { <-sem }()
					j.out, j.err = exec.Command(exe, "query", qpaths[j.qi], files[j.input]).Output()
				}(&jobs[i])
			}
			wg.Wait()
			nativeSecs := time.Since(start).Seconds()

			p := newParser(t, lang)
			var mism, matches, captures, compared, empty int
			perQuery := map[string][2]int{} // name -> [inputs with matches, matches]
			ji := 0
			for ii, in := range inputs {
				tree, err := p.Parse(bg, in.src, nil)
				if err != nil {
					t.Fatalf("%s: Parse: %v", in.id, err)
				}
				for qi, qf := range qfiles {
					if qf.probe() && ii%stride != 0 {
						continue
					}
					j := jobs[ji]
					ji++
					if j.err != nil {
						t.Fatalf("%s %s: native query: %v", in.id, qf.name, j.err)
					}
					got := goRawQuery(t, tree, raws[qi])
					compared++
					if !bytes.Equal(got, j.out) {
						mism++
						if mism <= 5 {
							t.Errorf("%s %s: raw matches differ from native: %s", in.id, qf.name, firstDiff(got, j.out))
						}
						preserve(fmt.Sprintf("query/%s/%s/%s.go.txt", lang, qf.name, in.name), got)
						preserve(fmt.Sprintf("query/%s/%s/%s.native.txt", lang, qf.name, in.name), j.out)
						continue
					}
					keys := rawMatchKeys(got)
					matches += len(keys)
					captures += strings.Count(string(got), "\nc\t")
					pq := perQuery[qf.name]
					if len(keys) > 0 {
						pq[0]++
					} else {
						empty++
					}
					pq[1] += len(keys)
					perQuery[qf.name] = pq
					// The filtered public result must be a subsequence of the raw one.
					var filtered bytes.Buffer
					for m, err := range tree.Matches(bg, full[qi], nil) {
						if err != nil {
							t.Fatalf("%s %s: filtered Matches: %v", in.id, qf.name, err)
						}
						fmt.Fprintf(&filtered, "m\t%d\t%d\n", m.Pattern, len(m.Captures))
						for _, c := range m.Captures {
							fmt.Fprintf(&filtered, "c\t%d\t%d\t%d\t%d\t%d\n", c.Index, c.Node.Index(), c.Node.Symbol(), c.Node.StartByte(), c.Node.EndByte())
						}
					}
					if fk := rawMatchKeys(append([]byte("h\n"), filtered.Bytes()...)); !isSubsequence(fk, keys) {
						mism++
						t.Errorf("%s %s: filtered matches are not a subsequence of the raw matches", in.id, qf.name)
					}
				}
				tree.Close()
			}
			names := make([]string, 0, len(perQuery))
			for k := range perQuery {
				names = append(names, k)
			}
			slices.Sort(names)
			var per []string
			for _, k := range names {
				per = append(per, k+"="+strconv.Itoa(perQuery[k][0])+"/"+strconv.Itoa(perQuery[k][1]))
			}
			summary(t, map[string]any{"lang": lang, "queries": len(qfiles), "inputs": len(inputs), "comparisons": compared,
				"probeStride": stride, "mismatches": mism, "matches": matches, "captures": captures, "emptyComparisons": empty,
				"perQueryInputsWithMatches/matches": strings.Join(per, " "), "nativeSeconds": nativeSecs,
				"queryDir": queryDirFor(root)})
			if mism != 0 {
				t.Fatalf("%d of %d comparisons differ from native", mism, compared)
			}
		})
	}
}
