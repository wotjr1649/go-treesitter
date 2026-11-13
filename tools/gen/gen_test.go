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
