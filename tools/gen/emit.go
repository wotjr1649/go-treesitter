package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/wotjr1649/go-treesitter/tools/bench/proc"
	"github.com/wotjr1649/go-treesitter/tools/gen/grammarpatch"
	"github.com/wotjr1649/go-treesitter/tools/gen/patches"
)

// Identity mirrors engine.Identity (field names and JSON keys must stay identical: the module test unmarshals
// provenance.json into engine.Identity and compares it with the compiled one).
type Identity struct {
	Runtime      string
	GrammarRepo  string
	GrammarRev   string
	SourceSHA256 map[string]string
	RecipeSHA256 string
	Tools        map[string]string
}

type provenance struct {
	Patches           []patches.Pin            `json:"patches,omitempty"`
	GrammarGeneration *grammarpatch.Provenance `json:"grammarGeneration,omitempty"`
	Schema            int                      `json:"schema"`
	ABI               uint32                   `json:"abi"`
	Name              string                   `json:"name"`
	Module            string                   `json:"module"`
	Identity          Identity                 `json:"identity"`
	Outputs           map[string]string        `json:"outputs"` // sha256 of generated artifacts
	TestOnly          string                   `json:"testOnly,omitempty"`
}

func marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	e.SetIndent("", "  ")
	err := e.Encode(v)
	return b.Bytes(), err
}

// ---- memory_grow replacement (ADR-0020 §8) ----

const replacementBody = "func memory_grow(mem *[]byte, delta, max int64) int64 {\n\treturn engine.MemoryGrow(mem, delta, max)\n}\n"

var (
	memoryGrowRx = regexp.MustCompile(`(?ms)^func memory_grow\(mem \*\[\]byte, delta, max int64\) int64 \{\n.*?^\}\n`)
	importRx     = regexp.MustCompile(`(?s)\nimport \(\n(.*?)\n\)\n`)
	ifaceRx      = regexp.MustCompile(`(?m)^type X(\w+) = interface \{\n((?:\t.*\n)*?)\}`)
	methodRx     = regexp.MustCompile(`(?m)^\t(X\w+)\(`)
)

const enginePath = "github.com/wotjr1649/go-treesitter/internal/engine"

// patchMemoryGrow replaces the generated memory_grow only if the original text matches the pinned sha256.
func patchMemoryGrow(src, wantSHA string) (string, error) {
	locs := memoryGrowRx.FindAllStringIndex(src, -1)
	if len(locs) != 1 {
		return "", fmt.Errorf("memory_grow: found %d definitions, want 1", len(locs))
	}
	orig := src[locs[0][0]:locs[0][1]]
	if got := sumBytes([]byte(orig)); got != wantSHA {
		return "", fmt.Errorf("memory_grow: original body sha256 %s, pinned %s (wasm2go output changed)", got, wantSHA)
	}
	src = src[:locs[0][0]] + replacementBody + src[locs[0][1]:]
	m := importRx.FindStringSubmatchIndex(src)
	if m == nil {
		return "", fmt.Errorf("import block not found")
	}
	lines := strings.Split(src[m[2]:m[3]], "\n")
	q := "\t" + strconv.Quote(enginePath)
	i := sort.Search(len(lines), func(i int) bool { return strings.TrimSpace(lines[i]) > strconv.Quote(enginePath) })
	lines = append(lines[:i], append([]string{q}, lines[i:]...)...)
	return src[:m[2]] + strings.Join(lines, "\n") + src[m[3]:], nil
}

// checkImports fails if the module needs an import the engine host does not stub (engine.Host).
func checkImports(src string) error {
	allowed := map[string]map[string]bool{
		"tsg":                    {"Xshould_stop": true},
		"wasi_snapshot_preview1": {"Xclock_time_get": true, "Xfd_close": true, "Xfd_seek": true, "Xfd_write": true, "Xproc_exit": true},
	}
	for _, m := range ifaceRx.FindAllStringSubmatch(src, -1) {
		mod := m[1]
		if mod == "Memory" {
			continue
		}
		al, ok := allowed[mod]
		if !ok {
			return fmt.Errorf("import module %q is not stubbed by engine.Host", mod)
		}
		for _, mm := range methodRx.FindAllStringSubmatch(m[2], -1) {
			if !al[mm[1]] {
				return fmt.Errorf("import %s.%s is not stubbed by engine.Host", mod, mm[1])
			}
		}
	}
	return nil
}

