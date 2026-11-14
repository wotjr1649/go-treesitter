package treesitter

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/wotjr1649/go-treesitter/internal/engine"
)

// A deterministic fake engine.Module. The "grammar" tokenizes: letters/digits -> word, '+' -> anonymous "+",
// "(" ... ")" -> group, anything else -> ERROR. Root = source_file ending at the input length.

const (
	fSrcFile = 1
	fWord    = 2
	fPlus    = 3
	fGroup   = 4
	fAnon    = 6 // anonymous "word": shares a name with symbol 2
)

type fnode struct {
	sym, field     uint16
	flags          uint8
	sb, eb         uint32
	sr, sc, er, ec uint32
	parent         int32
}

type fakeTree struct {
	nodes []fnode
	n     int
}

func buildFake(src []byte) *fakeTree {
	n := len(src)
	row, col := make([]uint32, n+1), make([]uint32, n+1)
	for i := 0; i < n; i++ {
		row[i+1], col[i+1] = row[i], col[i]+1
		if src[i] == '\n' {
			row[i+1], col[i+1] = row[i]+1, 0
		}
	}
	var nodes []fnode
	add := func(parent int32, sym, field uint16, flags uint8, sb, eb int) int32 {
		nodes = append(nodes, fnode{sym: sym, field: field, flags: flags, sb: uint32(sb), eb: uint32(eb),
			sr: row[sb], sc: col[sb], er: row[eb], ec: col[eb], parent: parent})
		return int32(len(nodes) - 1)
	}
	start := n
	for i, c := range src {
		if c != ' ' && c != '\n' && c != '\t' {
			start = i
			break
		}
	}
	if n == 0 {
		start = 0
	}
	root := add(-1, fSrcFile, 0, flagNamed, start, n)
	stack, rootWords := []int32{root}, 0
	isWord := func(c byte) bool { return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' }
	for i := 0; i < n; {
		top := stack[len(stack)-1]
		switch c := src[i]; {
		case c == ' ' || c == '\n' || c == '\t':
			i++
		case isWord(c):
			j := i
			for j < n && isWord(src[j]) {
				j++
			}
			var field uint16
			if top == root {
				if rootWords++; rootWords <= 2 {
					field = uint16(rootWords) // left, right
				}
			}
			add(top, fWord, field, flagNamed, i, j)
			i = j
		case c == '+':
			add(top, fPlus, 0, 0, i, i+1)
			i++
		case c == '(':
			stack = append(stack, add(top, fGroup, 0, flagNamed, i, n))
			i++
		case c == ')' && len(stack) > 1:
			nodes[top].eb, nodes[top].er, nodes[top].ec = uint32(i+1), row[i+1], col[i+1]
			stack = stack[:len(stack)-1]
			i++
		default:
			add(top, uint16(ErrorSymbol), 0, flagNamed|flagError|flagHasError, i, i+1)
			i++
		}
	}
	for i := len(nodes) - 1; i > 0; i-- {
		if nodes[i].flags&flagHasError != 0 {
			nodes[nodes[i].parent].flags |= flagHasError
		}
	}
	return &fakeTree{nodes: nodes, n: n}
}

type fakeMem struct{ b []byte }

func (m *fakeMem) Slice() *[]byte        { return &m.b }
func (m *fakeMem) Grow(_, _ int64) int64 { return -1 }

type fakeModule struct {
	h     *engine.Host
	m     *fakeMem
	brk   int32
	buf   int32
	kc    int
	trees map[int32]*fakeTree
	next  int32

	mu    sync.Mutex
	calls []string

	// script
	abi              int32
	initRC           int32
	badMagic         bool
	rootEndDelta     int32
	dcDelta          int32
	badExportMagic   bool // header is wrong at every export_begin (badMagic: wrong from tsg_init on)
	nullRanges       bool
	committed        int64
	limit            int64
	refused          bool
	released         atomic.Bool // set by the GC goroutine in the leak scenarios
	refusedBegin     bool        // export_begin fails (returns 0) with a refused grow
	exportErr        int32
	exportErrRefused bool
	overrun          bool // export_next yields one node more than the descendant count
	overrunDone      bool
	onCall           func(name string) // runs inside the engine call, on the calling goroutine
	parseHook        func(f *fakeModule, src []byte, old int32) (ret int32, handled bool)
	nextHook         func(f *fakeModule)
	corrupt          func(nodes []fnode)
	ranges           []Range
	halted           *fakeTree // models C: a halted parse resumes unless the parser was reset
	edits            [][10]int32
	lastOld          int32

	exp []fnode
	pos int
}

func (f *fakeModule) rec(s string) {
	f.mu.Lock()
	f.calls = append(f.calls, s)
	f.mu.Unlock()
	if f.onCall != nil {
		f.onCall(s)
	}
}
func (f *fakeModule) count(s string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == s {
			n++
		}
	}
	return n
}
func (f *fakeModule) mem() []byte { return f.m.b }

