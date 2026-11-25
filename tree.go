package treesitter

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"runtime"
	"sync/atomic"
)

// Tree is a complete parse tree living in its Parser's engine instance. It belongs to the Parser's confinement unit.
// After Parser.Close, Parser.Reset or an instance reset every method that touches the instance returns
// ErrTreeInvalidated. Close a Tree when done; a leaked Tree is freed by its Parser's owner goroutine later.
type Tree struct {
	p           *Parser
	gen         uint64
	ptr         int32
	src         string
	inputLen    atomic.Int64 // current input length; Edit keeps it in step
	outcome     Outcome
	invalidUTF8 bool
	edited      bool
	closed      atomic.Bool
	snap        *Snapshot
	cleanup     runtime.Cleanup
}

// valid reads the parser's generation under the busy guard.
func (t *Tree) valid() bool { return !t.closed.Load() && t.gen == t.p.c.gen && !t.p.c.closed }

// Outcome is AcceptedClean or AcceptedWithErrors.
func (t *Tree) Outcome() Outcome { return t.outcome }

// HasInvalidUTF8 reports whether the source had invalid UTF-8 (parsed like C: decode-error bytes).
func (t *Tree) HasInvalidUTF8() bool { return t.invalidUTF8 }

// InputBytes is the length of the source the tree describes (updated by Edit).
func (t *Tree) InputBytes() int { return int(t.inputLen.Load()) }

// Snapshot exports the tree into an immutable Snapshot. The result is cached; the export is chunked and checks ctx
// between chunks (Timeout/Cancelled leave the tree intact). An edited tree returns ErrTreeEdited: take snapshots
// before editing.
func (t *Tree) Snapshot(ctx context.Context) (*Snapshot, error) {
	c := t.p.c
	defer runtime.KeepAlive(t) // keeps the Parser (and its cleanup) alive for the whole call
	if ctx == nil {
		return nil, perr(InvalidInput, "nil context", nil)
	}
	if !c.busy.CompareAndSwap(false, true) {
		return nil, perr(InvalidInput, "parser is in use by another goroutine", nil)
	}
	defer c.end()
	if !t.valid() {
		return nil, ErrTreeInvalidated
	}
	if t.edited {
		return nil, ErrTreeEdited
	}
	if t.snap != nil {
		return t.snap, nil
	}
	if ctx.Err() != nil {
		return nil, ctxError(ctx, "context done before the call")
	}
	if err := c.drainQueries(); err != nil {
		return nil, err
	}
	snap, e := c.export(ctx, t.ptr, func() string { return t.src }, t.invalidUTF8)
	if e != nil {
		return nil, e
	}
	t.snap = snap
	return snap, nil
}

// InputEdit describes one edit of the source, as in C tree-sitter: columns are byte columns.
type InputEdit struct {
	StartByte, OldEndByte, NewEndByte    uint32
	StartPoint, OldEndPoint, NewEndPoint Point
}

// Edit applies e to the tree in place (C semantics). Afterwards the tree is old-only: pass it as the old tree of the
// next Parse; Snapshot returns ErrTreeEdited. An edit that does not fit the tree is rejected with ErrInvalidInput.
// Only byte bounds and point ordering are checked, not their correspondence; use EditFor for consistent points.
func (t *Tree) Edit(e InputEdit) error {
	c := t.p.c
	defer runtime.KeepAlive(t)
	if !c.busy.CompareAndSwap(false, true) {
		return perr(InvalidInput, "parser is in use by another goroutine", nil)
	}
	defer c.end()
	if !t.valid() {
		return ErrTreeInvalidated
	}
	inputLen := t.InputBytes()
	switch {
	case e.StartByte > e.OldEndByte || e.StartByte > e.NewEndByte:
		return fmt.Errorf("%w: edit start %d is after an end (%d, %d)", ErrInvalidInput, e.StartByte, e.OldEndByte, e.NewEndByte)
	case int64(e.OldEndByte) > int64(inputLen):
		return fmt.Errorf("%w: edit old end %d is beyond the %d-byte input", ErrInvalidInput, e.OldEndByte, inputLen)
	case e.StartPoint.key() > e.OldEndPoint.key() || e.StartPoint.key() > e.NewEndPoint.key():
		return fmt.Errorf("%w: edit start point is after an end point", ErrInvalidInput)
	}
	in := c.inst
	if err := ops.call(func() {
		in.m.Xtsg_tree_edit(t.ptr, int32(e.StartByte), int32(e.OldEndByte), int32(e.NewEndByte),
			int32(e.StartPoint.Row), int32(e.StartPoint.Column), int32(e.OldEndPoint.Row), int32(e.OldEndPoint.Column),
			int32(e.NewEndPoint.Row), int32(e.NewEndPoint.Column))
	}); err != nil {
		return c.trapped(err, "tsg_tree_edit")
	}
	t.edited, t.snap = true, nil
	t.inputLen.Store(int64(inputLen + int(e.NewEndByte) - int(e.OldEndByte)))
	return nil
}