// goMod is a grammar module's go.mod. rootVersion "" is the local-development form (replace ../..); a version
// emits the release form of ADR-0022: the exact root version, no replace.
func goMod(lang, rootVersion string) string {
	head := "module " + modulePrefix + lang + "\n\ngo 1.26.0\n\n"
	if rootVersion != "" {
		return head + "require github.com/wotjr1649/go-treesitter " + rootVersion + "\n"
	}
	return head + "require github.com/wotjr1649/go-treesitter v0.0.0\n\n" +
		"// Local development: the grammar modules live in the root repository. Removed when the root module is tagged.\n" +
		"replace github.com/wotjr1649/go-treesitter => ../..\n"
}

// rootGoSum returns the two go.sum lines of the root module at version, taken from `go mod download -json` through
// the default GOPROXY and GOSUMDB (so the checksum database has verified them). It fails when they are unavailable.
func rootGoSum(version string) (string, error) {
	const root = "github.com/wotjr1649/go-treesitter"
	c := proc.Command("go", "mod", "download", "-json", root+"@"+version)
	c.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOFLAGS=")
	c.Dir = os.TempDir()
	var stderr bytes.Buffer
	c.Stderr = &stderr
	out, err := c.Output()
	var r struct{ Sum, GoModSum, Error string }
	if jerr := json.Unmarshal(out, &r); err != nil || jerr != nil || r.Error != "" || r.Sum == "" || r.GoModSum == "" {
		return "", fmt.Errorf("go mod download %s@%s: sums unavailable (err=%v, json=%v, %s%s)", root, version, err, jerr, r.Error, stderr.String())
	}
	return fmt.Sprintf("%s %s %s\n%s %s/go.mod %s\n", root, version, r.Sum, root, version, r.GoModSum), nil
}

// ---- language meta ----

type langMeta struct {
	ABI           uint32
	Symbols       []string
	Flags         []uint8
	PublicSymbols []uint16
	Fields        []string
}

const metaRunner = `package main

import (
	"fmt"
	"os"

	g "%s"
	"github.com/wotjr1649/go-treesitter/internal/engine"
)

func fail(a ...any) { fmt.Fprintln(os.Stderr, a...); os.Exit(1) }

func main() {
	h := &engine.Host{}
	m := g.New(h, h)
	mem := m.Xmemory().Slice()
	if err := engine.SetLimit(mem, 256<<20); err != nil {
		fail(err)
	}
	if err := engine.Call(m.X_initialize); err != nil {
		fail(err)
	}
	if v := m.Xtsg_abi_version(); v != engine.ABIVersion {
		fail("abi version", v)
	}
	p := int(m.Xtsg_lang_meta())
	if p == 0 {
		fail("tsg_lang_meta returned NULL")
	}
	b := *mem
	u := func(o int) int { return int(uint32(b[p+o]) | uint32(b[p+o+1])<<8 | uint32(b[p+o+2])<<16 | uint32(b[p+o+3])<<24) }
	nSym, nField, pool := u(12), u(16), u(20)
	if u(4) != engine.MetaFormat {
		fail("meta format", u(4))
	}
	total := 24 + 4*(nSym+1) + 4*(nField+1) + 3*nSym + pool
	fmt.Printf("%%x\n", b[p:p+total])
	engine.Release(mem)
}
`

