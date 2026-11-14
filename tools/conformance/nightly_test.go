package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	treesitter "github.com/wotjr1649/go-treesitter"
)

// TestNightlyDiff is the time-budgeted differential fuzzer of .github/workflows/nightly-diff.yml. For each grammar it
// draws random cases until the budget is used up and compares the public API with the native C executables byte for
// byte: the fresh dump of the old text, the fresh dump of the new text, the incremental dump after Tree.Edit, and the
// changed ranges. A case is a corpus input of the CI data tree (TSG_DATA_DIR, from tools/native-ref/ciconf) with 0-4
// random byte mutations, plus one random edit (insert, delete or replace; not always on a UTF-8 boundary).
//
//	TSG_NIGHTLY_BUDGET   time per grammar, e.g. 4m (unset: the test skips)
//	TSG_NIGHTLY_SEED     base seed (the workflow uses the UTC date, YYYYMMDD); every case is derived from
//	                     (seed, language, iteration), so TSG_NIGHTLY_REPLAY=<lang>:<iteration> reruns exactly one case
//	TSG_NATIVE_EXE_DIR   native-ref output directory
//	TSG_CONF_OUT         where reproducers go: $TSG_CONF_OUT/nightly/<lang>-<iteration>/
//
// On the first mismatch of a grammar the case is shrunk (bounded) and written with both outputs, the seed and the
// commands; the test then fails and moves on to the next grammar. incremental != fresh is not a finding by itself (the
// known tsql cases differ in native C as well): the incremental output is compared with the native incremental output.
func TestNightlyDiff(t *testing.T) {
	budgetS := os.Getenv("TSG_NIGHTLY_BUDGET")
	if budgetS == "" {
		t.Skip("TSG_NIGHTLY_BUDGET is not set")
	}
	budget, err := time.ParseDuration(budgetS)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := strconv.ParseUint(os.Getenv("TSG_NIGHTLY_SEED"), 10, 64)
	if err != nil {
		t.Fatalf("TSG_NIGHTLY_SEED: %v", err)
	}
	root := dataRoot(t)
	replayLang, replayIt, replay := strings.Cut(os.Getenv("TSG_NIGHTLY_REPLAY"), ":")
	for _, lang := range langs(t) {
		if replay && lang != replayLang {
			continue
		}
		t.Run(lang, func(t *testing.T) {
			exe, ok := nativeExe(lang)
			if !ok {
				t.Fatalf("no native executable for %s in TSG_NATIVE_EXE_DIR", lang)
			}
			files, _ := filepath.Glob(filepath.Join(root, "corpus", lang, "*.in"))
			if len(files) == 0 {
				t.Fatalf("no corpus inputs under %s/corpus/%s", root, lang)
			}
			var seeds [][]byte
			for _, f := range files {
				seeds = append(seeds, readFile(t, f))
			}
			nd := &nightly{t: t, lang: lang, exe: exe, p: newParser(t, lang), tmp: t.TempDir()}
			h := fnv.New64a()
			h.Write([]byte(lang))
			stream := h.Sum64()
			start, deadline := time.Now(), time.Now().Add(budget)
			it, first := 0, 0
			if replay {
				first, _ = strconv.Atoi(replayIt)
				it = first
			}
			var cases int
			for ; replay && it == first || !replay && time.Now().Before(deadline); it++ {
				r := rand.New(rand.NewPCG(seed, stream+uint64(it)*0x9e3779b97f4a7c15))
				c := genCase(r, seeds)
				cases++
				if fs := nd.check(c); len(fs) > 0 {
					t.Errorf("%s iteration %d: %s", lang, it, fs[0].detail)
					nd.report(seed, it, c, fs[0], !replay)
					break
				}
			}
			summary(t, map[string]any{"lang": lang, "seed": seed, "cases": cases, "budget": budgetS, "goos": runtime.GOOS, "goarch": runtime.GOARCH,
				"seconds": time.Since(start).Seconds(), "mismatch": t.Failed()})
		})
	}
}

// kase is one comparison: new = old[:start] + ins + old[oe:].
type kase struct {
	old       []byte
	start, oe int
	ins       []byte
	noEdit    bool // shrinking a fresh-parse finding: the edit does not matter
}

func (c kase) next() []byte {
	return append(append(append([]byte{}, c.old[:c.start]...), c.ins...), c.old[c.oe:]...)
}

type finding struct {
	kind, detail  string
	goOut, native []byte
}

const maxCase = 1 << 16

var tokens = []string{" ", "\n", "\n\n", "\t", ";", "}", ")", "{", "(", "[", "]", "0", "\"", "'", "/*", "*/", "//", "#", "<", ">", "\\", "a b", ",", "=", "@", "$"}

