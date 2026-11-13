package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fakeGen = "package x\n\nimport (\n\t\"encoding/binary\"\n\t\"math\"\n\t\"unsafe\"\n)\n\n" +
	"func memory_grow(mem *[]byte, delta, max int64) int64 {\n\tbuf := *mem\n\treturn int64(len(buf))\n}\n\nfunc other() {}\n"

func TestPatchMemoryGrow(t *testing.T) {
	orig := fakeGen[strings.Index(fakeGen, "func memory_grow"):strings.Index(fakeGen, "func other")]
	orig = strings.TrimRight(orig, "\n") + "\n"
	got, err := patchMemoryGrow(fakeGen, sumBytes([]byte(orig)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "return engine.MemoryGrow(mem, delta, max)") || strings.Contains(got, "buf := *mem") {
		t.Fatal("function not replaced")
	}
	// the engine import lands in sorted position (after encoding/binary, before math)
	if !strings.Contains(got, "\t\"encoding/binary\"\n\t\""+enginePath+"\"\n\t\"math\"\n") {
		t.Fatalf("import not sorted in:\n%s", got)
	}
	if _, err := patchMemoryGrow(fakeGen, strings.Repeat("0", 64)); err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("a changed original body must fail, got %v", err)
	}
	if _, err := patchMemoryGrow("package x\n", "x"); err == nil {
		t.Fatal("missing memory_grow must fail")
	}
}

func TestCheckImports(t *testing.T) {
	ok := "type Xtsg = interface {\n\tXshould_stop() int32\n}\ntype Xwasi_snapshot_preview1 = interface {\n\tXfd_close(v0 int32) int32\n}\n" +
		"type Memory = interface {\n\tSlice() *[]byte\n}\n"
	if err := checkImports(ok); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		"type Xwasi_snapshot_preview1 = interface {\n\tXpoll_oneoff(v0 int32) int32\n}\n",
		"type Xenv = interface {\n\tXabort()\n}\n",
	} {
		if err := checkImports(bad); err == nil {
			t.Fatalf("unstubbed import accepted: %q", bad)
		}
	}
}

func TestParseMeta(t *testing.T) {
	le := binary.LittleEndian
	u := func(v ...uint32) (b []byte) {
		for _, x := range v {
			b = le.AppendUint32(b, x)
		}
		return
	}
	// symbols "end" and "a" (pool "enda"), fields "" and ""
	blk := append([]byte("TSGM"), u(1, 14, 2, 2, 4)...)
	blk = append(blk, u(0, 3, 4)...) // symOff
	blk = append(blk, u(4, 4, 4)...) // fldOff
	blk = append(blk, 3, 7)          // symFlags
	blk = append(blk, "enda"...)
	if _, err := parseMeta(blk); err != nil {
		t.Fatal(err)
	}
	m, err := parseMeta(append(append([]byte{}, blk...), 0))
	if err == nil {
		t.Fatalf("trailing byte accepted: %+v", m)
	}
	blk[0] = 'X'
	if _, err := parseMeta(blk); err == nil {
		t.Fatal("bad magic accepted")
	}
}

func TestTreeSHA256IsPathAndContentBound(t *testing.T) {
	d := t.TempDir()
	write := func(name, s string) {
		p := filepath.Join(d, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(s), 0o644)
	}
	write("src/a.c", "1")
	write("src/sub/b.h", "2")
	h1, err := treeSHA256(d, []string{"src"})
	if err != nil {
		t.Fatal(err)
	}
	if h2, _ := treeSHA256(d, []string{"src"}); h1 != h2 {
		t.Fatal("not deterministic")
	}
	write("src/sub/b.h", "3")
	if h3, _ := treeSHA256(d, []string{"src"}); h1 == h3 {
		t.Fatal("content change not detected")
	}
	if _, err := treeSHA256(d, []string{"missing"}); err == nil {
		t.Fatal("missing dir accepted")
	}
}

func TestRecipes(t *testing.T) {
	pins := &Pins{Tools: ToolPins{Binaryen: "b"}}
	if got := recipes["v0"].recipeLines("v0", pins); got != "" {
		t.Fatalf("v0 must not change the baseline recipe hash, got %q", got)
	}
	for name, r := range recipes {
		if _, err := lookupRecipe(name); err != nil {
			t.Fatal(err)
		}
		if name != "v0" && !strings.Contains(r.recipeLines(name, pins), "recipe="+name) {
			t.Fatalf("%s: recipe name not bound into the hash", name)
		}
		if r.embed != (len(r.wasm2goFlags()) == 2) || (r.opt != "") != strings.Contains(r.recipeLines(name, pins), "wasm-opt") {
			t.Fatalf("%s: flags and hash lines disagree", name)
		}
	}
	if _, err := lookupRecipe("nope"); err == nil {
		t.Fatal("unknown recipe accepted")
	}
}

