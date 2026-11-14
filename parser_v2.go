package treesitter

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"unicode/utf8"
	"unsafe"
	"weak"

	"github.com/wotjr1649/go-treesitter/internal/engine"
)

// Parser owns one engine instance (a translated wasm module, single-threaded) and the C trees living in it. A Parser
// and all its Trees are one confinement unit: use them from one goroutine at a time (misuse is detected best-effort
// and reported as InvalidInput). Snapshots and Nodes are immutable and need no such care.
type Parser struct{ c *core }

// core is the state shared by a Parser and its Trees. Trees point at the Parser (keeping it reachable); cleanups hold
// only the core.
type core struct {
	lang *Language
	cfg  config

	inst    *instance          // nil only after a failed recreate; the next operation retries
	gen     uint64             // bumped whenever the instance is dropped: invalidates every Tree
	live    int                // trees alive in the current instance
	ptrs    map[int32]struct{} // handles of those trees, so a pool can free them all (dropTrees)
	recycle bool               // committed memory passed WithRecycleAboveBytes
	closed  bool
	busy    atomic.Bool

	mu            sync.Mutex // guards cleanup queues only: the GC goroutine enqueues, the owner drains
	leaked        []leakedTree
	leakedQueries []queryRetirement
}

type instance struct {
	m       engine.Module
	h       *engine.Host
	mem     *[]byte // dereference at every use: the slice header changes on grow
	queries map[weak.Pointer[Query]]int32
}

type leakedTree struct {
	ptr int32
	gen uint64
}

type leakArg struct {
	c   *core
	ptr int32
	gen uint64
}

// NewParser creates the engine instance now (not lazily). It returns a *ParseError with IncompatibleLanguage when the
// grammar module was generated for another engine ABI.
func NewParser(lang *Language, opts ...Option) (*Parser, error) {
	if lang == nil || lang.g == nil || lang.g.New == nil {
		return nil, perr(InvalidInput, "nil or empty language", nil)
	}
	var cfg config
	for _, o := range opts {
		if err := o(&cfg); err != nil {
			return nil, err
		}
	}
	c := &core{lang: lang, cfg: cfg, gen: 1}
	if err := c.recreate(); err != nil {
		return nil, err
	}
	p := &Parser{c}
	runtime.AddCleanup(p, func(c *core) { // a leaked Parser still returns its memory; nothing else can use it any more
		if c.inst != nil && !c.closed {
			ops.release(c.inst.mem)
		}
	}, c)
	return p, nil
}

func (c *core) recreate() (err error) {
	in, e := c.newInstance()
	if e != nil {
		c.inst = nil
		return e
	}
	c.inst = in
	return nil
}

func (c *core) newInstance() (*instance, *ParseError) {
	g := c.lang.g
	if g.ABI != 1 && g.ABI != engine.ABIVersion {
		return nil, perr(IncompatibleLanguage, fmt.Sprintf("grammar module %q has engine ABI %d, this root module speaks %d", g.Name, g.ABI, engine.ABIVersion), nil)
	}
	in := &instance{h: &engine.Host{}}
	fail := func(o Outcome, reason string, cause error) (*instance, *ParseError) {
		if in.mem != nil {
			ops.release(in.mem)
		}
		return nil, perr(o, reason, cause)
	}
	trap := func(what string, err error) (*instance, *ParseError) {
		if in.mem != nil && ops.refused(in.mem) {
			return fail(ResourceLimit, what+": memory.grow refused", err)
		}
		return fail(EngineFault, what+": "+err.Error(), err)
	}
	if err := ops.call(func() { in.m = g.New(in.h); in.mem = in.m.Xmemory().Slice() }); err != nil {
		return trap("instantiate", err)
	}
	if c.cfg.maxMemory > 0 {
		if err := ops.setLimit(in.mem, c.cfg.maxMemory); err != nil {
			return fail(InvalidInput, "WithMaxMemoryBytes: "+err.Error(), err)
		}
	}
	var ver, rc int32
	if err := ops.call(in.m.X_initialize); err != nil {
		return trap("initialize", err)
	}
	if err := ops.call(func() { ver = in.m.Xtsg_abi_version() }); err != nil {
		return trap("tsg_abi_version", err)
	}
	if uint32(ver) != g.ABI {
		return fail(IncompatibleLanguage, fmt.Sprintf("module reports engine ABI %d, expected %d", ver, g.ABI), nil)
	}
	if ver == 2 {
		if _, ok := in.m.(engine.QueryModule); !ok {
			return fail(IncompatibleLanguage, "ABI 2 module lacks query exports", nil)
		}
	}
	if err := ops.call(func() { rc = in.m.Xtsg_init() }); err != nil {
		return trap("tsg_init", err)
	}
	if rc != 0 {
		return fail(IncompatibleLanguage, "the runtime rejected the language (language ABI)", nil)
	}
	var buf int32 // tsg_init writes the export header; a module this root cannot read is rejected now
	if err := ops.call(func() { buf = in.m.Xtsg_export_buf() }); err != nil {
		return trap("tsg_export_buf", err)
	}
	if _, e := parseExportHeader(*in.mem, buf); e != nil {
		return fail(e.Outcome, e.Reason, nil)
	}
	return in, nil
}