func genCase(r *rand.Rand, seeds [][]byte) kase {
	src := append([]byte{}, seeds[r.IntN(len(seeds))]...)
	for ops := r.IntN(5); ops > 0 && len(src) < maxCase; ops-- {
		n := len(src)
		switch r.IntN(6) {
		case 0: // replace one byte
			if n > 0 {
				src[r.IntN(n)] = byte(r.IntN(256))
			}
		case 1: // insert a token or random bytes
			src = splice(src, r.IntN(n+1), 0, randBytes(r, 8))
		case 2: // delete a range
			if n > 0 {
				a := r.IntN(n)
				src = splice(src, a, 1+r.IntN(min(32, n-a)), nil)
			}
		case 3: // duplicate a range
			if n > 0 {
				a := r.IntN(n)
				b := min(n, a+1+r.IntN(64))
				src = splice(src, r.IntN(n+1), 0, append([]byte{}, src[a:b]...))
			}
		case 4: // splice in a slice of another corpus input
			o := seeds[r.IntN(len(seeds))]
			if len(o) > 0 {
				a := r.IntN(len(o))
				src = splice(src, r.IntN(n+1), 0, o[a:min(len(o), a+1+r.IntN(128))])
			}
		case 5: // truncate
			src = src[:r.IntN(n+1)]
		}
	}
	n := len(src)
	c := kase{old: src, start: r.IntN(n + 1)}
	if r.IntN(5) > 0 { // mostly on a UTF-8 boundary, like the ciconf edits
		for c.start > 0 && c.start < n && src[c.start]&0xC0 == 0x80 {
			c.start--
		}
	}
	c.oe = c.start
	if kind := r.IntN(3); kind > 0 && c.start < n { // 0 insert, 1 delete, 2 replace
		c.oe = min(n, c.start+1+r.IntN(16))
		if kind == 2 {
			c.ins = randBytes(r, 16)
		}
	} else {
		c.ins = randBytes(r, 16)
	}
	return c
}

func randBytes(r *rand.Rand, max int) []byte {
	if r.IntN(8) == 0 {
		b := make([]byte, 1+r.IntN(max))
		for i := range b {
			b[i] = byte(r.IntN(256))
		}
		return b
	}
	return []byte(tokens[r.IntN(len(tokens))])
}

func splice(src []byte, at, del int, ins []byte) []byte {
	return append(append(append([]byte{}, src[:at]...), ins...), src[at+del:]...)
}

type nightly struct {
	t    *testing.T
	lang string
	exe  string
	p    *treesitter.Parser
	tmp  string
}