func (f *fakeModule) X_initialize()          { f.rec("initialize") }
func (f *fakeModule) Xmemory() engine.Memory { return f.m }
func (f *fakeModule) Xmalloc(n int32) int32 {
	f.rec("malloc")
	p := f.brk
	f.brk += (n + 7) &^ 7
	if int(f.brk) > len(f.m.b) {
		return 0
	}
	return p
}
func (f *fakeModule) Xfree(int32)             { f.rec("free") }
func (f *fakeModule) Xtsg_abi_version() int32 { return f.abi }
func (f *fakeModule) Xtsg_init() int32 {
	f.rec("init")
	f.writeHeader(f.badMagic)
	return f.initRC
}

func (f *fakeModule) writeHeader(bad bool) {
	h := f.mem()[f.buf : f.buf+16]
	copy(h, engine.ExportMagic)
	if bad {
		copy(h, "XXXX")
	}
	binary.LittleEndian.PutUint16(h[4:], engine.ExportFormat)
	binary.LittleEndian.PutUint32(h[8:], uint32(f.kc))
}

func (f *fakeModule) register(t *fakeTree) int32 {
	f.next++
	f.trees[f.next] = t
	return f.next
}

func (f *fakeModule) Xtsg_parse(src, n, old int32) int32 {
	f.rec("parse")
	f.lastOld = old
	data := append([]byte(nil), f.mem()[src:src+n]...)
	if f.parseHook != nil {
		if ret, ok := f.parseHook(f, data, old); ok {
			return ret
		}
	}
	if f.halted != nil { // the resume bug: stale tree of the halted parse
		t := f.halted
		f.halted = nil
		return f.register(t)
	}
	return f.register(buildFake(data))
}
func (f *fakeModule) Xtsg_parser_reset() { f.rec("reset"); f.halted = nil }

func (f *fakeModule) tree(h int32) *fakeTree {
	t := f.trees[h]
	if t == nil {
		panic(fmt.Sprintf("fake: unknown tree handle %d", h))
	}
	return t
}
func (f *fakeModule) Xtsg_root_end(t int32) int32 { return int32(f.tree(t).n) + f.rootEndDelta }
func (f *fakeModule) Xtsg_descendant_count(t int32) int32 {
	return int32(len(f.tree(t).nodes)) + f.dcDelta
}
func (f *fakeModule) Xtsg_tree_edit(t, sb, oeb, neb, sr, sc, oer, oec, ner, nec int32) {
	f.rec("edit")
	f.tree(t)
	f.edits = append(f.edits, [10]int32{t, sb, oeb, neb, sr, sc, oer, oec, ner, nec})
}
func (f *fakeModule) Xtsg_tree_delete(t int32) {
	f.rec("delete")
	f.tree(t)
	delete(f.trees, t)
}
func (f *fakeModule) Xtsg_changed_ranges(old, neu int32) int32 {
	f.rec("changed_ranges")
	f.tree(old)
	f.tree(neu)
	if f.nullRanges {
		return 0
	}
	p := f.Xmalloc(int32(4 + 24*len(f.ranges)))
	le := binary.LittleEndian
	le.PutUint32(f.mem()[p:], uint32(len(f.ranges)))
	for i, r := range f.ranges {
		for j, v := range []uint32{r.StartByte, r.EndByte, r.StartPoint.Row, r.StartPoint.Column, r.EndPoint.Row, r.EndPoint.Column} {
			le.PutUint32(f.mem()[int(p)+4+24*i+4*j:], v)
		}
	}
	return p
}
func (f *fakeModule) Xtsg_root_has_error(t int32) int32 {
	return int32(f.tree(t).nodes[0].flags & flagHasError >> 4)
}

