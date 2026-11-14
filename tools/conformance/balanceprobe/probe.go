// Package balanceprobe drives test-only generated modules; product modules never import it.
package balanceprobe

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// Host stops at the kth progress callback, independently of the machine's speed.
type Host struct{ n, stopAt int }

func (h *Host) Xshould_stop() int32 {
	h.n++
	if h.stopAt > 0 && h.n >= h.stopAt {
		return 1
	}
	return 0
}
func (*Host) Xclock_time_get(int32, int64, int32) int32  { return 52 }
func (*Host) Xfd_close(int32) int32                      { return 52 }
func (*Host) Xfd_seek(int32, int64, int32, int32) int32  { return 52 }
func (*Host) Xfd_write(int32, int32, int32, int32) int32 { return 52 }

type Memory = interface {
	Slice() *[]byte
	Grow(delta, max int64) int64
}

// Module is the shim's parse/export ABI plus the test-only observation exports.
type Module interface {
	X_initialize()
	Xmemory() Memory
	Xmalloc(int32) int32
	Xfree(int32)
	Xtsg_abi_version() int32
	Xtsg_init() int32
	Xtsg_parse(src, n, old int32) int32
	Xtsg_parser_reset()
	Xtsg_root_has_error(int32) int32
	Xtsg_tree_delete(int32)
	Xtsg_export_begin(int32) int32
	Xtsg_export_next() int32
	Xtsg_export_err() int32
	Xtsg_export_buf() int32
	Xtsg_export_end()
	Xtsg_lang_meta() int32
	Xtsg_probe_canceled_balancing() int32
	Xtsg_probe_flag_before_last_reset() int32
	Xtsg_probe_reset_count() int32
	Xtsg_probe_clear()
	Xtsg_probe_parse_noreset(src, n int32) int32
}

type env struct {
	t            *testing.T
	m            Module
	h            *Host
	syms, fields []string
}

func (e *env) block(ptr int32, n int) []byte {
	e.t.Helper()
	b := *e.m.Xmemory().Slice()
	if ptr <= 0 || n < 0 || uint64(uint32(ptr))+uint64(n) > uint64(len(b)) {
		e.t.Fatalf("out-of-range engine block ptr=%d size=%d memory=%d", ptr, n, len(b))
	}
	return b[int(ptr) : int(ptr)+n]
}

func (e *env) put(src []byte) int32 {
	p := e.m.Xmalloc(int32(max(1, len(src))))
	copy(e.block(p, len(src)), src)
	return p
}

func (e *env) readMeta() {
	p := e.m.Xtsg_lang_meta()
	b := e.block(p, 24)
	u := func(off int) int { return int(binary.LittleEndian.Uint32(b[off:])) }
	if string(b[:4]) != "TSGM" || u(4) != 1 {
		e.t.Fatal("bad language metadata header")
	}
	ns, nf, pool := u(12), u(16), u(20)
	if ns < 1 || ns > 65536 || nf < 1 || nf > 65536 || pool > 1<<24 {
		e.t.Fatal("invalid language metadata dimensions")
	}
	syms, fields := 24, 24+4*(ns+1)
	poolAt := fields + 4*(nf+1) + ns
	b = e.block(p, poolAt+pool)
	name := func(tab, i int) string {
		a, z := u(tab+4*i), u(tab+4*(i+1))
		if a > z || z > pool {
			e.t.Fatal("invalid language name offsets")
		}
		return string(b[poolAt+a : poolAt+z])
	}
	for i := range ns {
		e.syms = append(e.syms, name(syms, i))
	}
	for i := range nf {
		e.fields = append(e.fields, name(fields, i))
	}
	e.m.Xfree(p)
}

