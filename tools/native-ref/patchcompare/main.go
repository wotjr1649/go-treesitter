// Command patchcompare compares a pinned grammar's native dumps before and after
// an ADR-0023 patch and retains every input and both dumps.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wotjr1649/go-treesitter/tools/bench/proc"
)

type input struct {
	ID    string
	Bytes []byte
}
type tree struct {
	HasError   bool   `json:"has_error"`
	End        uint64 `json:"root_end"`
	DumpSHA256 string `json:"dump_sha256"`
}
type result struct {
	ID      string `json:"id"`
	Bytes   int    `json:"bytes"`
	SHA256  string `json:"sha256"`
	Before  tree   `json:"before"`
	After   tree   `json:"after"`
	Changed bool   `json:"changed"`
	NULFix  bool   `json:"documented_nul_fix"`
	Pass    bool   `json:"pass"`
}
type report struct {
	BeforeSHA256       string   `json:"before_exe_sha256"`
	AfterSHA256        string   `json:"after_exe_sha256"`
	FuzzSourceSHA256   string   `json:"fuzz_source_sha256"`
	FuzzTarget         string   `json:"fuzz_target"`
	AllowNULTruncation bool     `json:"allow_nul_truncation"`
	Pass               bool     `json:"pass"`
	Results            []result `json:"results"`
}

func sha(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func main() {
	before := flag.String("before", "", "unpatched native executable")
	after := flag.String("after", "", "patched native executable")
	corpus := flag.String("corpus", "", "directory of corpus .in files")
	source := flag.String("fuzz-source", "", "Go source containing the fuzz target's literal seeds")
	target := flag.String("fuzz-target", "", "fuzz function (for example FuzzParseYAML)")
	allowNUL := flag.Bool("allow-nul-truncation", false, "allow clean NUL-truncated trees to become full-length error trees (YAML 0001)")
	out := flag.String("out", "", "new output directory retaining inputs, dumps and report.json")
	build := flag.String("scanner-build", "", "native-ref manifest for scanner patch comparisons")
	expectations := flag.String("expectations", "", "read-only scanner expectations sidecar")
	regressions := flag.String("regressions", "", "conformance testdata directory containing shared scanner inputs")
	candidate := flag.String("write-candidate", "", "write an unreviewed candidate sidecar to a new file")
	flag.Parse()
	if *build != "" {
		if *before != "" || *after != "" || *target != "" || *allowNUL || flag.NArg() != 0 {
			fail(fmt.Errorf("scanner mode and legacy comparison flags are mutually exclusive"))
		}
		if err := runScanner(*build, *expectations, *candidate, *corpus, *source, *regressions, *out); err != nil {
			fail(err)
		}
		return
	}
	if *expectations != "" || *regressions != "" || *candidate != "" {
		fail(fmt.Errorf("-expectations, -regressions and -write-candidate require -scanner-build"))
	}
	if *before == "" || *after == "" || *corpus == "" || *source == "" || *target == "" || *out == "" {
		fail(fmt.Errorf("-before, -after, -corpus, -fuzz-source, -fuzz-target and -out are required"))
	}
	ok, err := run(*before, *after, *corpus, *source, *target, *out, *allowNUL)
	if err != nil {
		fail(err)
	}
	if !ok {
		os.Exit(1)
	}
}
func fail(err error) { fmt.Fprintln(os.Stderr, "patchcompare:", err); os.Exit(2) }
func run(before, after, corpus, source, target, out string, allowNUL bool) (bool, error) {
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		return false, fmt.Errorf("output must not exist: %s", out)
	}
	inputs, err := corpusInputs(corpus)
	if err != nil {
		return false, err
	}
	fuzz, e := os.ReadFile(source)
	if e != nil {
		return false, e
	}
	seeds, e := fuzzSeeds(source, fuzz, target)
	if e != nil {
		return false, e
	}
	inputs = appendFuzzInputs(inputs, seeds)
	var r report
	r.Pass = true
	r.FuzzTarget = target
	r.FuzzSourceSHA256 = sha(fuzz)
	r.AllowNULTruncation = allowNUL
	for i, p := range []string{before, after} {
		b, e := os.ReadFile(p)
		if e != nil {
			return false, e
		}
		if i == 0 {
			r.BeforeSHA256 = sha(b)
		} else {
			r.AfterSHA256 = sha(b)
		}
	}
	for _, in := range inputs {
		p := filepath.Join(out, "inputs", filepath.FromSlash(in.ID)+".in")
		if e := write(p, in.Bytes); e != nil {
			return false, e
		}
		var dumps [2][]byte
		for i, exe := range []string{before, after} {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			cmd := proc.CommandContext(ctx, exe, "dump", p)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			b, e := cmd.Output()
			cancel()
			kind := "before"
			if i == 1 {
				kind = "after"
			}
			if er := write(filepath.Join(out, kind, filepath.FromSlash(in.ID)+".dump"), b); er != nil {
				return false, er
			}
			if e != nil {
				return false, fmt.Errorf("%s %s: %w: %s", in.ID, kind, e, stderr.Bytes())
			}
			dumps[i] = b
		}
		row, e := compare(in, dumps[0], dumps[1], allowNUL)
		if e != nil {
			return false, e
		}
		r.Results = append(r.Results, row)
		r.Pass = r.Pass && row.Pass
	}
	b, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return false, e
	}
	if e = write(filepath.Join(out, "report.json"), append(b, '\n')); e != nil {
		return false, e
	}
	changed, exceptions := 0, 0
	for _, v := range r.Results {
		if v.Changed {
			changed++
		}
		if v.NULFix {
			exceptions++
		}
	}
	fmt.Printf("inputs=%d changed=%d documented_nul_fixes=%d pass=%t report=%s\n", len(r.Results), changed, exceptions, r.Pass, filepath.Join(out, "report.json"))
	return r.Pass, nil
}
func write(p string, b []byte) error {
	if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
		return e
	}
	return os.WriteFile(p, b, 0644)
}
func corpusInputs(corpus string) ([]input, error) {
	var inputs []input
	err := filepath.WalkDir(corpus, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.Type().IsRegular() || !strings.HasSuffix(p, ".in") {
			return nil
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(corpus, p)
		if e != nil {
			return e
		}
		inputs = append(inputs, input{"corpus/" + filepath.ToSlash(strings.TrimSuffix(rel, ".in")), b})
		return nil
	})
	if err == nil && len(inputs) == 0 {
		err = fmt.Errorf("no corpus .in files in %s", corpus)
	}
	return inputs, err
}

