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
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const modulePrefix = "github.com/wotjr1649/go-treesitter/grammars/"

// Fixed recipe (docs/design/engine-abi.md §1). Changing any of it changes every grammar's RecipeSHA256.
var (
	clangFlags = []string{"--target=wasm32-wasip1", "-mexec-model=reactor", "-O2", "-DNDEBUG",
		"-Wl,--gc-sections", "-Wl,--no-entry", "-Wl,--strip-all"}
	exports = []string{"tsg_abi_version", "tsg_init", "tsg_parse", "tsg_parser_reset", "tsg_root_end", "tsg_root_has_error", "tsg_descendant_count",
		"tsg_tree_edit", "tsg_tree_delete", "tsg_changed_ranges", "tsg_export_begin", "tsg_export_next", "tsg_export_err",
		"tsg_export_buf", "tsg_export_end", "tsg_lang_meta", "malloc", "free"}
)

type opts struct {
	lang, root, src, dl, sdk, wasm2go, out, work, repo string
	hash, noAPI                                        bool
}

func main() {
	var o opts
	flag.StringVar(&o.lang, "lang", "", "grammar module to generate (key in pins.json)")
	flag.StringVar(&o.root, "root", ".", "tools/gen directory (pins.json, shim/)")
	flag.StringVar(&o.src, "src", "", "directory holding the extracted upstream trees")
	flag.StringVar(&o.dl, "dl", "", "optional directory holding the upstream archives (verifies their sha256)")
	flag.StringVar(&o.sdk, "sdk", "", "wasi-sdk directory")
	flag.StringVar(&o.wasm2go, "wasm2go", "", "wasm2go binary")
	flag.StringVar(&o.out, "out", "", "output module directory")
	flag.StringVar(&o.work, "work", "", "scratch directory (wasm, meta runner)")
	flag.StringVar(&o.repo, "repo", "../..", "repository root (only for the meta runner's replace directive)")
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
	tm := map[string]float64{}
	id, err := verify(o, pins, g, tm)
	if err != nil {
		return err
	}

	work := filepath.Join(o.work, o.lang)
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	wasm := filepath.Join(work, o.lang+".wasm")
	t0 := time.Now()
	if err := compile(o, pins, g, wasm); err != nil {
		return err
	}
	tm["clang_s"] = since(t0)

	t0 = time.Now()
	genGo := filepath.Join(work, "gen.go")
	if out, err := runCmd("", nil, o.wasm2go, "-unsafe", "-pkg", o.lang, "-o", genGo, wasm); err != nil {
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
	tm["wasm2go_s"] = since(t0)

	wasmBytes, _ := os.ReadFile(wasm)
	files := map[string][]byte{
		"engine_gen.go": []byte(patched),
		"go.mod":        []byte(goMod(o.lang)),
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

	id.RecipeSHA256 = recipeSHA256(pins, g)
	id.Tools = map[string]string{"wasi-sdk": pins.Tools.WasiSDK, "clang": pins.Tools.Clang, "wasm2go": pins.Tools.Wasm2go, "go": pins.Tools.Go}
	lic, err := os.ReadFile(filepath.Join(o.src, g.ExtractDir, "LICENSE"))
	if err != nil {
		return err
	}
	prov := provenance{Schema: 1, ABI: uint32(pins.ABI), Name: o.lang, Module: modulePrefix + o.lang, Identity: id,
		Outputs: map[string]string{"engine_gen.go": sumBytes([]byte(patched)), "wasm": sumBytes(wasmBytes)}}
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
func verify(o opts, pins *Pins, g GrammarPin, tm map[string]float64) (Identity, error) {
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
	if got, err = sumFile(filepath.Join(o.root, "shim", "tsg_abi.c")); err != nil {
		return id, err
	}
	if err := expect("shim tsg_abi.c", got, pins.ShimSHA256); err != nil {
		return id, err
	}
	id.SourceSHA256["shim.tsg_abi.c"] = got
	if o.dl != "" {
		for _, a := range [][2]string{{rt.Archive, rt.ArchiveSHA256}, {g.Archive, g.ArchiveSHA256}} {
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
	if err := expect("clang version", strings.TrimSpace(strings.SplitN(out, "\n", 2)[0]), pins.Tools.Clang); err != nil {
		return id, err
	}
	if got, err = sumFile(o.wasm2go); err != nil {
		return id, err
	}
	if err := expect("wasm2go binary", got, pins.Tools.Wasm2goSHA256); err != nil {
		return id, err
	}
	bi, err := buildinfo.ReadFile(o.wasm2go)
	if err != nil {
		return id, err
	}
	if err := expect("wasm2go module version", bi.Main.Version, pins.Tools.Wasm2go); err != nil {
		return id, err
	}
	if err := expect("wasm2go built with", bi.GoVersion, pins.Tools.Go); err != nil {
		return id, err
	}
	return id, nil
}

func clangPath(sdk string) string {
	p := filepath.Join(sdk, "bin", "clang")
	if runtime.GOOS == "windows" {
		p += ".exe"
	}
	return p
}

func compile(o opts, pins *Pins, g GrammarPin, wasm string) error {
	rt := filepath.Join(o.src, pins.Runtime.ExtractDir, "lib")
	gd := filepath.Join(o.src, g.ExtractDir, filepath.FromSlash(g.GrammarDir))
	args := append([]string{}, clangFlags...)
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
	if out, err := runCmd("", nil, clangPath(o.sdk), args...); err != nil {
		return fmt.Errorf("clang: %w\n%s", err, out)
	}
	return nil
}

// recipeSHA256 binds the fixed recipe (flags, exports, tool invocations, the patch) to one identity string.
func recipeSHA256(pins *Pins, g GrammarPin) string {
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
		if err := os.WriteFile(filepath.Join(dir, n), files[n], 0o644); err != nil {
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
	if o.wasm2go != "" {
		res["wasm2goSHA256"], _ = sumFile(o.wasm2go)
	}
	return json.NewEncoder(os.Stdout).Encode(res)
}