// discard drops the instance and every tree in it.
func (c *core) discard() {
	if c.inst != nil {
		ops.release(c.inst.mem)
		c.inst = nil
	}
	c.gen++
	c.live, c.recycle, c.ptrs = 0, false, nil
	c.mu.Lock()
	c.leaked = nil
	c.leakedQueries = nil
	c.mu.Unlock()
}

// fault discards and recreates the instance and builds the error. Callers return it as the operation's result.
func (c *core) fault(o Outcome, reason string, cause error) *ParseError {
	c.discard()
	e := perr(o, reason, cause)
	e.InstanceReset = true
	if err := c.recreate(); err != nil {
		e.Reason += "; recreating the instance failed: " + err.Error()
	}
	return e
}

// engineFault is fault for a state violation: a refused memory.grow explains it as ResourceLimit.
func (c *core) engineFault(reason string) *ParseError {
	if c.inst != nil && ops.refused(c.inst.mem) {
		return c.fault(ResourceLimit, reason+": memory.grow refused", nil)
	}
	return c.fault(EngineFault, reason, nil)
}

// trapped classifies a trap: a refused memory.grow is ResourceLimit, anything else EngineFault; both reset.
func (c *core) trapped(err error, what string) *ParseError {
	if c.inst != nil && ops.refused(c.inst.mem) {
		return c.fault(ResourceLimit, what+": memory.grow refused", err)
	}
	return c.fault(EngineFault, what+": "+err.Error(), err)
}

// begin starts an operation: nil/done context, closed parser, then the single-owner guard.
func (c *core) begin(ctx context.Context) error {
	if ctx == nil {
		return perr(InvalidInput, "nil context", nil)
	}
	if ctx.Err() != nil {
		return ctxError(ctx, "context done before the call")
	}
	if !c.busy.CompareAndSwap(false, true) {
		return perr(InvalidInput, "parser is in use by another goroutine", nil)
	}
	if c.closed {
		c.busy.Store(false)
		return perr(InvalidInput, "parser is closed", ErrParserClosed)
	}
	if c.inst == nil {
		if err := c.recreate(); err != nil {
			c.busy.Store(false)
			return err
		}
	}
	if err := c.drainQueries(); err != nil {
		c.busy.Store(false)
		return err
	}
	return nil
}

func (c *core) end() { c.busy.Store(false) }

func (c *core) enqueueLeak(l leakedTree) {
	c.mu.Lock()
	c.leaked = append(c.leaked, l)
	c.mu.Unlock()
}

// drainLeaked frees trees whose owners were garbage collected. Owner goroutine only.
func (c *core) drainLeaked() *ParseError {
	c.mu.Lock()
	q := c.leaked
	c.leaked = nil
	c.mu.Unlock()
	for _, l := range q {
		if l.gen != c.gen || c.inst == nil {
			continue
		}
		c.live--
		delete(c.ptrs, l.ptr)
		if e := c.deleteTree(l.ptr); e != nil {
			return e
		}
	}
	return nil
}

func (c *core) deleteTree(ptr int32) *ParseError {
	in := c.inst
	if err := ops.call(func() { in.m.Xtsg_tree_delete(ptr) }); err != nil {
		return c.trapped(err, "tsg_tree_delete")
	}
	return nil
}

func (c *core) resetParser() *ParseError {
	in := c.inst
	if err := ops.call(func() { in.m.Xtsg_parser_reset() }); err != nil {
		return c.trapped(err, "tsg_parser_reset")
	}
	return nil
}

// abandonTree is the end of a no-tree path after a tree was produced: free it and reset the C parser.
func (c *core) abandonTree(ptr int32, gen uint64) {
	if gen != c.gen || c.inst == nil {
		return // the instance (and the tree) is already gone
	}
	if c.deleteTree(ptr) == nil {
		c.resetParser()
	}
}