func appendFuzzInputs(inputs []input, seeds [][]byte) []input {
	for i, b := range seeds {
		id := "fuzz/" + strconv.Itoa(i)
		inputs = append(inputs, input{id, b})
		for j, c := range []struct {
			start, del int
			repl       []byte
		}{{0, 0, []byte("x")}, {len(b) / 2, 2, []byte(" 1 ")}, {len(b), 0, []byte("\n")}} {
			oldEnd := c.start + c.del%(len(b)-c.start+1)
			next := append(append(append([]byte{}, b[:c.start]...), c.repl...), b[oldEnd:]...)
			inputs = append(inputs, input{fmt.Sprintf("%s-edit-%d", id, j), next})
		}
	}
	return inputs
}
func parseTree(b []byte) (tree, error) {
	lines := bytes.Split(b, []byte{'\n'})
	if len(lines) < 3 || !bytes.HasPrefix(lines[0], []byte("#tsg-dump/1 root_has_error=")) {
		return tree{}, fmt.Errorf("invalid native dump header")
	}
	fields := bytes.Split(lines[1], []byte{'\t'})
	if len(fields) != 12 {
		return tree{}, fmt.Errorf("invalid native root row")
	}
	end, e := strconv.ParseUint(string(fields[9]), 10, 32)
	if e != nil {
		return tree{}, e
	}
	return tree{bytes.Contains(lines[0], []byte("root_has_error=1")), end, sha(b)}, nil
}
func compare(in input, before, after []byte, allowNUL bool) (result, error) {
	b, e := parseTree(before)
	if e != nil {
		return result{}, fmt.Errorf("%s before: %w", in.ID, e)
	}
	a, e := parseTree(after)
	if e != nil {
		return result{}, fmt.Errorf("%s after: %w", in.ID, e)
	}
	changed := !bytes.Equal(before, after)
	nul := bytes.IndexByte(in.Bytes, 0)
	nulFix := allowNUL && nul >= 0 && !b.HasError && b.End == uint64(nul) && a.HasError && a.End == uint64(len(in.Bytes))
	return result{in.ID, len(in.Bytes), sha(in.Bytes), b, a, changed, nulFix, a.End == uint64(len(in.Bytes)) && (b.HasError || !changed || nulFix)}, nil
}
func fuzzSeeds(name string, b []byte, target string) ([][]byte, error) {
	f, e := parser.ParseFile(token.NewFileSet(), name, b, 0)
	if e != nil {
		return nil, e
	}
	var seeds [][]byte
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != target {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if e != nil {
				return false
			}
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			id, ok := c.Fun.(*ast.Ident)
			if !ok || id.Name != "fuzzParse" {
				return true
			}
			if len(c.Args) < 2 {
				e = fmt.Errorf("%s uses an incomplete fuzzParse call", target)
				return false
			}
			for _, a := range c.Args[2:] {
				s, ok := a.(*ast.BasicLit)
				if !ok || s.Kind != token.STRING {
					e = fmt.Errorf("%s uses a nonliteral fuzz seed", target)
					return false
				}
				var v string
				v, e = strconv.Unquote(s.Value)
				if e != nil {
					return false
				}
				seeds = append(seeds, []byte(v))
			}
			return false
		})
	}
	if e != nil {
		return nil, e
	}
	if len(seeds) == 0 {
		return nil, fmt.Errorf("no fuzzParse literal seeds found in %s", target)
	}
	return seeds, nil
}