// dump reads the unchanged product shim's columnar export, retaining the native dump format.
func (e *env) dump(tree int32) []byte {
	e.t.Helper()
	n := int(e.m.Xtsg_export_begin(tree))
	defer e.m.Xtsg_export_end()
	if n <= 0 {
		e.t.Fatal("empty tree export")
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, "#tsg-dump/1 root_has_error=%d nodes=%d\n", e.m.Xtsg_root_has_error(tree), n)
	depth := make([]int, 0, n)
	for {
		k := int(e.m.Xtsg_export_next())
		if k == 0 {
			break
		}
		p := e.m.Xtsg_export_buf()
		h := e.block(p, 16)
		kc := int(binary.LittleEndian.Uint32(h[8:]))
		if string(h[:4]) != "TSGX" || binary.LittleEndian.Uint32(h[4:]) != 1 || kc < k || kc > 1<<20 || k > n-len(depth) {
			e.t.Fatal("invalid chunk export header/count")
		}
		b := e.block(p, 16+33*kc)[16:]
		u16 := func(off, i int) int { return int(binary.LittleEndian.Uint16(b[off*kc+2*i:])) }
		u32 := func(off, i int) uint32 { return binary.LittleEndian.Uint32(b[off*kc+4*i:]) }
		for i := range k {
			par := int(int32(u32(29, i)))
			d := 0
			if par >= 0 && par < len(depth) {
				d = depth[par] + 1
			} else if par != -1 || len(depth) != 0 {
				e.t.Fatal("invalid exported parent index")
			}
			depth = append(depth, d)
			sym, fld, fl := u16(0, i), u16(2, i), b[4*kc+i]
			name := "ERROR"
			if sym != 65535 {
				if sym >= len(e.syms) {
					e.t.Fatal("invalid exported symbol")
				}
				name = e.syms[sym]
			}
			if fld >= len(e.fields) {
				e.t.Fatal("invalid exported field")
			}
			field := "-"
			if fld != 0 {
				field = e.fields[fld]
			}
			bit := func(mask byte) int {
				if fl&mask != 0 {
					return 1
				}
				return 0
			}
			fmt.Fprintf(&out, "%d\t%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d:%d\t%d:%d\n", d, field, name,
				bit(1), bit(2), bit(4), bit(8), bit(16), u32(5, i), u32(9, i), u32(13, i), u32(17, i), u32(21, i), u32(25, i))
		}
	}
	if len(depth) != n || e.m.Xtsg_export_err() != 0 {
		e.t.Fatalf("incomplete export: got %d nodes, want %d, error=%d", len(depth), n, e.m.Xtsg_export_err())
	}
	return out.Bytes()
}

func inputs(lang string) (string, []string) {
	const count = 20000
	switch lang {
	case "json":
		return "[" + strings.Repeat(`{"a":1},`, count-1) + `{"a":1}]`, []string{`{"next":[2,3]}`, `[false,null,"different"]`, `{"broken":`}
	case "golang":
		return "package p\n" + strings.Repeat("var _ = 1\n", count), []string{"package q\nfunc next() int { return 2 }\n", "package r\ntype T struct { S string }\n", "package s\nfunc broken(\n"}
	case "python":
		return strings.Repeat("x = 1\n", count), []string{"def next(x):\n    return x + 2\n", "class Other:\n    pass\n", "def broken(\n"}
	case "typescript":
		return strings.Repeat("const a = 1;\n", count), []string{"function next(x: number) { return x + 2; }\n", "interface Other { text: string }\n", "function broken(\n"}
	case "tsx":
		return strings.Repeat("const a = <div/>;\n", count), []string{"const Next = () => <span>next</span>;\n", "interface Other { text: string }\n", "const broken = <div\n"}
	case "csharp":
		return "class A { void F() {\n" + strings.Repeat("F();\n", count) + "} }\n", []string{"class Next { int F() => 2; }\n", "namespace Other { class B { string S; } }\n", "class Broken { void F(\n"}
	case "tsql":
		return strings.Repeat("SELECT 1;\n", count), []string{"SELECT x FROM next_table WHERE x > 2;\n", "CREATE TABLE other_table (id INT);\n", "SELECT (\n"}
	}
	return "", nil
}

func sweep(n, boundary int) []int {
	set := map[int]bool{}
	add := func(k int) {
		if k >= 1 && k <= n+2 {
			set[k] = true
		}
	}
	for i := range 80 {
		add(1 + i*(n-1)/79)
	}
	for k := boundary - 40; k <= boundary+40; k++ {
		add(k)
	}
	for i := 0; i <= 120; i++ {
		add(boundary + i*(n-boundary)/120)
	}
	for k := n - 60; k <= n+2; k++ {
		add(k)
	}
	list := make([]int, 0, len(set))
	for k := range set {
		list = append(list, k)
	}
	sort.Ints(list)
	return list
}