func TestGoModModes(t *testing.T) {
	dev := goMod("json", "")
	if !strings.Contains(dev, "require github.com/wotjr1649/go-treesitter v0.0.0\n") || !strings.Contains(dev, "replace github.com/wotjr1649/go-treesitter => ../..\n") {
		t.Fatalf("dev go.mod lost its local form:\n%s", dev)
	}
	rel := goMod("json", "v0.1.0")
	want := "module github.com/wotjr1649/go-treesitter/grammars/json\n\ngo 1.27.2\n\nrequire github.com/wotjr1649/go-treesitter v0.1.0\n"
	if rel != want {
		t.Fatalf("release go.mod:\n%s\nwant:\n%s", rel, want)
	}
	if strings.Contains(rel, "replace") || strings.Contains(rel, "v0.0.0") {
		t.Fatal("release go.mod must have neither replace nor the placeholder version")
	}
	for _, ok := range []string{"v0.1.0", "v1.22.333", "v0.1.0-rc.1"} {
		if !rootVersionRx.MatchString(ok) {
			t.Errorf("%s rejected", ok)
		}
	}
	for _, bad := range []string{"", "0.1.0", "v0.1", "v0.1.0 ", "v0.1.0\nreplace x => y", "latest", "v0.0.0+incompatible"} {
		if rootVersionRx.MatchString(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

// The pinned inputs must stay host independent: archives and versions, one entry per supported platform.
func TestPinsPlan(t *testing.T) {
	pins, err := loadPins("pins.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, plat := range []string{"windows-amd64", "linux-amd64"} {
		lines, err := pins.plan(plat)
		if err != nil {
			t.Fatalf("%s: %v", plat, err)
		}
		kinds := map[string]int{}
		for _, l := range lines {
			f := strings.Split(l, "\t")
			if len(f) != 5 {
				t.Fatalf("%s: malformed plan line %q", plat, l)
			}
			kinds[f[0]]++
			if f[0] != "goinstall" && (!strings.HasPrefix(f[2], "https://") || len(f[3]) != 64) {
				t.Fatalf("%s: unpinned download %q", plat, l)
			}
		}
		// sdk + binaryen, wasm2go, runtime + 6 distinct grammar repositories (typescript and tsx share one archive)
		if kinds["tool"] != 2 || kinds["goinstall"] != 1 || kinds["source"] != 7 {
			t.Fatalf("%s: plan kinds %v", plat, kinds)
		}
	}
	if _, err := pins.plan("plan9-amd64"); err == nil {
		t.Fatal("a platform without pinned archives must fail")
	}
	if pins.Tools.Wasm2goSum == "" || !strings.HasPrefix(pins.Tools.Wasm2goSum, "h1:") {
		t.Fatalf("wasm2go go.sum hash not pinned: %q", pins.Tools.Wasm2goSum)
	}
}

func TestTarballURL(t *testing.T) {
	c := strings.Repeat("a", 40)
	got, err := tarballURL("https://github.com/tree-sitter/tree-sitter-json", c)
	if err != nil || got != "https://api.github.com/repos/tree-sitter/tree-sitter-json/tarball/"+c {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := tarballURL("https://example.com/x/y", c); err == nil {
		t.Fatal("non-GitHub repository accepted")
	}
	if _, err := tarballURL("https://github.com/x/y", "abc"); err == nil {
		t.Fatal("short commit accepted")
	}
}

// The recipe hash binds tool versions, not host binaries: two hosts with different binaries must agree.
func TestRecipeHashIsHostIndependent(t *testing.T) {
	pins, err := loadPins("pins.json")
	if err != nil {
		t.Fatal(err)
	}
	a := recipes["v2-O3"].recipeLines("v2-O3", pins)
	pins.Tools.LocalBinarySHA256 = map[string]map[string]string{"linux-amd64": {"wasm-opt": strings.Repeat("0", 64)}}
	if b := recipes["v2-O3"].recipeLines("v2-O3", pins); a != b || strings.Contains(a, pins.Tools.LocalBinarySHA256["linux-amd64"]["wasm-opt"]) {
		t.Fatal("recipe lines depend on a host binary hash")
	}
}

// wasi-sdk 34 reports different clang strings per host for one LLVM commit (CI run 43506423917); the identity keeps
// only the number and the commit.
func TestToolVersionParsing(t *testing.T) {
	const c = "895aa2c896ada719451be2e3673c83da8ddf1141"
	for _, line := range []string{
		"clang version 23.1.0-rc3 (https://github.com/llvm/llvm-project " + c + ")",
		"clang version 23.1.0-wasi-sdk (https://github.com/llvm/llvm-project " + c + ")",
	} {
		v, got, err := parseClangVersion(line)
		if err != nil || v != "23.1.0" || got != c {
			t.Fatalf("%q -> %q %q %v", line, v, got, err)
		}
	}
	for _, bad := range []string{"", "clang version 23.1.0", "gcc version 13.2.0 (x " + c + ")", "clang version 23.1.0-x (https://x short)"} {
		if _, _, err := parseClangVersion(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	for line, want := range map[string]string{"wasm-opt version 133 (version_133)": "133", "wasm-opt version 133": "133", "wasm-opt version 133-linux (x)": ""} {
		got, err := parseWasmOptVersion(line)
		if (err == nil) != (want != "") || got != want {
			t.Errorf("%q -> %q %v", line, got, err)
		}
	}
}

func TestPinsHostIndependentStrings(t *testing.T) {
	pins, err := loadPins("pins.json")
	if err != nil {
		t.Fatal(err)
	}
	if pins.Tools.Clang != "23.1.0" || len(pins.Tools.LLVMCommit) != 40 || pins.Tools.Binaryen != "133" {
		t.Fatalf("tool pins carry host strings: %+v", pins.Tools)
	}
}
