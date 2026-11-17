package conformance

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	treesitter "github.com/wotjr1649/go-treesitter"
	proc "github.com/wotjr1649/go-treesitter/tools/bench/proc"
)

// Predicate comparison against the tree-sitter CLI (local only; CI compares native raw matches, see query_native_test.go).
// The CLI evaluates the text predicates (Rust), the Go side filters them itself, so the same query over the same bytes
// must give the same matches. The synthetic regex matrix records explicit syntax/Unicode residuals as E3 evidence.
//
//	TSG_TS_CLI      path of tree-sitter v0.27.2
//	TSG_TS_LIB_DIR  directory holding one shared library per grammar (built from its pinned parser.c + scanner.c)
//
// Without TSG_TS_CLI the test skips.

// cliLib maps a corpus language to the shared library file stem and the CLI --lang-name.
var cliLib = map[string][2]string{
	"postgres": {"postgres", "postgres"}, "kotlin": {"kotlin", "kotlin"}, "json": {"json", "json"}, "go": {"go", "go"}, "python": {"python", "python"},
	"typescript": {"typescript", "typescript"}, "tsx": {"tsx", "tsx"}, "csharp": {"c_sharp", "c_sharp"}, "tsql": {"mssql", "mssql"},
	"javascript": {"javascript", "javascript"},
	"html":       {"html", "html"},
	"css":        {"css", "css"},
	"java":       {"java", "java"},
	"c":          {"c", "c"},
	"rust":       {"rust", "rust"},
	"bash":       {"bash", "bash"},
	"cpp":        {"cpp", "cpp"},
	"yaml":       {"yaml", "yaml"},
	"php":        {"php", "php"},
	"powershell": {"powershell", "powershell"},
	"r":          {"r", "r"},
	"dart":       {"dart", "dart"},
	"xml":        {"xml", "xml"},
	"ruby":       {"ruby", "ruby"},
	"swift":      {"swift", "swift"},
}