func (c *core) noteCommitted() {
	if c.cfg.recycleAbove > 0 && c.inst != nil && ops.committed(c.inst.mem) > c.cfg.recycleAbove {
		c.recycle = true
	}
}

// maybeRecycle replaces the instance once it is over the threshold and no live tree would be invalidated.
func (c *core) maybeRecycle() {
	if c.recycle && c.live == 0 && c.inst != nil && !c.closed {
		c.discard()
		c.recreate() // on failure inst stays nil and the next operation retries
	}
}

// watchCtx sets Host.Stop when ctx is done; the returned func ends the watch and clears the flag.
func watchCtx(ctx context.Context, h *engine.Host) func() {
	h.Stop.Store(0)
	if ctx.Done() == nil {
		return func() {}
	}
	fired := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { h.Stop.Store(1); close(fired) })
	return func() {
		if !stop() {
			<-fired
		}
		h.Stop.Store(0)
	}
}

// run admits the input, drives one engine parse and returns the handle of a complete tree whose root ends at the input
// length. Every path without a tree ends with the C parser reset (or the instance replaced).
func (c *core) run(ctx context.Context, src []byte, old *Tree) (ptr int32, invalid bool, e *ParseError) {
	n := len(src)
	limit := c.cfg.maxInput
	if limit == 0 || limit > hardMaxInput {
		limit = hardMaxInput
	}
	if int64(n) > limit {
		return 0, false, perr(ResourceLimit, fmt.Sprintf("input is %d bytes, limit %d", n, limit), nil)
	}
	invalid = !utf8.Valid(src)
	if invalid && c.cfg.strictUTF8 {
		return 0, false, perr(InvalidInput, "input is not valid UTF-8 (WithStrictUTF8)", nil)
	}
	var oldPtr int32
	if old != nil {
		if old.p == nil || old.p.c != c {
			return 0, false, perr(InvalidInput, "old tree belongs to another parser", nil)
		}
		if !old.valid() {
			return 0, false, perr(InvalidInput, "old tree is closed or invalidated", ErrTreeInvalidated)
		}
		oldPtr = old.ptr
	}
	if e := c.drainLeaked(); e != nil {
		return 0, false, e
	}
	c.maybeRecycle()
	if c.inst == nil {
		if err := c.recreate(); err != nil {
			return 0, false, err.(*ParseError)
		}
	}
	in := c.inst

	var buf int32
	if err := ops.call(func() { buf = in.m.Xmalloc(int32(max(n, 1))) }); err != nil {
		return 0, false, c.trapped(err, "malloc")
	}
	mem := *in.mem
	if buf <= 0 || int(buf)+n > len(mem) {
		return 0, false, c.trapped(errors.New("malloc returned NULL or an out-of-range block"), "malloc")
	}
	copy(mem[buf:], src)

	ops.clearRefused(in.mem)
	stop := watchCtx(ctx, in.h)
	var tree int32
	err := ops.call(func() { tree = in.m.Xtsg_parse(buf, int32(n), oldPtr) })
	stop()
	if err != nil {
		return 0, false, c.trapped(err, "tsg_parse")
	}
	if ops.refused(in.mem) { // even if a tree came back: the C runtime may have written through a NULL
		return 0, false, c.fault(ResourceLimit, "memory.grow was refused during the parse", nil)
	}
	if err := ops.call(func() { in.m.Xfree(buf) }); err != nil {
		return 0, false, c.trapped(err, "free")
	}
	if tree == 0 {
		if e := c.resetParser(); e != nil {
			return 0, false, e
		}
		if ctx.Err() != nil {
			return 0, false, ctxError(ctx, "parse stopped by the context")
		}
		return 0, false, c.fault(EngineFault, "parse returned no tree although the context is live", nil)
	}
	var end int32
	if err := ops.call(func() { end = in.m.Xtsg_root_end(tree) }); err != nil {
		return 0, false, c.trapped(err, "tsg_root_end")
	}
	if int(end) != n {
		c.abandonTree(tree, c.gen)
		return 0, false, c.fault(EngineFault, fmt.Sprintf("root ends at byte %d, input has %d bytes", end, n), nil)
	}
	return tree, invalid, nil
}

// Parse parses src (copied once into the tree's snapshot source) and returns a complete tree, or a *ParseError.
// old, when non-nil, must be an Edit-ed tree of this parser (incremental parse); incremental results are not
// guaranteed to equal a fresh parse. Syntax errors are not Go errors: see Tree.Outcome.
func (p *Parser) Parse(ctx context.Context, src []byte, old *Tree) (*Tree, error) {
	return p.parse(ctx, src, old, false)
}

