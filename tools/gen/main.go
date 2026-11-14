// Command gen builds one grammar module (grammars/<lang>) from pinned upstream C sources:
//
//	clang (wasi-sdk) -> wasm -> wasm2go -unsafe -> hash-checked memory_grow replacement -> Go module
//
// Every input is verified against tools/gen/pins.json before it is used; a mismatch is a failure, never a warning.
// See docs/design/engine-abi.md (ABI) and docs/specs/baseline-provenance.md (pins).
package main

import (
	"bytes"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

var rootVersionRx = regexp.MustCompile(`^v[0-9]+[.][0-9]+[.][0-9]+(-[0-9A-Za-z.-]+)?$`)

const modulePrefix = "github.com/wotjr1649/go-treesitter/grammars/"

// Fixed recipe (docs/design/engine-abi.md §1). Changing any of it changes every grammar's RecipeSHA256.
var (
	clangFlags = []string{"--target=wasm32-wasip1", "-mexec-model=reactor", "-O2", "-DNDEBUG",
		"-Wl,--gc-sections", "-Wl,--no-entry", "-Wl,--strip-all"}
	exports = []string{"tsg_abi_version", "tsg_init", "tsg_parse", "tsg_parser_reset", "tsg_root_end", "tsg_root_has_error", "tsg_descendant_count",
		"tsg_tree_edit", "tsg_tree_delete", "tsg_changed_ranges", "tsg_export_begin", "tsg_export_next", "tsg_export_err",
		"tsg_export_buf", "tsg_export_end", "tsg_lang_meta", "malloc", "free",
		"tsg_query_new", "tsg_query_error_offset", "tsg_query_error_type", "tsg_query_delete", "tsg_query_meta",
		"tsg_qcursor_new", "tsg_qcursor_delete", "tsg_qcursor_exec", "tsg_qcursor_next", "tsg_qcursor_halted",
		"tsg_qcursor_exceeded", "tsg_qcursor_buf"}
)

type opts struct {
	lang, root, src, dl, sdk, wasm2go, wasmOpt, recipe, out, work, repo, rootVersion, plan string
	hash, noAPI, verifyBinaries                                                            bool
}

func main() {
	var o opts
	flag.StringVar(&o.lang, "lang", "", "grammar module to generate (key in pins.json)")
	flag.StringVar(&o.root, "root", ".", "tools/gen directory (pins.json, shim/)")
	flag.StringVar(&o.src, "src", "", "directory holding the extracted upstream trees")
	flag.StringVar(&o.dl, "dl", "", "optional directory holding the upstream archives (verifies their sha256)")
	flag.StringVar(&o.sdk, "sdk", "", "wasi-sdk directory")
	flag.StringVar(&o.wasm2go, "wasm2go", "", "wasm2go binary")
	flag.StringVar(&o.wasmOpt, "wasm-opt", "", "wasm-opt binary (binaryen; only for recipes that run it)")
	flag.StringVar(&o.recipe, "recipe", "", "build variant; overrides the recipe field of pins.json (size comparisons), recorded in provenance.json")
	flag.StringVar(&o.out, "out", "", "output module directory")
	flag.StringVar(&o.work, "work", "", "scratch directory (wasm, meta runner)")
	flag.StringVar(&o.repo, "repo", "../..", "repository root (only for the meta runner's replace directive)")
	flag.StringVar(&o.rootVersion, "root-version", "", "release mode: emit go.mod requiring the root module at this exact version (vX.Y.Z), without replace; default is the local-development form")
	flag.StringVar(&o.plan, "plan", "", "print the fetch plan for this host platform (GOOS-GOARCH, or \"host\") and exit; tools/gen/regen.sh executes it")
	flag.BoolVar(&o.verifyBinaries, "verify-binaries", false, "also check the host binary hashes pinned in localBinarySHA256 (optional local verification, not an identity)")
	flag.BoolVar(&o.hash, "hash", false, "print the computed values of every pin and exit")
	flag.BoolVar(&o.noAPI, "no-api", false, "do not emit language_api.go (Language() needs the root treesitter package)")
	flag.Parse()
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

func run(o opts) error {
	pins, err := loadPins(filepath.Join(o.root, "pins.json"))
	if err != nil {
		return err
	}
	if o.plan != "" {
		if o.plan == "host" {
			o.plan = hostPlatform()
		}
		lines, err := pins.plan(o.plan)
		if err != nil {
			return err
		}
		fmt.Println(strings.Join(lines, "\n"))
		return nil
	}
	if o.rootVersion != "" && !rootVersionRx.MatchString(o.rootVersion) {
		return fmt.Errorf("-root-version %q is not a semantic version vX.Y.Z", o.rootVersion)
	}
	goSum := "" // release mode only; dev mode writes none and removes a stale one
	if o.rootVersion != "" {
		var err error
		if goSum, err = rootGoSum(o.rootVersion); err != nil {
			return err
		}
	}
	if o.src == "" {
		return fmt.Errorf("-src is required")
	}
	if o.hash {
		return printHashes(o, pins)
	}
	g, ok := pins.Grammars[o.lang]
	if !ok {
		return fmt.Errorf("unknown -lang %q", o.lang)
	}
	if o.sdk == "" || o.wasm2go == "" || o.out == "" || o.work == "" {
		return fmt.Errorf("-sdk, -wasm2go, -out and -work are required")
	}
	rname := pins.Recipe
	if o.recipe != "" {
		rname = o.recipe
	}
	rc, err := lookupRecipe(rname)
	if err != nil {
		return err
	}
	tm := map[string]float64{}
	id, err := verify(o, pins, g, rc, tm)
	if err != nil {
		return err
	}

	work := filepath.Join(o.work, o.lang)
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	wasm := filepath.Join(work, o.lang+".wasm")
	t0 := time.Now()
	if _, err := compile(o, pins, g, wasm); err != nil {
		return err
	}
	tm["clang_s"] = since(t0)
	if err := checkLink(o, pins, g, filepath.Join(work, o.lang+".linkcheck.wasm")); err != nil {
		return err
	}
	outputs := map[string]string{}
	if rc.opt != "" {
		clangWasm, err := os.ReadFile(wasm)
		if err != nil {
			return err
		}
		outputs["wasm.clang"] = sumBytes(clangWasm)
		t0 = time.Now()
		opt := filepath.Join(work, o.lang+".opt.wasm")
		if out, err := runCmd("", nil, o.wasmOpt, append(rc.wasmOptArgs(), wasm, "-o", opt)...); err != nil {
			return fmt.Errorf("wasm-opt: %w\n%s", err, out)
		}
		wasm = opt
		tm["wasm_opt_s"] = since(t0)
	}

	t0 = time.Now()
	// engine_gen.go in the work dir: with -embed wasm2go names the data file after it (engine_gen.dat) and go:embeds that name.
	genGo := filepath.Join(work, "engine_gen.go")
	if out, err := runCmd("", nil, o.wasm2go, append(rc.wasm2goFlags(), "-pkg", o.lang, "-o", genGo, wasm)...); err != nil {
		return fmt.Errorf("wasm2go: %w\n%s", err, out)
	}
	raw, err := os.ReadFile(genGo)
	if err != nil {
		return err
	}
	if err := checkImports(string(raw)); err != nil {
		return err
	}
	patched, err := patchMemoryGrow(string(raw), pins.MemoryGrowSHA256)
	if err != nil {
		return err
	}
	if rc.embed { // wasm2go -embed leaves no blank line before the directive, which gofmt -l reports
		patched = strings.Replace(patched, "}\n//go:embed engine_gen.dat\n", "}\n\n//go:embed engine_gen.dat\n", 1)
	}
	tm["wasm2go_s"] = since(t0)

	wasmBytes, _ := os.ReadFile(wasm)
	files := map[string][]byte{
		"engine_gen.go": []byte(patched),
		"go.mod":        []byte(goMod(o.lang, o.rootVersion)),
	}
	if rc.embed {
		dat, err := os.ReadFile(filepath.Join(work, "engine_gen.dat"))
		if err != nil {
			return fmt.Errorf("wasm2go -embed wrote no engine_gen.dat: %w", err)
		}
		if !strings.Contains(patched, "//go:embed engine_gen.dat\n") {
			return fmt.Errorf("wasm2go -embed output has no //go:embed engine_gen.dat directive")
		}
		files["engine_gen.dat"] = dat
		outputs["engine_gen.dat"] = sumBytes(dat)
	} else if err := os.Remove(filepath.Join(o.out, "engine_gen.dat")); err != nil && !errors.Is(err, fs.ErrNotExist) { // stale file of another recipe
		return err
	}
	if goSum != "" {
		files["go.sum"] = []byte(goSum)
	} else if err := os.Remove(filepath.Join(o.out, "go.sum")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := writeFiles(o.out, files); err != nil {
		return err
	}

	t0 = time.Now()
	meta, err := dumpMeta(o, work)
	if err != nil {
		return err
	}
	tm["meta_s"] = since(t0)

	id.RecipeSHA256 = recipeSHA256(pins, g, rname, rc)
	id.Tools = map[string]string{"wasi-sdk": pins.Tools.WasiSDK, "clang": pins.Tools.Clang, "llvm": pins.Tools.LLVMCommit, "wasm2go": pins.Tools.Wasm2go, "wasm2go-sum": pins.Tools.Wasm2goSum, "recipe": rname}
	if rc.opt != "" {
		id.Tools["binaryen"] = pins.Tools.Binaryen
	}
	lic, err := os.ReadFile(filepath.Join(o.src, g.ExtractDir, "LICENSE"))
	if err != nil {
		return err
	}
	notices, err := wasiLibcNotices(o, pins)
	if err != nil {
		return err
	}
	rtn, err := runtimeNotice(o, pins)
	if err != nil {
		return err
	}
	for name, b := range rtn {
		notices[name] = b
	}
	var noticeBytes []byte
	if g.NoticeSHA256 != "" {
		if noticeBytes, err = os.ReadFile(filepath.Join(o.src, g.ExtractDir, "NOTICE")); err != nil {
			return err
		}
	}
	outputs["engine_gen.go"], outputs["wasm"] = sumBytes([]byte(patched)), sumBytes(wasmBytes)
	for name, b := range notices {
		outputs[name] = sumBytes(b)
	}
	prov := provenance{Schema: 1, ABI: uint32(pins.ABI), Name: o.lang, Module: modulePrefix + o.lang, Identity: id, Outputs: outputs}
	provJSON, err := marshal(prov)
	if err != nil {
		return err
	}
	lang, err := emitLanguage(o.lang, pins.ABI, meta, id)
	if err != nil {
		return err
	}
	test, err := emitTest(o.lang)
	if err != nil {
		return err
	}
	files = map[string][]byte{
		"language.go":     lang,
		"module_test.go":  test,
		"LICENSE":         lic,
		"provenance.json": provJSON,
		"README.md":       []byte(readme(o.lang, pins, g, id)),
	}
	if noticeBytes != nil {
		files["NOTICE"] = noticeBytes
	}
	for name, b := range notices {
		files[name] = b
	}
	if !o.noAPI {
		files["language_api.go"] = []byte(languageAPI(o.lang))
	}
	if err := writeFiles(o.out, files); err != nil {
		return err
	}
	tm["wasm_bytes"] = float64(len(wasmBytes))
	tm["engine_gen_bytes"] = float64(len(patched))
	b, _ := json.Marshal(map[string]any{"lang": o.lang, "ok": true, "metrics": tm, "wasmSHA256": prov.Outputs["wasm"],
		"engineGenSHA256": prov.Outputs["engine_gen.go"], "symbols": len(meta.Symbols), "fields": len(meta.Fields)})
	fmt.Println(string(b))
	return nil
}

func since(t time.Time) float64 { return float64(time.Since(t).Milliseconds()) / 1000 }

// verify checks every pinned input and tool, and returns the identity inputs.
func verify(o opts, pins *Pins, g GrammarPin, rc recipe, tm map[string]float64) (Identity, error) {
	id := Identity{Runtime: pins.Runtime.Release, GrammarRepo: g.Repo, GrammarRev: g.Commit, SourceSHA256: map[string]string{}}
	rt := pins.Runtime
	got, err := treeSHA256(filepath.Join(o.src, rt.ExtractDir), rt.Dirs)
	if err != nil {
		return id, err
	}
	if err := expect("runtime tree", got, rt.TreeSHA256); err != nil {
		return id, err
	}
	id.SourceSHA256["runtime.tree"] = got
	if got, err = treeSHA256(filepath.Join(o.src, g.ExtractDir), g.Dirs); err != nil {
		return id, err
	}
	if err := expect("grammar tree", got, g.TreeSHA256); err != nil {
		return id, err
	}
	id.SourceSHA256["grammar.tree"] = got
	for name, want := range g.Files {
		got, err := sumFile(filepath.Join(o.src, g.ExtractDir, filepath.FromSlash(g.GrammarDir), name))
		if err != nil {
			return id, err
		}
		if err := expect("grammar "+name, got, want); err != nil {
			return id, err
		}
		id.SourceSHA256["grammar."+name] = got
	}
	if got, err = sumFile(filepath.Join(o.src, g.ExtractDir, "LICENSE")); err != nil {
		return id, err
	}
	if err := expect("grammar LICENSE", got, g.LicenseSHA256); err != nil {
		return id, err
	}
	id.SourceSHA256["grammar.LICENSE"] = got
	// NOTICE is optional upstream; an unpinned NOTICE is a failure so a new one cannot be missed silently.
	notice := filepath.Join(o.src, g.ExtractDir, "NOTICE")
	if got, err = sumFile(notice); err == nil {
		if err := expect("grammar NOTICE", got, g.NoticeSHA256); err != nil {
			return id, err
		}
		id.SourceSHA256["grammar.NOTICE"] = got
	} else if !errors.Is(err, fs.ErrNotExist) {
		return id, err
	} else if g.NoticeSHA256 != "" {
		return id, fmt.Errorf("pin mismatch: grammar NOTICE pinned %s but the file is absent", g.NoticeSHA256)
	}
	if got, err = sumFile(filepath.Join(o.root, "shim", "tsg_abi.c")); err != nil {
		return id, err
	}
	if err := expect("shim tsg_abi.c", got, pins.ShimSHA256); err != nil {
		return id, err
	}
	id.SourceSHA256["shim.tsg_abi.c"] = got
	if o.dl != "" {
		for _, a := range [][2]string{{rt.Archive, rt.ArchiveSHA256}, {g.Archive, g.ArchiveSHA256}, {pins.WasiLibc.Archive, pins.WasiLibc.ArchiveSHA256}} {
			got, err := sumFile(filepath.Join(o.dl, a[0]))
			if err != nil {
				return id, err
			}
			if err := expect("archive "+a[0], got, a[1]); err != nil {
				return id, err
			}
		}
	}
	// tools
	out, err := runCmd("", nil, clangPath(o.sdk), "--version")
	if err != nil {
		return id, fmt.Errorf("clang: %w", err)
	}
	cv, cc, err := parseClangVersion(strings.SplitN(out, "\n", 2)[0])
	if err != nil {
		return id, err
	}
	if err := expect("clang version", cv, pins.Tools.Clang); err != nil {
		return id, err
	}
	if err := expect("llvm commit", cc, pins.Tools.LLVMCommit); err != nil {
		return id, err
	}
	sdkVer, err := os.ReadFile(filepath.Join(o.sdk, "VERSION"))
	if err != nil {
		return id, err
	}
	if err := expect("wasi-sdk version", strings.TrimSpace(strings.SplitN(string(sdkVer), "\n", 2)[0]), pins.Tools.WasiSDK); err != nil {
		return id, err
	}
	if err := checkWasiLibcCommit(string(sdkVer), pins); err != nil {
		return id, err
	}
	bi, err := buildinfo.ReadFile(o.wasm2go)
	if err != nil {
		return id, err
	}
	// The module version and its go.sum hash identify wasm2go; the binary itself is rebuilt per host (go install -trimpath).
	if err := expect("wasm2go module", bi.Main.Path, pins.Tools.Wasm2goModule); err != nil {
		return id, err
	}
	if err := expect("wasm2go module version", bi.Main.Version, pins.Tools.Wasm2go); err != nil {
		return id, err
	}
	if err := expect("wasm2go module sum", bi.Main.Sum, pins.Tools.Wasm2goSum); err != nil {
		return id, err
	}
	if err := verifyBinary(o, pins, "wasm2go", o.wasm2go); err != nil {
		return id, err
	}
	if rc.opt != "" {
		if o.wasmOpt == "" {
			return id, fmt.Errorf("-wasm-opt is required by this recipe")
		}
		if err := verifyBinary(o, pins, "wasm-opt", o.wasmOpt); err != nil {
			return id, err
		}
		out, err := runCmd("", nil, o.wasmOpt, "--version")
		if err != nil {
			return id, fmt.Errorf("wasm-opt: %w", err)
		}
		wv, err := parseWasmOptVersion(strings.SplitN(out, "\n", 2)[0])
		if err != nil {
			return id, err
		}
		if err := expect("wasm-opt version", wv, pins.Tools.Binaryen); err != nil {
			return id, err
		}
	}
	return id, nil
}

// verifyBinary is the optional local check of a host binary against localBinarySHA256 (-verify-binaries).
func verifyBinary(o opts, pins *Pins, tool, path string) error {
	if !o.verifyBinaries {
		return nil
	}
	want, ok := pins.Tools.LocalBinarySHA256[hostPlatform()][tool]
	if !ok { // wasm2go has none: a go install build differs with the Go version, so only its module sum is pinned
		return nil
	}
	got, err := sumFile(path)
	if err != nil {
		return err
	}
	return expect(tool+" binary", got, want)
}

func clangPath(sdk string) string {
	p := filepath.Join(sdk, "bin", "clang")
	if runtime.GOOS == "windows" {
		p += ".exe"
	}
	return p
}

// compile builds the wasm; extra are additional clang flags that must not change the output (the link check passes -Wl,-t).
func compile(o opts, pins *Pins, g GrammarPin, wasm string, extra ...string) (string, error) {
	rt := filepath.Join(o.src, pins.Runtime.ExtractDir, "lib")
	gd := filepath.Join(o.src, g.ExtractDir, filepath.FromSlash(g.GrammarDir))
	args := append([]string{}, clangFlags...)
	args = append(args, extra...)
	args = append(args, "--sysroot="+filepath.Join(o.sdk, "share", "wasi-sysroot"), "-DTSG_LANG_FN="+g.CSymbol,
		"-I"+filepath.Join(rt, "src"), "-I"+filepath.Join(rt, "include"), "-I"+gd)
	for _, e := range exports {
		args = append(args, "-Wl,--export="+e)
	}
	args = append(args, filepath.Join(rt, "src", "lib.c"), filepath.Join(gd, "parser.c"))
	if _, ok := g.Files["scanner.c"]; ok {
		args = append(args, filepath.Join(gd, "scanner.c"))
	}
	args = append(args, filepath.Join(o.root, "shim", "tsg_abi.c"), "-o", wasm)
	out, err := runCmd("", nil, clangPath(o.sdk), args...)
	if err != nil {
		return out, fmt.Errorf("clang: %w\n%s", err, out)
	}
	return out, nil
}

// recipeSHA256 binds the fixed recipe (flags, exports, tool invocations, the patch) to one identity string.
func recipeSHA256(pins *Pins, g GrammarPin, name string, rc recipe) string {
	var b strings.Builder
	fmt.Fprintf(&b, "abi=%d\nclang=%s\n", pins.ABI, strings.Join(clangFlags, " "))
	fmt.Fprintf(&b, "define=TSG_LANG_FN=%s\ninclude=runtime/lib/src runtime/lib/include grammar/%s\n", g.CSymbol, g.GrammarDir)
	var srcs []string
	srcs = append(srcs, "lib.c", "parser.c")
	if _, ok := g.Files["scanner.c"]; ok {
		srcs = append(srcs, "scanner.c")
	}
	srcs = append(srcs, "tsg_abi.c")
	fmt.Fprintf(&b, "sources=%s\nexports=%s\nwasm2go=-unsafe\n", strings.Join(srcs, " "), strings.Join(exports, " "))
	fmt.Fprintf(&b, "memory_grow.original=%s\nmemory_grow.replacement=%s", pins.MemoryGrowSHA256, replacementBody)
	b.WriteString(rc.recipeLines(name, pins))
	return sumBytes([]byte(b.String()))
}

func runCmd(dir string, env []string, name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Env = append(os.Environ(), env...)
	var buf bytes.Buffer
	c.Stdout, c.Stderr = &buf, &buf
	err := c.Run()
	return buf.String(), err
}

func writeFiles(dir string, files map[string][]byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		p := filepath.Join(dir, filepath.FromSlash(n))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, files[n], 0o644); err != nil {
			return err
		}
	}
	return nil
}

func printHashes(o opts, pins *Pins) error {
	res := map[string]any{}
	rt := pins.Runtime
	h := map[string]string{}
	var err error
	if h["treeSHA256"], err = treeSHA256(filepath.Join(o.src, rt.ExtractDir), rt.Dirs); err != nil {
		return err
	}
	if o.dl != "" {
		if h["archiveSHA256"], err = sumFile(filepath.Join(o.dl, rt.Archive)); err != nil {
			return err
		}
	}
	res["runtime"] = h
	gs := map[string]any{}
	for name, g := range pins.Grammars {
		if o.lang != "" && o.lang != name {
			continue
		}
		m := map[string]any{}
		if m["treeSHA256"], err = treeSHA256(filepath.Join(o.src, g.ExtractDir), g.Dirs); err != nil {
			return err
		}
		files := map[string]string{}
		for _, f := range []string{"parser.c", "scanner.c"} {
			if s, err := sumFile(filepath.Join(o.src, g.ExtractDir, filepath.FromSlash(g.GrammarDir), f)); err == nil {
				files[f] = s
			}
		}
		m["files"] = files
		if m["licenseSHA256"], err = sumFile(filepath.Join(o.src, g.ExtractDir, "LICENSE")); err != nil {
			return err
		}
		if s, err := sumFile(filepath.Join(o.src, g.ExtractDir, "NOTICE")); err == nil {
			m["noticeSHA256"] = s
		}
		if o.dl != "" {
			if m["archiveSHA256"], err = sumFile(filepath.Join(o.dl, g.Archive)); err != nil {
				return err
			}
		}
		gs[name] = m
	}
	res["grammars"] = gs
	if s, err := sumFile(filepath.Join(o.root, "shim", "tsg_abi.c")); err == nil {
		res["shimSHA256"] = s
	}
	return json.NewEncoder(os.Stdout).Encode(res)
}