// ChangedRanges returns the ranges whose syntactic structure differs between old (the edited tree that was passed to
// Parse) and t (its result), with one call into the instance.
func (t *Tree) ChangedRanges(old *Tree) ([]Range, error) {
	c := t.p.c
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(old)
	if old == nil || old.p == nil || old.p.c != c {
		return nil, perr(InvalidInput, "old tree is nil or belongs to another parser", nil)
	}
	if !c.busy.CompareAndSwap(false, true) {
		return nil, perr(InvalidInput, "parser is in use by another goroutine", nil)
	}
	defer c.end()
	if !t.valid() || !old.valid() {
		return nil, ErrTreeInvalidated
	}
	if t.edited {
		return nil, ErrTreeEdited
	}
	in := c.inst
	var blk int32
	if err := ops.call(func() { blk = in.m.Xtsg_changed_ranges(old.ptr, t.ptr) }); err != nil {
		return nil, c.trapped(err, "tsg_changed_ranges")
	}
	mem := *in.mem
	if blk <= 0 || int(blk)+4 > len(mem) {
		return nil, c.engineFault("changed-ranges block pointer is out of range")
	}
	le := binary.LittleEndian
	count := int(le.Uint32(mem[blk:]))
	if count < 0 || int64(blk)+4+24*int64(count) > int64(len(mem)) {
		return nil, c.engineFault(fmt.Sprintf("changed-ranges count %d does not fit the block", count))
	}
	var lines *LineIndex
	if count > 0 {
		if t.snap != nil {
			lines = t.snap.Lines()
		} else {
			lines = NewLineIndex(t.src)
		}
	}
	out := make([]Range, count)
	for i := range out {
		r := mem[int(blk)+4+24*i:]
		out[i] = Range{
			StartByte: le.Uint32(r), EndByte: le.Uint32(r[4:]),
		}
		out[i].StartPoint = lines.point(out[i].StartByte)
		out[i].EndPoint = lines.point(out[i].EndByte)
	}
	if err := ops.call(func() { in.m.Xfree(blk) }); err != nil {
		return nil, c.trapped(err, "free")
	}
	return out, nil
}

// Close marks the tree closed and frees its handle. It is idempotent. If the parser is busy, Close queues the
// handle for the owner to free.
func (t *Tree) Close() {
	if !t.closed.CompareAndSwap(false, true) {
		return
	}
	defer runtime.KeepAlive(t)
	t.cleanup.Stop()
	c := t.p.c
	if !c.busy.CompareAndSwap(false, true) {
		c.enqueueLeak(leakedTree{t.ptr, t.gen})
		return
	}
	defer c.end()
	if t.gen != c.gen || c.closed || c.inst == nil {
		return // the instance (and the tree with it) is already gone
	}
	c.live--
	delete(c.ptrs, t.ptr)
	if c.deleteTree(t.ptr) == nil {
		c.maybeRecycle()
	}
}

// EditFor computes the InputEdit for replacing old[start:oldEnd] with new[start:newEnd]. Points use byte columns and
// count '\n' as row breaks; offsets beyond a text are clamped to its length. It is O(n).
func EditFor(old, new []byte, start, oldEnd, newEnd uint32) InputEdit {
	return InputEdit{
		StartByte: start, OldEndByte: oldEnd, NewEndByte: newEnd,
		StartPoint: pointAt(old, start), OldEndPoint: pointAt(old, oldEnd), NewEndPoint: pointAt(new, newEnd),
	}
}

func pointAt(b []byte, off uint32) Point {
	if int64(off) > int64(len(b)) {
		off = uint32(len(b))
	}
	b = b[:off]
	return Point{uint32(bytes.Count(b, []byte{'\n'})), uint32(len(b) - 1 - bytes.LastIndexByte(b, '\n'))}
}