// predicateSuite runs over the json corpus (the grammar with the simplest tree). Each entry states what it probes.
var predicateSuite = []queryFile{
	{"eq-string", `((string) @s (#eq? @s "\"a\""))`},
	{"not-eq-string", `((string) @s (#not-eq? @s "\"a\""))`},
	{"match", `((string) @s (#match? @s "^\"a"))`},
	{"not-match", `((string) @s (#not-match? @s "^\"a"))`},
	{"match-unanchored", `((string) @s (#match? @s "b"))`},
	{"any-of", `((string) @s (#any-of? @s "\"a\"" "\"b\"" "\"c\""))`},
	{"not-any-of", `((string) @s (#not-any-of? @s "\"a\"" "\"b\"" "\"c\""))`},
	{"eq-capture", `(pair key: (_) @k value: (_) @v (#eq? @k @v))`},
	{"not-eq-capture", `(pair key: (_) @k value: (_) @v (#not-eq? @k @v))`},
	{"quant-eq", `(array (_)+ @items (#eq? @items "1"))`},
	{"quant-not-eq", `(array (_)+ @items (#not-eq? @items "1"))`},
	{"quant-match", `(array (_)+ @items (#match? @items "^[0-9]+$"))`},
	{"quant-not-match", `(array (_)+ @items (#not-match? @items "^[0-9]+$"))`},
	{"quant-any-of", `(array (_)+ @items (#any-of? @items "1" "2"))`},
	{"quant-not-any-of", `(array (_)+ @items (#not-any-of? @items "1" "2"))`},
	{"star-eq", `(array (_)* @items (#eq? @items "1"))`},
	{"star-not-eq", `(array (_)* @items (#not-eq? @items "1"))`},
	{"opt-eq", `(array (number)? @x (#eq? @x "1"))`},
	{"opt-not-eq", `(array (number)? @x (#not-eq? @x "1"))`},
	{"opt-any-of", `(array (number)? @x (#any-of? @x "1"))`},
	{"quant-eq-capture", `(array (number)+ @a (string)+ @b (#eq? @a @b))`},
	{"quant-not-eq-capture", `(array (number)+ @a (number)+ @b (#not-eq? @a @b))`},
	{"qg-eq", `(array ((number) @items ","?)+ (#eq? @items "1"))`},
	{"qg-not-eq", `(array ((number) @items ","?)+ (#not-eq? @items "1"))`},
	{"qg-any-eq", `(array ((number) @items ","?)+ (#any-eq? @items "1"))`},
	{"qg-any-not-eq", `(array ((number) @items ","?)+ (#any-not-eq? @items "1"))`},
	{"qg-match", `(array ((number) @items ","?)+ (#match? @items "^[12]$"))`},
	{"qg-not-match", `(array ((number) @items ","?)+ (#not-match? @items "^[12]$"))`},
	{"qg-any-match", `(array ((number) @items ","?)+ (#any-match? @items "^[12]$"))`},
	{"qg-any-not-match", `(array ((number) @items ","?)+ (#any-not-match? @items "^[12]$"))`},
	{"qg-any-of", `(array ((number) @items ","?)+ (#any-of? @items "1" "2"))`},
	{"qg-not-any-of", `(array ((number) @items ","?)+ (#not-any-of? @items "1" "2"))`},
	{"qg-eq-capture", `(array ((number) @a ","?)+ ((number) @b ","?)* (#eq? @a @b))`},
	{"qg-not-eq-capture", `(array ((number) @a ","?)+ ((number) @b ","?)* (#not-eq? @a @b))`},
	{"any-eq", `(array (_)+ @items (#any-eq? @items "1"))`},
	{"any-not-eq", `(array (_)+ @items (#any-not-eq? @items "1"))`},
	{"any-match", `(array (_)+ @items (#any-match? @items "^[0-9]+$"))`},
	{"any-not-match", `(array (_)+ @items (#any-not-match? @items "^[0-9]+$"))`},
	{"any-eq-capture", `(array (_)+ @a (_)+ @b (#any-eq? @a @b))`},
	{"two-predicates", `((string) @s (#match? @s "^\"") (#not-eq? @s "\"a\""))`},
	{"alt-predicate", `[((string) @s (#eq? @s "\"a\"")) ((number) @s (#eq? @s "1"))]`},
	{"re-unicode-word", `((string) @s (#match? @s "^\"\\w+\"$"))`},
	{"re-unicode-digit", `((string) @s (#match? @s "\\d"))`},
	{"re-case-insensitive", `((string) @s (#match? @s "(?i)^\"A"))`},
	{"re-unicode-class", `((string) @s (#match? @s "\\p{L}"))`},
	{"re-word-boundary", `((string) @s (#match? @s "\\bx\\b"))`},
	{"set-directive", `((string) @s (#set! kind "str"))`},
	{"is-assertion", `((string) @s (#is? local))`},
}

// acceptProbes are query sources whose acceptance (not their matches) is compared: the CLI compiles with the Rust
// binding, the Go side with NewQuery.
var acceptProbes = []queryFile{
	{"literal-first-arg", `((string) @s (#eq? "a" @s))`},
	{"eq-one-arg", `((string) @s (#eq? @s))`},
	{"eq-three-args", `((string) @s (#eq? @s "a" "b"))`},
	{"match-capture-regex", `((string) @s (#match? @s @s))`},
	{"match-bad-regex", `((string) @s (#match? @s "("))`},
	{"match-lookahead", `((string) @s (#match? @s "a(?=b)"))`},
	{"any-of-one-arg", `((string) @s (#any-of? @s))`},
	{"any-of-capture-arg", `((string) @s (#any-of? @s @s))`},
	{"set-capture-arg", `((string) @s (#set! @s "k" "v"))`},
	{"set-three-args", `((string) @s (#set! "k" "v" "w"))`},
	{"set-no-args", `((string) @s (#set!))`},
	{"is-capture-arg", `((string) @s (#is? @s local))`},
	{"is-no-args", `((string) @s (#is?))`},
	{"undefined-capture", `((string) (#eq? @s "a"))`},
	{"unknown-predicate", `((string) @s (#nonesuch? @s "a"))`},
	{"unknown-directive", `((string) @s (#nonesuch! @s "a"))`},
	{"not-eq-capture-first-literal", `((string) @s (#not-eq? "a" "b"))`},
	{"any-match-two-captures", `((string) @s (#any-match? @s @s))`},
	{"not-any-of-ok", `((string) @s (#not-any-of? @s "a"))`},
	{"any-not-any-of", `((string) @s (#any-not-any-of? @s "a"))`},
}