func (f *fakeModule) Xtsg_export_begin(t int32) int32 {
	f.rec("export_begin")
	f.exp = append([]fnode(nil), f.tree(t).nodes...)
	if f.corrupt != nil {
		f.corrupt(f.exp)
	}
	f.pos = 0
	f.writeHeader(f.badMagic || f.badExportMagic)
	if f.refusedBegin {
		f.refused = true
		return 0
	}
	return int32(len(f.exp))
}
func (f *fakeModule) Xtsg_export_next() int32 {
	f.rec("export_next")
	if f.nextHook != nil {
		f.nextHook(f)
	}
	k := min(len(f.exp)-f.pos, f.kc)
	if k <= 0 {
		if f.overrun && !f.overrunDone {
			f.overrunDone = true
			return 1
		}
		return 0
	}
	base, kc, le := int(f.buf)+16, f.kc, binary.LittleEndian
	for i := 0; i < k; i++ {
		nd := f.exp[f.pos+i]
		le.PutUint16(f.mem()[base+2*i:], nd.sym)
		le.PutUint16(f.mem()[base+2*kc+2*i:], nd.field)
		f.mem()[base+4*kc+i] = nd.flags
		for col, v := range map[int]uint32{5: nd.sb, 9: nd.eb, 13: nd.sr, 17: nd.sc, 21: nd.er, 25: nd.ec, 29: uint32(nd.parent)} {
			le.PutUint32(f.mem()[base+col*kc+4*i:], v)
		}
	}
	f.pos += k
	return int32(k)
}
func (f *fakeModule) Xtsg_export_err() int32 {
	if f.exportErrRefused {
		f.refused = true
	}
	return f.exportErr
}
func (f *fakeModule) Xtsg_export_buf() int32 { return f.buf }
func (f *fakeModule) Xtsg_export_end()       { f.rec("export_end") }

// recoverCall runs fn and turns any panic into *engine.Trap (same contract as engine.Call).
func recoverCall(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &engine.Trap{Value: r}
		}
	}()
	fn()
	return nil
}

// fakeLang is a Language whose instances are fakeModules; setup runs on each new module (mods already holds it).
type fakeLang struct {
	mu    sync.Mutex // New may be called from several goroutines (ParserPool tests)
	lang  *Language
	mods  []*fakeModule
	setup func(f *fakeModule)
	abi   uint32
}

func newFakeLang(setup func(fl *fakeLang, f *fakeModule)) *fakeLang {
	fl := &fakeLang{abi: engine.ABIVersion}
	meta := engine.LangMeta{
		LanguageABI: 15,
		Symbols:     []string{"end", "source_file", "word", "+", "group", "_hidden", "word"},
		SymbolFlags: []uint8{0, 3, 3, 2, 3, 0, 2},
		Fields:      []string{"", "left", "right"},
	}
	g := &engine.Grammar{Name: "fake", ABI: engine.ABIVersion, Meta: meta}
	g.New = func(h *engine.Host) engine.Module {
		f := &fakeModule{h: h, m: &fakeMem{b: make([]byte, 4<<20)}, brk: 1 << 16, kc: 4, trees: map[int32]*fakeTree{},
			abi: engine.ABIVersion, committed: 1 << 20}
		f.buf = f.brk
		f.brk += 16 + 33*int32(f.kc)
		fakeReg.Store(f.m.Slice(), f)
		fl.mu.Lock()
		fl.mods = append(fl.mods, f)
		if setup != nil {
			setup(fl, f)
		}
		fl.mu.Unlock()
		return f
	}
	fl.lang = NewLanguage(g)
	return fl
}

func (fl *fakeLang) cur() *fakeModule { return fl.mods[len(fl.mods)-1] }

var fakeReg sync.Map // *[]byte -> *fakeModule

func regOf(mem *[]byte) *fakeModule { v, _ := fakeReg.Load(mem); return v.(*fakeModule) }

func TestMain(m *testing.M) {
	ops = engineOps{
		setLimit: func(mem *[]byte, n int64) error {
			if n < 1<<20 {
				return errors.New("below the fake provider's minimum")
			}
			regOf(mem).limit = n
			return nil
		},
		refused:      func(mem *[]byte) bool { return regOf(mem).refused },
		clearRefused: func(mem *[]byte) { regOf(mem).refused = false },
		committed:    func(mem *[]byte) int64 { return regOf(mem).committed },
		release:      func(mem *[]byte) { regOf(mem).released.Store(true) },
		call:         recoverCall,
	}
	os.Exit(m.Run())
}