// ParseOwned is Parse without the source copy: the caller transfers ownership of src and must never modify it again.
func (p *Parser) ParseOwned(ctx context.Context, src []byte, old *Tree) (*Tree, error) {
	return p.parse(ctx, src, old, true)
}

func ownedString(src []byte, owned bool) string {
	if owned {
		return unsafe.String(unsafe.SliceData(src), len(src))
	}
	return string(src)
}

func (p *Parser) parse(ctx context.Context, src []byte, old *Tree, owned bool) (*Tree, error) {
	c := p.c
	defer runtime.KeepAlive(p) // the cleanup registered in NewParser must not release the instance mid-call
	defer runtime.KeepAlive(old)
	if err := c.begin(ctx); err != nil {
		return nil, err
	}
	defer c.end()
	ptr, invalid, e := c.run(ctx, src, old)
	if e != nil {
		return nil, e
	}
	gen := c.gen
	text := ownedString(src, owned)
	outcome, e := c.readOutcome(ptr)
	if e != nil {
		return nil, e
	}
	t := &Tree{p: p, gen: gen, ptr: ptr, src: text, inputLen: len(src), outcome: outcome, invalidUTF8: invalid}
	t.cleanup = runtime.AddCleanup(t, func(a leakArg) { a.c.enqueueLeak(leakedTree{a.ptr, a.gen}) }, leakArg{c, ptr, gen})
	c.live++
	if c.ptrs == nil {
		c.ptrs = map[int32]struct{}{}
	}
	c.ptrs[ptr] = struct{}{}
	c.noteCommitted()
	return t, nil
}

func (c *core) readOutcome(tree int32) (Outcome, *ParseError) {
	in := c.inst
	var v int32
	if err := ops.call(func() { v = in.m.Xtsg_root_has_error(tree) }); err != nil {
		return 0, c.trapped(err, "tsg_root_has_error")
	}
	return outcomeOf(v != 0), nil
}

func outcomeOf(hasError bool) Outcome {
	if hasError {
		return AcceptedWithErrors
	}
	return AcceptedClean
}

// ParseSnapshot parses src, exports the tree into a Snapshot (source copied once) and frees the tree. The instance
// may be recycled right away (WithRecycleAboveBytes).
func (p *Parser) ParseSnapshot(ctx context.Context, src []byte) (*Snapshot, error) {
	c := p.c
	defer runtime.KeepAlive(p)
	if err := c.begin(ctx); err != nil {
		return nil, err
	}
	defer c.end()
	ptr, invalid, e := c.run(ctx, src, nil)
	if e != nil {
		return nil, e
	}
	gen := c.gen
	snap, e := c.export(ctx, ptr, string(src), invalid)
	if e != nil {
		c.abandonTree(ptr, gen)
		return nil, e
	}
	c.deleteTree(ptr) // a trap here already replaced the instance; the snapshot is independent of it
	c.noteCommitted()
	c.maybeRecycle()
	return snap, nil
}

// Reset drops every tree, recreates the instance and returns its memory to the OS.
func (p *Parser) Reset() error {
	c := p.c
	defer runtime.KeepAlive(p)
	if !c.busy.CompareAndSwap(false, true) {
		return perr(InvalidInput, "parser is in use by another goroutine", nil)
	}
	defer c.end()
	if c.closed {
		return perr(InvalidInput, "parser is closed", ErrParserClosed)
	}
	c.discard()
	return c.recreate()
}

// Close releases the instance and invalidates the parser's trees (snapshots stay valid). It is idempotent.
func (p *Parser) Close() error {
	c := p.c
	defer runtime.KeepAlive(p)
	if !c.busy.CompareAndSwap(false, true) {
		return perr(InvalidInput, "Close while the parser is in use by another goroutine", nil)
	}
	defer c.end()
	if !c.closed {
		c.closed = true
		c.discard()
	}
	return nil
}

// dropTrees invalidates every live Tree of the parser and frees them in the engine, keeping the instance (and its warm
// memory). Used by ParserPool.Put. Owner goroutine only; the caller holds the busy guard.
func (c *core) dropTrees() *ParseError {
	c.gen++ // every Tree handle now fails with ErrTreeInvalidated
	ptrs := c.ptrs
	c.ptrs, c.live = nil, 0
	c.mu.Lock()
	c.leaked = nil // entries carry the old generation: their trees are freed below
	c.mu.Unlock()
	if c.inst == nil {
		return nil
	}
	for ptr := range ptrs {
		if e := c.deleteTree(ptr); e != nil {
			return e // the instance was replaced; nothing is left to free
		}
	}
	if e := c.resetParser(); e != nil {
		return e
	}
	c.maybeRecycle()
	return nil
}