func (n *nightly) native(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(bg, 2*time.Minute)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, n.exe, args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// check runs every comparison for c and returns the findings, in a fixed order.
func (n *nightly) check(c kase) (out []finding) {
	oldP, newP := filepath.Join(n.tmp, "old.bin"), filepath.Join(n.tmp, "new.bin")
	next := c.next()
	if err := os.WriteFile(oldP, c.old, 0o644); err != nil {
		n.t.Fatal(err)
	}
	if err := os.WriteFile(newP, next, 0o644); err != nil {
		n.t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(bg, 60*time.Second)
	defer cancel()
	cmp := func(kind string, goOut []byte, goErr error, nativeArgs ...string) {
		nat, nerr := n.native(nativeArgs...)
		switch {
		case nerr != nil:
			out = append(out, finding{kind + "-native-error", "native " + strings.Join(nativeArgs[:1], " ") + " failed: " + nerr.Error(), goOut, nil})
		case goErr != nil:
			out = append(out, finding{kind + "-go-error", "Go failed where native succeeded: " + goErr.Error(), nil, nat})
		case !bytes.Equal(goOut, nat):
			out = append(out, finding{kind, kind + " differs: " + firstDiff(goOut, nat), goOut, nat})
		}
	}
	dump := func(src []byte) ([]byte, error) {
		s, err := n.p.ParseSnapshot(ctx, src)
		if err != nil {
			return nil, err
		}
		return Dump(s), nil
	}
	d, err := dump(c.old)
	cmp("fresh-old", d, err, "dump", oldP)
	if c.noEdit {
		return out
	}
	d, err = dump(next)
	cmp("fresh-new", d, err, "dump", newP)

	args := []string{oldP, newP, strconv.Itoa(c.start), strconv.Itoa(c.oe), strconv.Itoa(c.start + len(c.ins))}
	incDump, ranges, err := n.incremental(ctx, c, next)
	cmp("incremental", incDump, err, append([]string{"edit"}, args...)...)
	var rs bytes.Buffer
	for _, rg := range ranges {
		fmt.Fprintf(&rs, "%d %d %d %d %d %d\n", rg.StartByte, rg.EndByte, rg.StartPoint.Row, rg.StartPoint.Column, rg.EndPoint.Row, rg.EndPoint.Column)
	}
	if err == nil { // a ranges comparison after an incremental failure would only repeat it
		cmp("ranges", rs.Bytes(), nil, append([]string{"changed-ranges"}, args...)...)
	}
	return out
}

func (n *nightly) incremental(ctx context.Context, c kase, next []byte) (dump []byte, ranges []treesitter.Range, err error) {
	old, err := n.p.Parse(ctx, c.old, nil)
	if err != nil {
		return nil, nil, err
	}
	defer old.Close()
	if err := old.Edit(treesitter.EditFor(c.old, next, uint32(c.start), uint32(c.oe), uint32(c.start+len(c.ins)))); err != nil {
		return nil, nil, err
	}
	inc, err := n.p.Parse(ctx, next, old)
	if err != nil {
		return nil, nil, err
	}
	defer inc.Close()
	s, err := inc.Snapshot(ctx)
	if err != nil {
		return nil, nil, err
	}
	ranges, err = inc.ChangedRanges(old)
	return Dump(s), ranges, err
}

// shrink reduces c while a finding of the same kind persists: it removes chunks of the old text that do not overlap
// the edit (for a fresh-parse finding the edit is dropped and any chunk may go), then halves the inserted text. At most
// 400 candidate checks and one minute.
func (n *nightly) shrink(c kase, kind string) kase {
	still := func(c kase) bool {
		for _, f := range n.check(c) {
			if f.kind == kind {
				return true
			}
		}
		return false
	}
	if strings.HasPrefix(kind, "fresh-old") {
		c = kase{old: c.old, noEdit: true}
	}
	deadline, budget := time.Now().Add(time.Minute), 400
	try := func(cand kase) bool {
		if budget == 0 || time.Now().After(deadline) {
			return false
		}
		budget--
		return still(cand)
	}
	for size := max(1, len(c.old)/2); size >= 1; size /= 2 {
		for a := 0; a+size <= len(c.old); {
			cand := kase{old: splice(c.old, a, size, nil), start: c.start, oe: c.oe, ins: c.ins, noEdit: c.noEdit}
			switch {
			case c.noEdit, a >= c.oe:
			case a+size <= c.start:
				cand.start, cand.oe = c.start-size, c.oe-size
			default:
				a += size
				continue
			}
			if try(cand) {
				c = cand
			} else {
				a += size
			}
		}
	}
	for len(c.ins) > 1 {
		cand := c
		cand.ins = c.ins[:len(c.ins)/2]
		if !try(cand) {
			break
		}
		c = cand
	}
	return c
}

func (n *nightly) report(seed uint64, it int, c kase, f finding, doShrink bool) {
	out := outDir()
	if out == "" {
		return
	}
	orig := c
	if doShrink {
		c = n.shrink(c, f.kind)
		for _, g := range n.check(c) { // outputs of the shrunk case
			if g.kind == f.kind {
				f = g
			}
		}
	}
	dir := filepath.Join(out, "nightly", fmt.Sprintf("%s-%d", n.lang, it))
	os.MkdirAll(dir, 0o755)
	w := func(name string, b []byte) { os.WriteFile(filepath.Join(dir, name), b, 0o644) }
	next := c.next()
	w("old.bin", c.old)
	w("new.bin", next)
	w("go.out", f.goOut)
	w("native.out", f.native)
	w("original-old.bin", orig.old)
	nativeCmd := "dump old.bin"
	switch {
	case strings.HasPrefix(f.kind, "fresh-new"):
		nativeCmd = "dump new.bin"
	case strings.HasPrefix(f.kind, "incremental"):
		nativeCmd = "edit old.bin new.bin"
	case strings.HasPrefix(f.kind, "ranges"):
		nativeCmd = "changed-ranges old.bin new.bin"
	}
	edit := ""
	if !strings.HasPrefix(f.kind, "fresh") {
		edit = fmt.Sprintf(" %d %d %d", c.start, c.oe, c.start+len(c.ins))
	}
	repro := fmt.Sprintf("language:   %s\nfinding:    %s\ndetail:     %s\nseed:       %d (TSG_NIGHTLY_SEED)\niteration:  %d\nplatform:   %s/%s\n"+
		"edit:       start=%d old_end=%d new_end=%d (shrunk=%v; original input %d bytes, shrunk %d bytes)\n\n"+
		"native:     %s %s%s      # go.out and native.out are the two outputs compared\n"+
		"replay:     TSG_NIGHTLY_BUDGET=1m TSG_NIGHTLY_SEED=%d TSG_NIGHTLY_REPLAY=%s:%d TSG_DATA_DIR=<ciconf data> TSG_NATIVE_EXE_DIR=<native exe dir> go test -run TestNightlyDiff -v . (in tools/conformance, CGO_ENABLED=0)\n",
		n.lang, f.kind, f.detail, seed, it, runtime.GOOS, runtime.GOARCH, c.start, c.oe, c.start+len(c.ins), doShrink, len(orig.old), len(c.old),
		filepath.Base(n.exe), nativeCmd, edit, seed, n.lang, it)
	w("repro.txt", []byte(repro))
	meta, _ := json.Marshal(map[string]any{"lang": n.lang, "seed": seed, "iteration": it, "kind": f.kind, "start": c.start, "oldEnd": c.oe, "newEnd": c.start + len(c.ins)})
	w("repro.json", meta)
}