// cliCapture.idx is -1 where the CLI does not print it; String leaves it out then and the comparison drops it on both sides.
type cliCapture struct {
	idx            int
	name           string
	sr, sc, er, ec int
}

func (c cliCapture) String() string {
	return fmt.Sprintf("%s(%d,%d)-(%d,%d)", c.name, c.sr, c.sc, c.er, c.ec)
}

func renderMatch(pattern int, caps []cliCapture) string {
	var parts []string
	for _, c := range caps {
		parts = append(parts, c.String())
	}
	return fmt.Sprintf("p%d [%s]", pattern, strings.Join(parts, " "))
}

var (
	cliPattern = regexp.MustCompile(`^  pattern: (\d+)$`)
	// Multi-line captures are printed as "capture: <name>, start, end" (no index, no text); the others with " - " and text.
	cliCapLine = regexp.MustCompile("^    capture: (?:(\\d+) - )?([^,]*), start: \\((\\d+), (\\d+)\\), end: \\((\\d+), (\\d+)\\)(, text: `)?")
)

// parseCLI parses `tree-sitter query` output (match order). Capture text, which may span lines, is skipped up to the
// closing backtick that precedes a recognizable line.
func parseCLI(out []byte, srcs map[string][]byte) (map[string][]string, error) {
	got := map[string][]string{}
	var cur string
	pos := 0
	var curPattern = -1
	var caps []cliCapture
	flush := func() {
		if curPattern >= 0 {
			got[cur] = append(got[cur], renderMatch(curPattern, caps))
		}
		curPattern, caps = -1, nil
	}
	next := func() (string, bool) {
		if pos >= len(out) {
			return "", false
		}
		i := bytes.IndexByte(out[pos:], '\n')
		var line string
		if i < 0 {
			line, pos = string(out[pos:]), len(out)
		} else {
			line, pos = string(out[pos:pos+i]), pos+i+1
		}
		return strings.TrimSuffix(line, "\r"), true
	}
	for {
		line, ok := next()
		if !ok {
			break
		}
		switch {
		case line == "":
		case line[0] != ' ':
			flush()
			cur = filepath.Base(line)
			if _, ok := srcs[cur]; !ok {
				return nil, fmt.Errorf("output names unknown input %q", line)
			}
			got[cur] = got[cur][:0:0]
		case cliPattern.MatchString(line):
			flush()
			curPattern, _ = strconv.Atoi(cliPattern.FindStringSubmatch(line)[1])
		case cliCapLine.MatchString(line):
			m := cliCapLine.FindStringSubmatch(line)
			c := cliCapture{name: m[2]}
			if m[1] != "" {
				c.idx, _ = strconv.Atoi(m[1])
			} else {
				c.idx = -1 // not printed for multi-line captures: compared by name only
			}
			c.sr, _ = strconv.Atoi(m[3])
			c.sc, _ = strconv.Atoi(m[4])
			c.er, _ = strconv.Atoi(m[5])
			c.ec, _ = strconv.Atoi(m[6])
			caps = append(caps, c)
			if m[7] == "" { // no text printed
				continue
			}
			if rest := line[len(m[0]):]; strings.HasSuffix(rest, "`") {
				continue
			}
			// multi-line text: consume lines until one ends with the closing backtick and the next line is a known prefix
			for {
				l, ok := next()
				if !ok {
					break
				}
				if strings.HasSuffix(l, "`") {
					save := pos
					peek, ok2 := next()
					pos = save
					if !ok2 || peek == "" || peek[0] != ' ' || cliPattern.MatchString(peek) || cliCapLine.MatchString(peek) {
						break
					}
				}
			}
		default:
			return nil, fmt.Errorf("unrecognised CLI output line %q", line)
		}
	}
	flush()
	return got, nil
}