// dumpMeta runs the freshly generated module once (X_initialize + tsg_lang_meta) in a scratch module and parses the block.
func dumpMeta(o opts, work string) (*langMeta, error) {
	dir := filepath.Join(work, "meta")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	repo, err := filepath.Abs(o.repo)
	if err != nil {
		return nil, err
	}
	out, err := filepath.Abs(o.out)
	if err != nil {
		return nil, err
	}
	mod := modulePrefix + o.lang
	gm := fmt.Sprintf("module github.com/wotjr1649/go-treesitter/tools/gen/metarun\n\ngo 1.26.0\n\nrequire (\n\t%s v0.0.0\n\tgithub.com/wotjr1649/go-treesitter v0.0.0\n)\n\n"+
		"replace %s => %s\n\nreplace github.com/wotjr1649/go-treesitter => %s\n", mod, mod, filepath.ToSlash(out), filepath.ToSlash(repo))
	if err := writeFiles(dir, map[string][]byte{"go.mod": []byte(gm), "main.go": []byte(fmt.Sprintf(metaRunner, mod))}); err != nil {
		return nil, err
	}
	res, err := runCmd(dir, []string{"CGO_ENABLED=0", "GOWORK=off", "GOPROXY=off", "GOTOOLCHAIN=local", "GOFLAGS=-mod=mod"}, "go", "run", ".")
	if err != nil {
		return nil, fmt.Errorf("meta runner: %w\n%s", err, res)
	}
	raw, err := hex.DecodeString(strings.TrimSpace(res))
	if err != nil {
		return nil, fmt.Errorf("meta runner output: %w", err)
	}
	return parseMeta(raw)
}

// parseMeta decodes the tsg_lang_meta block (engine-abi.md §3).
func parseMeta(b []byte) (*langMeta, error) {
	if len(b) < 24 || string(b[:4]) != "TSGM" {
		return nil, fmt.Errorf("meta: bad magic")
	}
	le := binary.LittleEndian
	u := func(o int) int { return int(le.Uint32(b[o:])) }
	if u(4) != 2 {
		return nil, fmt.Errorf("meta: format %d", u(4))
	}
	nSym, nField, pool := u(12), u(16), u(20)
	if nSym < 1 || nSym > 65535 || nField < 1 || nField > 65536 || pool < 0 || pool > len(b) {
		return nil, fmt.Errorf("meta: invalid symbol, field or pool size")
	}
	symOff, fldOff := 24, 24+4*(nSym+1)
	publicAt := fldOff + 4*(nField+1)
	flags := publicAt + 2*nSym
	poolAt := flags + nSym
	if poolAt != len(b)-pool {
		return nil, fmt.Errorf("meta: length %d does not match header", len(b))
	}
	name := func(tab, i int) (string, error) {
		a, z := u(tab+4*i), u(tab+4*i+4)
		if a < 0 || a > z || z > pool {
			return "", fmt.Errorf("meta: bad offsets %d..%d", a, z)
		}
		return string(b[poolAt+a : poolAt+z]), nil
	}
	m := &langMeta{ABI: uint32(u(8)), Flags: append([]uint8(nil), b[flags:flags+nSym]...)}
	for i := 0; i < nSym; i++ {
		public := le.Uint16(b[publicAt+2*i:])
		if int(public) >= nSym {
			return nil, fmt.Errorf("meta: public symbol %d for raw ID %d out of range", public, i)
		}
		m.PublicSymbols = append(m.PublicSymbols, public)
		s, err := name(symOff, i)
		if err != nil {
			return nil, err
		}
		m.Symbols = append(m.Symbols, s)
	}
	for i := 0; i < nField; i++ {
		s, err := name(fldOff, i)
		if err != nil {
			return nil, err
		}
		m.Fields = append(m.Fields, s)
	}
	if m.Fields[0] != "" {
		return nil, fmt.Errorf("meta: field 0 is %q", m.Fields[0])
	}
	return m, nil
}

// ---- emitted files ----

