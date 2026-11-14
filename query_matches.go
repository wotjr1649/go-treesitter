package treesitter

import (
	"context"
	"encoding/binary"
	"fmt"
	"iter"
	"runtime"
	"weak"

	"github.com/wotjr1649/go-treesitter/internal/engine"
)

// QueryOptions restricts matches to nodes intersecting [StartByte, EndByte).
// EndByte zero is unlimited, including zero-width nodes at EOF. An explicit EndByte equal to the
// source length excludes those EOF nodes. MatchLimit zero uses C's default (unlimited); a nonzero
// limit bounds in-progress match states, not the number of results.
type QueryOptions struct {
	StartByte, EndByte uint32
	MatchLimit         uint32
}
type QueryCapture struct {
	Index uint32
	Node  Node
}
type QueryMatch struct {
	Pattern  int
	Captures []QueryCapture
}

// Matches streams matches against an unedited tree, mapping captures to its immutable snapshot.
// The parser remains busy until iteration ends (including an early break). Errors terminate the stream.
// On match-limit overflow it ends with ErrQueryMatchLimit, discarding any match produced by that
// same engine call. Earlier matches remain valid; a small limit may produce no matches before the error.
// Cost: the first match of each call indexes the whole tree (O(nodes)), whatever StartByte/EndByte select.
func (t *Tree) Matches(ctx context.Context, q *Query, opts *QueryOptions) iter.Seq2[QueryMatch, error] {
	var options QueryOptions
	if opts != nil {
		options = *opts
	}
	return func(yield func(QueryMatch, error) bool) {
		stopped := false
		err := t.matches(ctx, q, options, func(m QueryMatch) bool {
			if !yield(m, nil) {
				stopped = true
				return false
			}
			return true
		})
		if err != nil && !stopped {
			yield(QueryMatch{}, err)
		}
	}
}

func (t *Tree) matches(ctx context.Context, q *Query, opts QueryOptions, yield func(QueryMatch) bool) (err error) {
	defer runtime.KeepAlive(t)
	defer runtime.KeepAlive(q)
	if t == nil || t.p == nil {
		return ErrTreeInvalidated
	}
	c := t.p.c
	if err = c.begin(ctx); err != nil {
		return err
	}
	defer c.end()
	if !t.valid() {
		return ErrTreeInvalidated
	}
	if t.edited {
		return ErrTreeEdited
	}
	if q == nil || q.lang != c.lang {
		return perr(InvalidInput, "query is nil or belongs to another language", nil)
	}
	if c.lang.g.ABI != 2 {
		return ErrQueryUnsupported
	}
	end := opts.EndByte
	if end == 0 {
		end = uint32(t.inputLen)
	}
	if opts.StartByte > end || uint64(end) > uint64(t.inputLen) {
		return perr(InvalidInput, "query byte range is outside the source", nil)
	}
	if t.snap == nil {
		snap, e := c.export(ctx, t.ptr, t.src, t.invalidUTF8)
		if e != nil {
			return e
		}
		t.snap = snap
	}
	in := c.inst
	gen := c.gen
	m := in.m.(engine.QueryModule)
	ops.clearRefused(in.mem)
	stop := watchCtx(ctx, in.h)
	defer stop()
	key := weak.Make(q)
	ptr := in.queries[key]
	if ptr == 0 {
		ptr, err = c.compileQuery(q)
		if err != nil {
			return err
		}
		if in.queries == nil {
			in.queries = map[weak.Pointer[Query]]int32{}
		}
		in.queries[key] = ptr
		runtime.AddCleanup(q, retireQuery, queryCleanup{
			owner: weak.Make(c), queryRetirement: queryRetirement{key: key, instance: weak.Make(in)},
		})
	}
	if ctx.Err() != nil {
		return ctxError(ctx, "query compilation stopped by the context")
	}
	var cursor int32
	if err = c.queryCall("tsg_qcursor_new", func() { cursor = m.Xtsg_qcursor_new() }); err != nil {
		return err
	}
	if cursor <= 0 {
		return c.engineFault("query cursor allocation returned NULL")
	}
	defer func() {
		if c.gen != gen || c.inst != in {
			return
		}
		cleanupErr := c.queryCall("tsg_qcursor_delete", func() { m.Xtsg_qcursor_delete(cursor) })
		if cleanupErr != nil {
			err = cleanupErr
		}
		c.noteCommitted()
	}()
	var rc int32
	if err = c.queryCall("tsg_qcursor_exec", func() {
		rc = m.Xtsg_qcursor_exec(cursor, ptr, t.ptr, int32(opts.StartByte), int32(opts.EndByte), int32(opts.MatchLimit))
	}); err != nil {
		return err
	}
	if rc != 0 {
		return c.engineFault("query cursor rejected a validated byte range")
	}
	for {
		if ctx.Err() != nil {
			return ctxError(ctx, "query stopped by the context")
		}
		if !t.valid() {
			return ErrTreeInvalidated
		}
		var count, halted, exceeded, buf int32
		if err = c.queryCall("tsg_qcursor_next", func() {
			count = m.Xtsg_qcursor_next(cursor)
			halted = m.Xtsg_qcursor_halted(cursor)
			exceeded = m.Xtsg_qcursor_exceeded(cursor)
			if count != 0 {
				buf = m.Xtsg_qcursor_buf(cursor)
			}
		}); err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctxError(ctx, "query stopped by the context")
		}
		if halted != 0 {
			return c.engineFault("query halted with a live context")
		}
		if exceeded != 0 {
			return ErrQueryMatchLimit
		}
		if count == 0 {
			return nil
		}
		matches, e := decodeQueryMatches(*in.mem, buf, count, q, t.snap)
		if e != nil {
			return c.engineFault(e.Error())
		}
		for _, match := range matches {
			if ctx.Err() != nil {
				return ctxError(ctx, "query stopped during predicate evaluation")
			}
			accepted := q.accepts(match)
			if ctx.Err() != nil {
				return ctxError(ctx, "query stopped during predicate evaluation")
			}
			if accepted && !yield(match) {
				return nil
			}
		}
	}
}