func cliEnv(home string) []string {
	return append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "XDG_CONFIG_HOME="+home, "XDG_DATA_HOME="+home, "XDG_CACHE_HOME="+home)
}

func TestQueryCLI(t *testing.T) {
	cli := os.Getenv("TSG_TS_CLI")
	if cli == "" {
		t.Skip("TSG_TS_CLI (tree-sitter v0.27.2) is not set")
	}
	libDir := os.Getenv("TSG_TS_LIB_DIR")
	if libDir == "" {
		t.Fatal("TSG_TS_LIB_DIR is required with TSG_TS_CLI")
	}
	root := dataRoot(t)
	rows := readCSV(t, filepath.Join(root, "corpus", "manifest.csv"))
	home := t.TempDir()
	ext := ".so"
	switch runtime.GOOS {
	case "windows":
		ext = ".dll"
	case "darwin":
		ext = ".dylib"
	}
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			lib := filepath.Join(libDir, cliLib[lang][0]+ext)
			if _, err := os.Stat(lib); err != nil {
				t.Skip("no shared library for " + lang + ": " + lib)
			}
			qfiles := queryFilesFor(t, root, lang)[:0]
			for _, qf := range queryFilesFor(t, root, lang) {
				if strings.HasSuffix(qf.name, ".scm") {
					qfiles = append(qfiles, qf)
				}
			}
			if lang == "json" {
				qfiles = append(qfiles, predicateSuite...)
			}
			dir := t.TempDir()
			srcs := map[string][]byte{}
			var paths []string
			var order []string
			for _, r := range rows {
				if r[1] != lang {
					continue
				}
				name := filepath.Base(r[0])
				src := readFile(t, corpusFile(t, root, lang, name))
				fn := "in-" + name
				p := filepath.Join(dir, fn)
				if err := os.WriteFile(p, src, 0o644); err != nil {
					t.Fatal(err)
				}
				srcs[fn] = src
				paths = append(paths, p)
				order = append(order, fn)
			}
			if len(paths) == 0 {
				t.Skip("no corpus rows for " + lang)
			}
			list := filepath.Join(dir, "paths.txt")
			os.WriteFile(list, []byte(strings.Join(paths, "\n")+"\n"), 0o644)
			language := languages[lang]()
			parser := newParser(t, lang)
			trees := map[string]*treesitter.Tree{}
			for _, fn := range order {
				tree, err := parser.Parse(bg, srcs[fn], nil)
				if err != nil {
					t.Fatal(err)
				}
				trees[fn] = tree
			}
			defer func() {
				for _, tr := range trees {
					tr.Close()
				}
			}()
			var diffs, compared, matches int
			var diffQueries []string
			for _, qf := range qfiles {
				qpath := filepath.Join(dir, qf.name+".scm")
				os.WriteFile(qpath, []byte(qf.source), 0o644)
				cmd := proc.Command(cli, "query", "-l", lib, "--lang-name", cliLib[lang][1], "--paths", list, qpath)
				cmd.Env = cliEnv(home)
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				out, err := cmd.Output()
				q, qerr := treesitter.NewQuery(language, qf.source)
				if err != nil {
					t.Errorf("%s: CLI failed: %v %s (Go NewQuery err=%v)", qf.name, err, strings.TrimSpace(stderr.String()), qerr)
					diffs++
					continue
				}
				if qerr != nil {
					t.Errorf("%s: CLI accepted the query, NewQuery rejected it: %v", qf.name, qerr)
					diffs++
					continue
				}
				want, perr := parseCLI(out, srcs)
				if perr != nil {
					t.Fatalf("%s: %v", qf.name, perr)
				}
				names := q.CaptureNames()
				qdiffs := 0
				for _, fn := range order {
					var got []string
					for m, err := range trees[fn].Matches(bg, q, nil) {
						if err != nil {
							t.Fatalf("%s %s: %v", qf.name, fn, err)
						}
						var caps []cliCapture
						for _, c := range m.Captures {
							sp, ep := c.Node.StartPoint(), c.Node.EndPoint()
							caps = append(caps, cliCapture{int(c.Index), names[c.Index], int(sp.Row), int(sp.Column), int(ep.Row), int(ep.Column)})
						}
						got = append(got, renderMatch(m.Pattern, caps))
					}
					compared++
					matches += len(got)
					if !slices.Equal(got, want[fn]) {
						qdiffs++
						diffs++
						if qdiffs <= 3 {
							t.Errorf("%s %s: Go %d matches, CLI %d; first difference: %s", qf.name, fn, len(got), len(want[fn]), firstListDiff(got, want[fn]))
						}
						preserve(fmt.Sprintf("cli/%s/%s/%s.go.txt", lang, qf.name, fn), []byte(strings.Join(got, "\n")+"\n"))
						preserve(fmt.Sprintf("cli/%s/%s/%s.cli.txt", lang, qf.name, fn), []byte(strings.Join(want[fn], "\n")+"\n"))
					}
				}
				if qdiffs > 0 {
					diffQueries = append(diffQueries, fmt.Sprintf("%s=%d", qf.name, qdiffs))
				}
			}
			summary(t, map[string]any{"lang": lang, "queries": len(qfiles), "inputs": len(order), "comparisons": compared,
				"goMatches": matches, "differences": diffs, "differingQueries(inputs)": strings.Join(diffQueries, " ")})
		})
	}
	t.Run("accept", func(t *testing.T) {
		lib := filepath.Join(libDir, "json"+ext)
		if _, err := os.Stat(lib); err != nil {
			t.Skip("no json library")
		}
		dir := t.TempDir()
		src := filepath.Join(dir, "a.json")
		os.WriteFile(src, []byte(`["a"]`), 0o644)
		language := languages["json"]
		if language == nil {
			t.Skip("json is not linked")
		}
		var rows []string
		for _, qf := range acceptProbes {
			qpath := filepath.Join(dir, qf.name+".scm")
			os.WriteFile(qpath, []byte(qf.source), 0o644)
			cmd := proc.Command(cli, "query", "-l", lib, "--lang-name", "json", qpath, src)
			cmd.Env = cliEnv(home)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			_, cerr := cmd.Output()
			_, gerr := treesitter.NewQuery(language(), qf.source)
			verdict := "same"
			if (cerr == nil) != (gerr == nil) {
				verdict = "DIFFERENT"
			}
			first := strings.SplitN(strings.TrimSpace(stderr.String()), "\n", 2)[0]
			rows = append(rows, fmt.Sprintf("%s | cli=%v | go=%v | %s | cliMsg=%q", qf.name, cerr == nil, gerr == nil, verdict, first))
			if verdict != "same" {
				t.Errorf("acceptance differs for %s %s: CLI ok=%v (%s), Go err=%v", qf.name, qf.source, cerr == nil, first, gerr)
			}
		}
		t.Log("\n" + strings.Join(rows, "\n"))
		preserve("cli/accept.txt", []byte(strings.Join(rows, "\n")+"\n"))
	})
}