// parseExportHeader validates the 16-byte header of the export chunk buffer and returns the chunk capacity.
func parseExportHeader(mem []byte, buf int32) (int, *ParseError) {
	if buf <= 0 || int(buf)+16 > len(mem) {
		return 0, perr(EngineFault, "export buffer pointer is out of range", nil)
	}
	h := mem[buf : buf+16]
	if string(h[:4]) != engine.ExportMagic || binary.LittleEndian.Uint16(h[4:]) != engine.ExportFormat {
		return 0, perr(IncompatibleLanguage, fmt.Sprintf("snapshot block magic %q format %d, expected %q format %d",
			h[:4], binary.LittleEndian.Uint16(h[4:]), engine.ExportMagic, engine.ExportFormat), nil)
	}
	kc := int(binary.LittleEndian.Uint32(h[8:]))
	if kc <= 0 || kc%4 != 0 || int(buf)+16+33*kc > len(mem) {
		return 0, perr(EngineFault, fmt.Sprintf("snapshot chunk capacity %d is invalid", kc), nil)
	}
	return kc, nil
}

// export runs the chunked export of tree t into a Snapshot. Cancellation abandons the export and keeps the tree.
func (c *core) export(ctx context.Context, t int32, text string, invalid bool) (*Snapshot, *ParseError) {
	in := c.inst
	var cnt int32
	if err := ops.call(func() { cnt = in.m.Xtsg_descendant_count(t) }); err != nil {
		return nil, c.trapped(err, "tsg_descendant_count")
	}
	if lim := c.cfg.maxSnapshotNodes; lim > 0 && int64(cnt) > lim {
		return nil, perr(ResourceLimit, fmt.Sprintf("tree has %d nodes, WithMaxSnapshotNodes is %d", cnt, lim), nil)
	}
	if cnt <= 0 || int64(cnt)*8 > int64(len(*in.mem)) { // a node needs at least 8 bytes of tree memory
		return nil, c.engineFault(fmt.Sprintf("descendant count %d is not plausible", cnt))
	}
	endExport := func() *ParseError {
		if err := ops.call(in.m.Xtsg_export_end); err != nil {
			return c.trapped(err, "tsg_export_end")
		}
		return nil
	}
	var n, buf int32
	if err := ops.call(func() {
		if n = in.m.Xtsg_export_begin(t); n != 0 {
			buf = in.m.Xtsg_export_buf()
		}
	}); err != nil {
		return nil, c.trapped(err, "tsg_export_begin")
	}
	if n != cnt {
		return nil, c.engineFault(fmt.Sprintf("export_begin returned %d nodes, descendant count is %d", n, cnt))
	}
	kc, e := parseExportHeader(*in.mem, buf)
	if e != nil {
		if e.Outcome == IncompatibleLanguage { // the instance is healthy but cannot be read by this root module
			if e2 := endExport(); e2 != nil {
				return nil, e2
			}
			return nil, e
		}
		return nil, c.fault(e.Outcome, e.Reason, nil)
	}
	b := newSnapshotBuilder(int(n), c.lang, text, invalid)
	for { // until the engine reports 0: this also catches more nodes than the descendant count, even when n is a multiple of kc
		if ctx.Err() != nil {
			if e := endExport(); e != nil {
				return nil, e
			}
			return nil, ctxError(ctx, "snapshot export stopped by the context")
		}
		var k int32
		if err := ops.call(func() { k = in.m.Xtsg_export_next() }); err != nil {
			return nil, c.trapped(err, "tsg_export_next")
		}
		if k == 0 {
			break
		}
		if k < 0 || int(k) > kc || b.filled+int(k) > int(n) {
			return nil, c.engineFault(fmt.Sprintf("export_next returned %d nodes (capacity %d, %d of %d filled)", k, kc, b.filled, n))
		}
		mem := *in.mem
		if err := b.add(int(k), kc, mem[int(buf)+16:int(buf)+16+33*kc]); err != nil {
			return nil, c.engineFault("snapshot chunk: " + err.Error())
		}
	}
	var xerr int32
	if err := ops.call(func() { xerr = in.m.Xtsg_export_err() }); err != nil {
		return nil, c.trapped(err, "tsg_export_err")
	}
	if xerr != 0 {
		return nil, c.engineFault("the export traversal disagreed with the descendant count")
	}
	if e := endExport(); e != nil {
		return nil, e
	}
	snap, err := b.finish()
	if err != nil {
		return nil, c.engineFault("snapshot structure: " + err.Error())
	}
	return snap, nil
}