func decodeQueryMatches(mem []byte, ptr, count int32, q *Query, s *Snapshot) ([]QueryMatch, error) {
	bad := func() ([]QueryMatch, error) { return nil, fmt.Errorf("invalid query match buffer or capture mapping") }
	if ptr <= 0 || uint64(uint32(ptr))+16 > uint64(len(mem)) {
		return bad()
	}
	b := mem[ptr:]
	le := binary.LittleEndian
	capacity, size := le.Uint32(b[8:]), le.Uint32(b[12:])
	if string(b[:4]) != "TSGM" || le.Uint32(b[4:]) != 1 || count <= 0 || uint32(count) > capacity || uint64(size)+16 > uint64(len(b)) || uint64(count)*8 > uint64(size) {
		return bad()
	}
	b = b[16 : 16+int(size)]
	var out []QueryMatch
	for j := int32(0); j < count; j++ {
		if len(b) < 8 {
			return bad()
		}
		pattern, nc := le.Uint32(b), le.Uint32(b[4:])
		b = b[8:]
		if uint64(pattern) >= uint64(len(q.patterns)) || uint64(nc)*20 > uint64(len(b)) {
			return bad()
		}
		m := QueryMatch{Pattern: int(pattern), Captures: make([]QueryCapture, int(nc))}
		for i := range m.Captures {
			capture, index, sym, sb, eb := le.Uint32(b), le.Uint32(b[4:]), le.Uint32(b[8:]), le.Uint32(b[12:]), le.Uint32(b[16:])
			b = b[20:]
			if uint64(capture) >= uint64(len(q.names)) || uint64(index) >= uint64(s.n) || sym != uint32(s.symbol[index]) || sb != s.startByte[index] || eb != s.endByte[index] {
				return bad()
			}
			m.Captures[i] = QueryCapture{Index: capture, Node: s.Node(int(index))}
		}
		out = append(out, m)
	}
	if len(b) != 0 {
		return bad()
	}
	return out, nil
}