func firstListDiff(got, want []string) string {
	for i := 0; i < max(len(got), len(want)); i++ {
		var a, b string
		if i < len(got) {
			a = got[i]
		}
		if i < len(want) {
			b = want[i]
		}
		if a != b {
			return fmt.Sprintf("#%d go=%q cli=%q", i, a, b)
		}
	}
	return "identical"
}

func TestQueryCLIInvalidUTF8Output(t *testing.T) {
	// CLI 0.27.2 omits invalid UTF-8 capture text, retaining the byte span.
	out := []byte("bad.json\n  pattern: 0\n    capture: 0 - s, start: (0, 0), end: (0, 3), text: ``\n")
	got, err := parseCLI(out, map[string][]byte{"bad.json": {'"', 0xff, '"'}})
	if err != nil || !slices.Equal(got["bad.json"], []string{"p0 [s(0,0)-(0,3)]"}) {
		t.Fatalf("invalid UTF-8 capture: %v, %v", got, err)
	}
}

// TestQueryCLIRegexMatrix needs only the local CLI and json library, not a native corpus data directory.
func TestQueryCLIRegexMatrix(t *testing.T) {
	cli := os.Getenv("TSG_TS_CLI")
	if cli == "" {
		t.Skip("TSG_TS_CLI (tree-sitter v0.27.2) is not set")
	}
	libDir := os.Getenv("TSG_TS_LIB_DIR")
	if libDir == "" {
		t.Fatal("TSG_TS_LIB_DIR is required with TSG_TS_CLI")
	}
	if languages["json"] == nil {
		t.Skip("json is not linked")
	}
	ext := ".so"
	if runtime.GOOS == "windows" {
		ext = ".dll"
	} else if runtime.GOOS == "darwin" {
		ext = ".dylib"
	}
	lib := filepath.Join(libDir, "json"+ext)
	data := readFile(t, filepath.Join("..", "..", "testdata", "query-regex-cli.json"))
	var fixture struct {
		CLIVersion    string `json:"cli_version"`
		CLISHA256     string `json:"cli_sha256"`
		LibrarySHA256 string `json:"json_library_sha256"`
		Inputs        []struct{ Name, Base64 string }
		Patterns      []struct {
			Name        string
			Pattern     string `json:"pattern_base64"`
			Matches     []bool
			GoOverrides map[string]bool `json:"go_overrides"`
			Residual    string
		}
		Acceptance []struct {
			Name      string
			Pattern   string `json:"pattern_base64"`
			CLIAccept bool   `json:"cli_accept"`
			GoReject  string `json:"go_reject"`
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	checkName := func(category, name string) {
		key := category + "/" + strings.ToLower(name)
		if seen[key] {
			t.Fatalf("case-insensitive fixture filename collision: %s", key)
		}
		seen[key] = true
	}
	for _, input := range fixture.Inputs {
		checkName("input", input.Name)
	}
	for _, row := range fixture.Patterns {
		checkName("pattern", row.Name)
	}
	for _, probe := range fixture.Acceptance {
		checkName("accept", probe.Name)
	}
	if sha(readFile(t, cli)) != fixture.CLISHA256 || sha(readFile(t, lib)) != fixture.LibrarySHA256 {
		t.Fatal("CLI or json library SHA-256 differs from recorded fixture")
	}
	home, dir := t.TempDir(), t.TempDir()
	run := func(t *testing.T, args ...string) ([]byte, []byte, error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(bg, 15*time.Second)
		defer cancel()
		cmd := proc.CommandContext(ctx, cli, args...)
		cmd.Env = cliEnv(home)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if ctx.Err() != nil {
			t.Fatalf("CLI timed out: %v", args)
		}
		return out, stderr.Bytes(), err
	}
	version, stderr, err := run(t, "--version")
	if err != nil || strings.TrimSpace(string(version)) != fixture.CLIVersion {
		t.Fatalf("CLI version: %q, %v, %s", version, err, stderr)
	}
	decode := func(t *testing.T, value string) []byte {
		t.Helper()
		b, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	srcs := map[string][]byte{}
	trees := map[string]*treesitter.Tree{}
	var paths []string
	parser := newParser(t, "json")
	for _, input := range fixture.Inputs {
		name := input.Name + ".json"
		src := decode(t, input.Base64)
		srcs[name] = src
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, src, 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
		tree, err := parser.Parse(bg, src, nil)
		if err != nil {
			t.Fatal(err)
		}
		trees[name] = tree
		t.Cleanup(tree.Close)
	}
	list := filepath.Join(dir, "paths.txt")
	if err := os.WriteFile(list, []byte(strings.Join(paths, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	query := func(t *testing.T, name, pattern string, all bool) ([]byte, []byte, error, *treesitter.Query, error) {
		t.Helper()
		source := "((document) @s (#match? @s " + strconv.Quote(pattern) + "))"
		path := filepath.Join(dir, name+".scm")
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		args := []string{"query", "-l", lib, "--lang-name", "json"}
		if all {
			args = append(args, "--paths", list, path)
		} else {
			args = append(args, path, paths[0])
		}
		out, stderr, cerr := run(t, args...)
		q, gerr := treesitter.NewQuery(languages["json"](), source)
		return out, stderr, cerr, q, gerr
	}
	var comparisons, residuals int
	for _, row := range fixture.Patterns {
		t.Run(row.Name, func(t *testing.T) {
			out, stderr, cerr, q, gerr := query(t, "matrix-"+row.Name, string(decode(t, row.Pattern)), true)
			preserve("cli/regex/"+row.Name+".stdout", out)
			preserve("cli/regex/"+row.Name+".stderr", stderr)
			if cerr != nil || gerr != nil {
				t.Fatalf("CLI=%v %s Go=%v", cerr, stderr, gerr)
			}
			matches, err := parseCLI(out, srcs)
			if err != nil {
				t.Fatal(err)
			}
			if len(row.Matches) != len(fixture.Inputs) {
				t.Fatal("incomplete fixture matrix")
			}
			for i, input := range fixture.Inputs {
				name := input.Name + ".json"
				cliMatch := len(matches[name]) != 0
				if cliMatch != row.Matches[i] {
					t.Errorf("%s: CLI=%v recorded=%v", input.Name, cliMatch, row.Matches[i])
				}
				goMatch := false
				for _, err := range trees[name].Matches(bg, q, nil) {
					if err != nil {
						t.Fatal(err)
					}
					goMatch = true
				}
				want := cliMatch
				if override, ok := row.GoOverrides[input.Name]; ok {
					if row.Residual == "" {
						t.Fatal("unexplained residual")
					}
					want = override
					if row.Name == "property-L" && input.Name == "unicode17-new-letter" {
						want = unicode.IsLetter('\U00010940')
					}
					if want != cliMatch {
						residuals++
						t.Logf("residual %s: CLI=%v Go=%v: %s", input.Name, cliMatch, want, row.Residual)
					}
				}
				comparisons++
				if goMatch != want {
					t.Errorf("%s: CLI=%v Go=%v expected Go=%v residual=%q", input.Name, cliMatch, goMatch, want, row.Residual)
				}
			}
		})
	}
	for _, probe := range fixture.Acceptance {
		t.Run("accept/"+probe.Name, func(t *testing.T) {
			out, stderr, cerr, _, gerr := query(t, "accept-"+probe.Name, string(decode(t, probe.Pattern)), false)
			preserve("cli/regex/accept-"+probe.Name+".stdout", out)
			preserve("cli/regex/accept-"+probe.Name+".stderr", stderr)
			if (cerr == nil) != probe.CLIAccept || (gerr != nil) != (probe.GoReject != "") {
				t.Errorf("CLI accepted=%v (recorded=%v) %s; Go rejection=%v (expected=%q)", cerr == nil, probe.CLIAccept, stderr, gerr, probe.GoReject)
			}
			if probe.CLIAccept != (probe.GoReject == "") {
				residuals++
				t.Logf("acceptance residual: CLI accepts=%v; Go accepts=%v; Go rejection=%q", probe.CLIAccept, probe.GoReject == "", probe.GoReject)
			}
		})
	}
	t.Logf("E3 recorded matrix: %d patterns x %d inputs = %d comparisons; %d acceptance probes; %d explicit residuals", len(fixture.Patterns), len(fixture.Inputs), comparisons, len(fixture.Acceptance), residuals)
}