func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// Run requires a balancing stop and native equality after every positive trial, including boundary search.
func Run(t *testing.T, m Module, h *Host, lang string) {
	t.Helper()
	start := time.Now()
	big, small := inputs(lang)
	if big == "" {
		t.Fatalf("no balancing input for %s", lang)
	}
	exe := filepath.Join(os.Getenv("TSG_NATIVE_EXE_DIR"), lang)
	if os.Getenv("TSG_NATIVE_EXE_DIR") == "" {
		t.Fatal("TSG_NATIVE_EXE_DIR is required for the probe")
	}
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	exeBytes, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if out := os.Getenv("TSG_CONF_OUT"); out != "" {
		dir = filepath.Join(out, "balance", lang)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(name string, b []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prov, err := os.ReadFile("provenance.json")
	if err != nil {
		t.Fatal(err)
	}
	write("provenance.json", prov)
	trials, err := os.Create(filepath.Join(dir, "trials.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer trials.Close()
	enc := json.NewEncoder(trials)
	record := func(v any) {
		t.Helper()
		if err := enc.Encode(v); err != nil {
			t.Fatal(err)
		}
	}
	native := func(name, source string) []byte {
		t.Helper()
		write(name+".src", []byte(source))
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, "dump", filepath.Join(dir, name+".src"))
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		b, err := cmd.Output()
		if err != nil {
			t.Fatalf("native dump %s: %v %s", name, err, stderr.String())
		}
		write(name+".native.dump", b)
		return b
	}
	wantBig := native("big", big)
	wants := make([][]byte, len(small))
	hashes := map[string]string{"big": hash([]byte(big)), "nativeExecutable": hash(exeBytes), "provenance": hash(prov)}
	for i, src := range small {
		name := fmt.Sprintf("next-%d", i)
		wants[i] = native(name, src)
		hashes[name] = hash([]byte(src))
	}
	write("identities.json", mustJSON(t, hashes))
	m.X_initialize()
	if m.Xtsg_abi_version() != 2 || m.Xtsg_init() != 0 {
		t.Fatal("probe initialization/ABI")
	}
	e := &env{t: t, m: m, h: h}
	e.readMeta()
	ptr := e.put([]byte(big))
	defer m.Xfree(ptr)
	h.n, h.stopAt = 0, 0
	tree := m.Xtsg_parse(ptr, int32(len(big)), 0)
	if tree == 0 {
		t.Fatal("baseline returned NULL")
	}
	n := h.n
	gotBig := e.dump(tree)
	m.Xtsg_tree_delete(tree)
	if !bytes.Equal(gotBig, wantBig) {
		write("baseline.go.dump", gotBig)
		t.Fatal("uncancelled probe differs from native")
	}
	if n < 1 {
		t.Fatal("baseline has no progress callbacks")
	}
	boundary, positive, balanced, nextComparisons, mismatches, negative := 0, 0, 0, 0, 0, 0
	defer func() {
		v := map[string]any{"lang": lang, "callbacks": n, "boundary": boundary, "trials": positive, "stopsInBalancing": balanced,
			"nextComparisons": nextComparisons, "mismatches": mismatches, "negativeMismatches": negative, "seconds": time.Since(start).Seconds(),
			"goos": runtime.GOOS, "goarch": runtime.GOARCH, "inputHashes": hashes, "failed": t.Failed()}
		b := mustJSON(t, v)
		write("summary.json", b)
		t.Logf("SUMMARY %s", bytes.TrimSpace(b))
	}()
	parseNext := func(i int) []byte {
		h.n, h.stopAt = 0, 0
		p := e.put([]byte(small[i]))
		defer m.Xfree(p)
		tr := m.Xtsg_parse(p, int32(len(small[i])), 0)
		if tr == 0 {
			return nil
		}
		defer m.Xtsg_tree_delete(tr)
		return e.dump(tr)
	}
	checkNext := func(phase string, k, i int) bool {
		got := parseNext(i)
		nextComparisons++
		ok := bytes.Equal(got, wants[i])
		if !ok {
			mismatches++
			write(fmt.Sprintf("%s-%d.go.dump", phase, k), got)
		}
		record(map[string]any{"phase": phase + "/next", "k": k, "nextCase": i, "equalsNative": ok, "gotSHA256": hash(got), "nativeSHA256": hash(wants[i])})
		return ok
	}
	trial := func(phase string, k int) int32 {
		t.Helper()
		trialStart := time.Now()
		h.n, h.stopAt = 0, k
		m.Xtsg_probe_clear()
		r0 := m.Xtsg_probe_reset_count()
		tr := m.Xtsg_parse(ptr, int32(len(big)), 0)
		before, live, resets := m.Xtsg_probe_flag_before_last_reset(), m.Xtsg_probe_canceled_balancing(), m.Xtsg_probe_reset_count()-r0
		called := h.n
		same := true
		if tr != 0 {
			got := e.dump(tr)
			same = bytes.Equal(got, wantBig)
			if !same {
				write(fmt.Sprintf("%s-%d.complete.go.dump", phase, k), got)
			}
			m.Xtsg_tree_delete(tr)
		}
		positive++
		if before == 1 {
			balanced++
		}
		i := (positive - 1) % len(small)
		nextOK := checkNext(phase, k, i)
		record(map[string]any{"phase": phase, "k": k, "returnedTree": tr != 0, "beforeReset": before, "liveAfter": live, "resetCalls": resets, "callbacks": called,
			"completeEqualsNative": same, "nextCase": i, "nextEqualsNative": nextOK, "ms": float64(time.Since(trialStart).Microseconds()) / 1000})
		if k <= n && (tr != 0 || live != 0 || resets != 1) || k > n && (tr == 0 || resets != 0 || live != 0) || !same || !nextOK {
			t.Fatalf("%s k=%d/%d tree=%v before=%d live=%d resets=%d complete=%v next=%v", phase, k, n, tr != 0, before, live, resets, same, nextOK)
		}
		return before
	}
	lo, hi := 1, n+1
	for lo < hi {
		mid := (lo + hi) / 2
		if trial("search", mid) == 1 {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	boundary = lo
	if boundary > n {
		t.Fatal("no callback reached canceled_balancing; this grammar/input is uncovered")
	}
	for _, k := range sweep(n, boundary) {
		flag := trial("sweep", k)
		if k < boundary && flag != 0 || k >= boundary && k <= n && flag != 1 {
			t.Fatalf("non-monotone balancing boundary at k=%d B=%d flag=%d", k, boundary, flag)
		}
	}
	if balanced == 0 {
		t.Fatal("no stop landed in balancing")
	}
	if lang == "json" {
		for _, k := range []int{boundary, (boundary + n) / 2, n} {
			h.n, h.stopAt = 0, k
			m.Xtsg_probe_clear()
			r0 := m.Xtsg_probe_reset_count()
			tr := m.Xtsg_probe_parse_noreset(ptr, int32(len(big)))
			live, resets := m.Xtsg_probe_canceled_balancing(), m.Xtsg_probe_reset_count()-r0
			if tr != 0 {
				m.Xtsg_tree_delete(tr)
			}
			got := parseNext(0)
			different := !bytes.Equal(got, wants[0])
			write(fmt.Sprintf("negative-%d.go.dump", k), got)
			if different {
				negative++
			}
			record(map[string]any{"phase": "noreset", "k": k, "returnedTree": tr != 0, "liveAfter": live, "resetCalls": resets, "nextDiffersFromNative": different})
			if tr != 0 || live != 1 || resets != 0 || !different {
				t.Fatalf("negative control did not detect the missing reset: k=%d tree=%v live=%d resets=%d differs=%v", k, tr != 0, live, resets, different)
			}
			m.Xtsg_parser_reset()
			if !checkNext("recovery", k, 0) {
				t.Fatal("explicit reset failed to recover after negative control")
			}
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return append(b, '\n')
}