func emitLanguage(lang string, abi int, m *langMeta, id Identity) ([]byte, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by tools/gen; DO NOT EDIT.\n\npackage %s\n\nimport \"%s\"\n\n", lang, enginePath)
	b.WriteString("var _ engine.Module = (*Module)(nil)\n\n")
	fmt.Fprintf(&b, "// grammar is the static description handed to treesitter.NewLanguage; its Identity equals provenance.json.\nvar grammar = engine.Grammar{\n")
	fmt.Fprintf(&b, "\tABI:  %d,\n\tName: %q,\n\tNew:  func(h *engine.Host) engine.Module { return New(h, h) },\n", abi, lang)
	fmt.Fprintf(&b, "\tMeta: engine.LangMeta{\n\t\tLanguageABI: %d,\n\t\tSymbols: []string{\n", m.ABI)
	for _, s := range m.Symbols {
		fmt.Fprintf(&b, "\t\t\t%s,\n", strconv.Quote(s))
	}
	b.WriteString("\t\t},\n\t\tSymbolFlags: []uint8{")
	for i, f := range m.Flags {
		if i%32 == 0 {
			b.WriteString("\n\t\t\t")
		} else {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%d,", f)
	}
	b.WriteString("\n\t\t},\n\t\tPublicSymbols: []uint16{")
	for i, s := range m.PublicSymbols {
		if i%24 == 0 {
			b.WriteString("\n\t\t\t")
		} else {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%d,", s)
	}
	b.WriteString("\n\t\t},\n\t\tFields: []string{\n")
	for _, s := range m.Fields {
		fmt.Fprintf(&b, "\t\t\t%s,\n", strconv.Quote(s))
	}
	b.WriteString("\t\t},\n\t},\n\tIdentity: engine.Identity{\n")
	fmt.Fprintf(&b, "\t\tRuntime:     %q,\n\t\tGrammarRepo: %q,\n\t\tGrammarRev:  %q,\n", id.Runtime, id.GrammarRepo, id.GrammarRev)
	b.WriteString("\t\tSourceSHA256: map[string]string{\n")
	for _, k := range sortedKeys(id.SourceSHA256) {
		fmt.Fprintf(&b, "\t\t\t%q: %q,\n", k, id.SourceSHA256[k])
	}
	fmt.Fprintf(&b, "\t\t},\n\t\tRecipeSHA256: %q,\n\t\tTools: map[string]string{\n", id.RecipeSHA256)
	for _, k := range sortedKeys(id.Tools) {
		fmt.Fprintf(&b, "\t\t\t%q: %q,\n", k, id.Tools[k])
	}
	b.WriteString("\t\t},\n\t},\n}\n")
	return format.Source([]byte(b.String()))
}

func sortedKeys(m map[string]string) []string {
	k := make([]string, 0, len(m))
	for x := range m {
		k = append(k, x)
	}
	sort.Strings(k)
	return k
}

func languageAPI(lang string) string {
	return "// Code generated by tools/gen; DO NOT EDIT.\n\npackage " + lang + "\n\nimport (\n\t\"sync\"\n\n\ttreesitter \"github.com/wotjr1649/go-treesitter\"\n)\n\n" +
		"// languageOnce builds the *treesitter.Language once: a Language caches its symbol and field indexes.\n" +
		"var languageOnce = sync.OnceValue(func() *treesitter.Language { return treesitter.NewLanguage(&grammar) })\n\n" +
		"// Language returns the grammar for treesitter.NewParser. Every call returns the same value.\nfunc Language() *treesitter.Language { return languageOnce() }\n"
}

func readme(lang string, pins *Pins, g GrammarPin, id Identity) string {
	files := "`LICENSE` is the upstream grammar's license."
	if g.NoticeSHA256 != "" {
		files = "`LICENSE` and `NOTICE` are the upstream grammar's license files."
	}
	result := fmt.Sprintf("# %s%s\n\nGenerated tree-sitter grammar module for go-treesitter. **Do not edit**: `engine_gen.go` and `language.go` are output of\n"+
		"`tools/gen` (docs/design/engine-abi.md, ADR-0020).\n\n"+
		"- upstream grammar: %s at `%s`\n- tree-sitter runtime: %s (unmodified)\n- engine ABI: %d\n"+
		"- recipe sha256: `%s`\n- identity and output hashes: `provenance.json` (equals the compiled `engine.Identity`; `go test` checks it)\n\n"+
		"%s `LICENSES/tree-sitter.txt` is the license of the tree-sitter runtime C translated into `engine_gen.go`. `LICENSES/wasi-libc/` holds the license texts of the wasi-libc code translated into `engine_gen.go` (wasi-libc `%s`;\n"+
		"its `LICENSE` is the index: musl MIT, cloudlibc BSD-2-Clause, dlmalloc CC0, the rest Apache-2.0 WITH LLVM-exception, Apache-2.0 or MIT;\n"+
		"every file's hash is in `provenance.json`). Regenerate with `tools/gen` (see its README); regenerating gives identical bytes.\n",
		modulePrefix, lang, g.Repo, g.Commit, pins.Runtime.Release, pins.ABI, id.RecipeSHA256, files, pins.WasiLibc.Commit)
	switch lang {
	case "php", "ruby":
		result += "\nSource tier: 1 (core), per ADR-0024.\n"
	case "yaml", "xml":
		result += "\nSource tier: 2 (tree-sitter-grammars), per ADR-0024.\n"
	case "postgres":
		result += "\nSource tier: owner-designated, per ADR-0024. PostgreSQL SQL only; PL/pgSQL is not included.\n"
	case "kotlin", "powershell", "r", "dart", "swift":
		result += "\nSource tier: 3 (third party), per ADR-0024.\n"
	}
	if len(g.Patches) > 0 {
		result += "\nThis grammar differs temporarily from upstream under ADR-0023. Applied patches (full hashes in `provenance.json`):\n"
		switch lang {
		case "bash", "ruby":
			result += "\nThe heredoc patch stores and compares UTF-8 end words, preserving ASCII bytes and distinguishing code points that share a low byte. Invalid lookaheads all compare as 0xFF, distinct from U+FFFD. The 1024-byte scanner state buffer includes all open end words and metadata; overflow loses state (about 330 Hangul syllables total, fewer with other state).\n"
		case "yaml":
			result += "\nYAML patch 0001 prevents premature end-of-input acceptance and silent source truncation.\n"
			if len(g.Patches) > 1 {
				result += "\nYAML patch 0002 bounds serialized indentation by bytes written, preventing deep-nesting buffer overruns (upstream PR #47).\n"
			}
			if len(g.Patches) > 2 {
				result += "\nYAML patch 0003 widens scanner rows to 32 bits, so documents past 32,767 line breaks parse, counting CR and LF separately: line 16,385 is the boundary with CRLF line ends (adapted from upstream PR #35: the block implicit key row is serialized as a saturating 16-bit distance, keeping 253 indentation levels within the 0002 bound).\n"
			}
		case "rust":
			result += "\nThe block comment patch compares full code points for comment delimiters and documentation markers (upstream PR #316). The unpatched scanner read a code point whose low byte is `/`, `*` or `!` as that character, so previously error-free trees change: ordinary comments lose a false `inner`/`outer` marker and `doc` child; after a false nested opening is removed, the comment ends at the `*/` that closes every opening the scanner counts, and the rest is parsed as code (the scanner still does not count a `/*` that directly follows `/`, for example `//*`, an unchanged upstream limitation; where a false opening used to offset it, valid Rust such as `/* 是个 //* */ */` followed by code was error-free before and now ends the comment at the first `*/` with an `ERROR` over the second); after a false nested closing is removed or a previously hidden real `/*` is recognized, the block comment can extend to a later real `*/` and absorb nodes and query captures even in an error-free tree. When real closes run out, the tree can contain a `MISSING` `*/` or an `ERROR` that absorbs the following code. A documentation comment's `doc_comment` range can also change (see MIGRATING.md).\n"
		case "javascript":
			result += "\nThe JSX text patch accepts a bare `&` in JSX text; character references still produce `html_character_reference` (upstream PR #388). TSX has the same defect and is not patched.\n"
		case "html":
			result += "\nPatch 0001 accepts `>` in element text (upstream PR #141). Patch 0002 is this project's own fix for upstream issue #143, which has no upstream pull request. The upstream `entity` token matches bounded prefixes with an optional `;`. The forms are `&` followed by 1–30 ASCII letters, `&#` followed by 1–5 decimal digits, and `&#x` or `&#X` followed by 1–6 hexadecimal digits. For example, `R&D` gives `text R` and `entity &D`, and `&#x26x` gives `entity &#x26` and `text x`. Other `&` characters are text. A bare `&` followed by whitespace can join a `text` token when text continues after the whitespace (`Users & roles` is one `text`). If a tag, an entity or end of input follows the whitespace, the `&` can be a separate `text` node.\n"
		case "golang":
			result += "\nThe Go 1.26 patch accepts expressions as the first argument to `new` and prefers the type interpretation when ambiguous. The first argument to `make` still uses the type rule (adapted from upstream PR #202).\n"
		case "postgres":
			result += "\nOwner-approved exception (2026-11-17) to ADR-0023 decision 1 for Korean support; this gap does not block a release gate. Unquoted identifiers accept all non-ASCII code points, and dollar-quote tags compare complete code points. The upstream 63-code-point tag cap remains.\n"
		case "python":
			result += "\nThe indentation patch bounds serialization by bytes written, preventing the state-buffer overrun that deep indentation causes when an odd number of string delimiters is open, e.g. inside one string literal (upstream PR #351).\n"
		}
		for _, p := range g.Patches {
			if p.File == "patches/ruby/0002-global-variable-non-ascii.patch" {
				result += "\nThe global-variable patch accepts non-ASCII names such as `$한글`. Previously error-free trees change where an ASCII global name is directly followed by non-ASCII characters (for example `$a한 = 1` or `\"#$a한\"`): the whole name becomes one `global_variable` (see MIGRATING.md).\n"
				break
			}
		}
		for _, p := range g.Patches {
			result += fmt.Sprintf("\n- `%s` changes `%s`; upstream defect report: %s.\n", p.File, p.Source, p.UpstreamReport)
		}
	}
	return result
}

var samples = map[string]string{
	"swift":      "func add(_ a: Int, _ b: Int) -> Int { return a + b }\n",
	"postgres":   "SELECT customer_id, count(*) FROM rental WHERE return_date IS NULL GROUP BY customer_id;\n",
	"kotlin":     "fun add(a: Int, b: Int): Int = a + b\n",
	"yaml":       "items:\n  - name: one\n    value: 1\n",
	"php":        "<?php function add($a, $b) { return $a + $b; }\n",
	"powershell": "$items = @(1, 2, 3)\nforeach ($item in $items) { Write-Output $item }\n",
	"r":          "f <- function(x) { x + 1 }\ny <- f(2)\n",
	"dart":       "int add(int a, int b) { return a + b; }\n",
	"xml":        "<?xml version=\"1.0\"?><root><item id=\"1\">value</item></root>\n",
	"ruby":       "def add(a, b)\n  a + b\nend\n",

	"javascript": "const C = (p) => <div id=\"a\">{p.n + 1}</div>;\n",
	"html":       "<!doctype html><html><body><p>Hello</p></body></html>\n",
	"css":        "body { color: red; margin: 0; }\n",
	"java":       "class A { int f(int x) { return x + 1; } }\n",
	"c":          "int f(int x) { return x + 1; }\n",
	"rust":       "fn f(x: i32) -> i32 { x + 1 }\n",
	"bash":       "#!/bin/bash\nfor x in one two; do echo \"$x\"; done\n",
	"cpp":        "template <class T> T add(T a, T b) { return a + b; }\n",
	"json":       "{\"a\": [1, 2.5, true, null, {\"b\": \"c\"}]}\n",
	"golang":     "package main\n\nfunc main() {\n\tx := []int{1, 2}\n\t_ = x\n}\n",
	"python":     "def f(x):\n    return [i * 2 for i in x]\n\nprint(f([1, 2]))\n",
	"typescript": "interface A { x: number }\nconst f = (a: A): number => a.x + 1;\n",
	"tsx":        "const C = (p: { n: number }) => <div id=\"a\">{p.n}</div>;\n",
	"csharp":     "class A { int F(int x) => x + 1; }\n",
	"tsql":       "SELECT a, b FROM t WHERE a > 1;\n",
}

func emitTest(lang string) ([]byte, error) {
	s, ok := samples[lang]
	if !ok {
		return nil, fmt.Errorf("no sample for %q", lang)
	}
	t := strings.NewReplacer("@PKG@", lang, "@SAMPLE@", strconv.Quote(s)).Replace(testTemplate)
	return format.Source([]byte(t))
}
