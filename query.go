package treesitter

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"github.com/wotjr1649/go-treesitter/internal/engine"
)

var (
	ErrQueryUnsupported = &sentinel{msg: "treesitter: queries require engine ABI 2 or later"}
	// ErrQueryMatchLimit ends a match stream that exceeded QueryOptions.MatchLimit. It is not a *ParseError, and
	// errors.Is also matches ErrResourceLimit, so one ErrResourceLimit check covers every limit.
	ErrQueryMatchLimit = &sentinel{msg: "treesitter: query match limit exceeded", also: ErrResourceLimit}
)

// QueryErrorType identifies a C query compilation error, or a Go text-predicate error.
type QueryErrorType uint32

const (
	QueryErrorSyntax QueryErrorType = iota + 1
	QueryErrorNodeType
	QueryErrorField
	QueryErrorCapture
	QueryErrorStructure
	QueryErrorLanguage
	QueryErrorPredicate
)

// QueryError.Offset is a byte offset in the query source. Predicate errors point to the pattern start.
type QueryError struct {
	Offset uint32
	Type   QueryErrorType
	reason string
}

func (e *QueryError) Error() string {
	return fmt.Sprintf("treesitter: query error %d at byte %d: %s", e.Type, e.Offset, e.reason)
}

// Query is immutable and may be shared by parsers of its Language. Text predicates accept a Rust regex subset
// with CLI 0.27.2 Unicode tables; Go general categories/scripts follow the Go toolchain (see docs/design/query-abi.md).
// Directives, property assertions and the any-eq?/any-not-eq?/
// any-match?/any-not-match? operators are validated and exposed as properties without filtering, as in CLI v0.27.2.
// Reuse Query values to avoid repeated validation. An unreachable Query queues its compiled copies for deletion
// by each parser's next engine operation, such as Parse or Matches (Reset/Close release the instance).
type Query struct {
	lang     *Language
	source   string
	names    []string
	strings  []string
	patterns []queryPattern
}
type queryPattern struct {
	start      uint32
	predicates []textPredicate
	properties map[string]string
}
type queryArg struct {
	capture bool
	id      uint32
}
type textPredicate struct {
	op     string
	args   []queryArg
	re     *queryRegex
	negate bool
}

func (q *Query) CaptureNames() []string { return append([]string(nil), q.names...) }
func (q *Query) PatternCount() int      { return len(q.patterns) }

// Properties returns a copy. An invalid pattern index returns nil. #set! uses its property name as the key;
// other directives/assertions use the operator name and their space-separated arguments.
func (q *Query) Properties(pattern int) map[string]string {
	if pattern < 0 || pattern >= len(q.patterns) {
		return nil
	}
	out := make(map[string]string, len(q.patterns[pattern].properties))
	for k, v := range q.patterns[pattern].properties {
		out[k] = v
	}
	return out
}

// NewQuery validates pattern using a scratch instance; each parser compiles it once per instance on first use.
func NewQuery(lang *Language, pattern string) (*Query, error) {
	return newQuery(context.Background(), lang, pattern, nil)
}

func newQuery(ctx context.Context, lang *Language, pattern string, cfg *queryCompileConfig) (result *Query, err error) {
	if ctx == nil {
		return nil, perr(InvalidInput, "nil context", nil)
	}
	if ctx.Err() != nil {
		return nil, ctxError(ctx, "context done before query compilation")
	}
	if lang == nil || lang.g == nil {
		return nil, perr(InvalidInput, "nil language", nil)
	}
	if lang.g.ABI == 1 {
		return nil, ErrQueryUnsupported
	}
	limit := int64(hardMaxInput)
	var memory int64
	var metadata, regex *queryMeter
	if cfg != nil {
		if cfg.source > 0 {
			limit = cfg.source
		}
		memory = cfg.memory
		m, r := newQueryMeter(ctx, cfg.metadata, "query metadata"), newQueryMeter(ctx, cfg.regex, "query regex")
		metadata, regex = &m, &r
	}
	if int64(len(pattern)) > limit {
		return nil, perr(ResourceLimit, "query source is too large", nil)
	}
	p, err := NewParser(lang, WithMaxMemoryBytes(memory))
	if err != nil {
		return nil, err
	}
	defer p.Close()
	c := p.c
	in := c.inst
	q := &Query{lang: lang, source: pattern}
	ptr, err := c.compileQuery(ctx, q)
	if err != nil {
		return nil, err
	}
	m := in.m.(engine.QueryModule)
	var meta int32
	defer func() {
		if c.inst == in {
			if e := c.queryCall("query metadata cleanup", func() {
				if meta > 0 {
					m.Xfree(meta)
				}
				m.Xtsg_query_delete(ptr)
			}); e != nil {
				result, err = nil, e
			}
		}
	}()
	if ctx.Err() != nil {
		return nil, ctxError(ctx, "query compilation stopped before metadata export")
	}
	if err = c.queryCall("tsg_query_meta", func() { meta = m.Xtsg_query_meta(ptr) }); err != nil {
		return nil, err
	}
	mem := *in.mem
	if meta <= 0 || uint64(meta)+24 > uint64(len(mem)) {
		return nil, c.engineFault("invalid query metadata pointer")
	}
	b := mem[meta:]
	size := binary.LittleEndian.Uint32(b[8:])
	if size < 24 || uint64(size) > uint64(len(b)) {
		return nil, c.engineFault("invalid query metadata size")
	}
	if err = q.readMeta(b[:int(size)], metadata, regex); err != nil {
		switch err.(type) {
		case *QueryError, *ParseError:
			return nil, err
		default:
			return nil, c.engineFault(err.Error())
		}
	}
	return q, nil
}

func (c *core) queryCall(what string, fn func()) error {
	if err := ops.call(fn); err != nil {
		return c.trapped(err, what)
	}
	if ops.refused(c.inst.mem) {
		return c.fault(ResourceLimit, what+": memory.grow refused", nil)
	}
	return nil
}

func (c *core) compileQuery(ctx context.Context, q *Query) (int32, error) {
	if ctx.Err() != nil {
		return 0, ctxError(ctx, "query compilation stopped before source upload")
	}
	in := c.inst
	m, ok := in.m.(engine.QueryModule)
	if !ok || c.lang.g.ABI < 2 {
		return 0, ErrQueryUnsupported
	}
	ops.clearRefused(in.mem)
	var src, ptr int32
	if err := c.queryCall("query malloc", func() { src = m.Xmalloc(int32(max(1, len(q.source)))) }); err != nil {
		return 0, err
	}
	if src <= 0 || uint64(uint32(src))+uint64(len(q.source)) > uint64(len(*in.mem)) {
		return 0, c.engineFault("invalid query source allocation")
	}
	if ctx.Err() == nil {
		copy((*in.mem)[src:], q.source)
	}
	if ctx.Err() != nil {
		if err := c.queryCall("query source free", func() { m.Xfree(src) }); err != nil {
			return 0, err
		}
		return 0, ctxError(ctx, "query compilation stopped after source upload")
	}
	if err := c.queryCall("tsg_query_new", func() { ptr = m.Xtsg_query_new(src, int32(len(q.source))) }); err != nil {
		return 0, err
	}
	var offset, typ int32
	if ptr == 0 {
		if err := c.queryCall("query compilation error", func() { offset = m.Xtsg_query_error_offset(); typ = m.Xtsg_query_error_type() }); err != nil {
			return 0, err
		}
	}
	if err := c.queryCall("query source free", func() { m.Xfree(src) }); err != nil {
		return 0, err
	}
	if ctx.Err() != nil {
		if ptr != 0 {
			if err := c.queryCall("cancelled query cleanup", func() { m.Xtsg_query_delete(ptr) }); err != nil {
				return 0, err
			}
		}
		return 0, ctxError(ctx, "query compilation stopped by the context")
	}
	if ptr == 0 {
		if offset < 0 || int64(offset) > int64(len(q.source)) || typ < 1 || typ > 6 {
			return 0, c.engineFault("invalid query compilation error")
		}
		return 0, &QueryError{Offset: uint32(offset), Type: QueryErrorType(typ), reason: "C query compiler rejected the pattern"}
	}
	return ptr, nil
}

func (q *Query) readMeta(b []byte, metadata, regex *queryMeter) error {
	bad := func() error { return fmt.Errorf("invalid query metadata") }
	if len(b) < 24 || string(b[:4]) != "TSGQ" || binary.LittleEndian.Uint32(b[4:]) != 1 || int(binary.LittleEndian.Uint32(b[8:])) != len(b) {
		return bad()
	}
	if !metadata.charge(0) {
		return metadata.err
	}
	if metadata != nil && metadata.limited && int64(len(b)) > metadata.remaining {
		return perr(ResourceLimit, "query metadata block limit exceeded", nil)
	}
	patterns, captures, literals := binary.LittleEndian.Uint32(b[12:]), binary.LittleEndian.Uint32(b[16:]), binary.LittleEndian.Uint32(b[20:])
	if uint64(patterns)*8+uint64(captures)*4+uint64(literals)*4 > uint64(len(b)-24) {
		return bad()
	}
	np, nc, ns := int(patterns), int(captures), int(literals)
	pos := 24
	take := func() (uint32, bool) {
		if len(b)-pos < 4 {
			return 0, false
		}
		v := binary.LittleEndian.Uint32(b[pos:])
		pos += 4
		return v, true
	}
	readStrings := func(n int) ([]string, error) {
		out := make([]string, n)
		for i := range out {
			if !metadata.charge(0) {
				return nil, metadata.err
			}
			l, ok := take()
			if !ok || uint64(pos)+uint64(l) > uint64(len(b)) {
				return nil, bad()
			}
			out[i] = string(b[pos : pos+int(l)])
			pos += int(l)
		}
		return out, nil
	}
	var err error
	if q.names, err = readStrings(nc); err != nil {
		return err
	}
	if q.strings, err = readStrings(ns); err != nil {
		return err
	}
	q.patterns = make([]queryPattern, np)
	for i := range q.patterns {
		if !metadata.charge(0) {
			return metadata.err
		}
		start, ok := take()
		if !ok || uint64(start) > uint64(len(q.source)) {
			return bad()
		}
		count, ok := take()
		if !ok || uint64(count)*8 > uint64(len(b)-pos) {
			return bad()
		}
		p := &q.patterns[i]
		p.start = start
		p.properties = map[string]string{}
		var steps []queryArg
		for j := uint32(0); j < count; j++ {
			if !metadata.charge(0) {
				return metadata.err
			}
			typ, _ := take()
			id, _ := take()
			switch typ {
			case 0:
				if len(steps) == 0 || steps[0].capture {
					return bad()
				}
				if err := q.addPredicate(p, steps, metadata, regex); err != nil {
					return err
				}
				steps = nil
			case 1, 2:
				if (typ == 1 && uint64(id) >= uint64(nc)) || (typ == 2 && uint64(id) >= uint64(ns)) {
					return bad()
				}
				steps = append(steps, queryArg{capture: typ == 1, id: id})
			default:
				return bad()
			}
		}
		if len(steps) != 0 {
			return bad()
		}
	}
	if pos != len(b) {
		return bad()
	}
	if !metadata.charge(0) {
		return metadata.err
	}
	return nil
}

func (q *Query) addPredicate(p *queryPattern, steps []queryArg, metadata, regex *queryMeter) error {
	op, args := q.strings[steps[0].id], steps[1:]
	fail := func(why string) error {
		return &QueryError{Offset: p.start, Type: QueryErrorPredicate, reason: op + ": " + why}
	}
	opaqueAny := op == "any-eq?" || op == "any-not-eq?" || op == "any-match?" || op == "any-not-match?"
	base := op
	if opaqueAny {
		base = strings.TrimPrefix(base, "any-")
	}
	pred := textPredicate{op: strings.TrimPrefix(base, "not-"), args: args, negate: strings.HasPrefix(base, "not-")}
	filter := true
	switch pred.op {
	case "eq?":
		if len(args) != 2 || !args[0].capture {
			return fail("expected a capture and a capture or string")
		}
	case "match?":
		if len(args) != 2 || !args[0].capture || args[1].capture {
			return fail("expected a capture and a regex string")
		}
		var err error
		pred.re, err = compileQueryRegexWithMeter(q.strings[args[1].id], regex)
		if err != nil {
			if _, ok := err.(*ParseError); ok {
				return err
			}
			return fail(err.Error())
		}
	case "any-of?":
		if len(args) < 1 || !args[0].capture {
			return fail("expected a capture and optional strings")
		}
		for _, a := range args[1:] {
			if !metadata.charge(0) {
				return metadata.err
			}
			if a.capture {
				return fail("expected literal alternatives")
			}
		}
	default:
		filter = false
		if op == "set!" || op == "is?" || op == "is-not?" {
			if len(args) < 1 || len(args) > 3 {
				return fail("expected one to three property arguments")
			}
			var literals []string
			captures := 0
			for _, a := range args {
				if a.capture {
					captures++
				} else {
					literals = append(literals, q.strings[a.id])
				}
			}
			if captures > 1 || len(literals) < 1 || len(literals) > 2 {
				return fail("expected a property name, optional value and at most one capture")
			}
			if op == "set!" {
				value := ""
				if len(literals) == 2 {
					value = literals[1]
				}
				p.properties[literals[0]] = value
				return nil
			}
		}
	}
	if !filter || opaqueAny {
		size := int64(max(0, len(args)-1))
		if !metadata.charge(size) {
			return metadata.err
		}
		for _, a := range args {
			var n int64
			if a.capture {
				n = 1 + int64(len(q.names[a.id]))
			} else {
				n = int64(len(q.strings[a.id]))
			}
			if !metadata.charge(n) {
				return metadata.err
			}
			if n > int64(math.MaxInt)-size {
				return perr(ResourceLimit, "query property exceeds string length range", nil)
			}
			size += n
		}
		var value strings.Builder
		value.Grow(int(size))
		for i, a := range args {
			if !metadata.charge(0) {
				return metadata.err
			}
			if i > 0 {
				value.WriteByte(' ')
			}
			if a.capture {
				value.WriteByte('@')
				value.WriteString(q.names[a.id])
			} else {
				value.WriteString(q.strings[a.id])
			}
		}
		p.properties[op] = value.String()
		return nil
	}
	p.predicates = append(p.predicates, pred)
	return nil
}

func (q *Query) accepts(m QueryMatch, work *queryMeter) bool {
	if !work.charge(1) {
		return false
	}
	for _, p := range q.patterns[m.Pattern].predicates {
		if !work.charge(1) {
			return false
		}
		var left, right []string
		for _, c := range m.Captures {
			if !work.charge(1) {
				return false
			}
			if c.Index == p.args[0].id {
				left = append(left, c.Node.Text())
			}
		}
		if p.op == "eq?" && p.args[1].capture {
			for _, c := range m.Captures {
				if !work.charge(1) {
					return false
				}
				if c.Index == p.args[1].id {
					right = append(right, c.Node.Text())
				}
			}
			if !work.charge(1) || len(left) != len(right) {
				return false
			}
		}
		for i, text := range left {
			if !work.charge(1) {
				return false
			}
			matched := false
			switch p.op {
			case "eq?":
				if p.args[1].capture {
					matched = queryTextEqual(text, right[i], work)
				} else {
					matched = queryTextEqual(text, q.strings[p.args[1].id], work)
				}
			case "match?":
				if work == nil {
					matched = p.re.MatchString(text)
				} else {
					matched = regexVMMatchWithMeter(p.re.prog, text, p.re.asciiBoundaries, work)
				}
			case "any-of?":
				for _, a := range p.args[1:] {
					if queryTextEqual(text, q.strings[a.id], work) {
						matched = true
						break
					}
					if work != nil && work.err != nil {
						return false
					}
				}
			}
			if (work != nil && work.err != nil) || matched == p.negate {
				return false
			}
		}
	}
	return true
}

// queryTextEqual meters the length check and each attempted byte comparison.
func queryTextEqual(a, b string, work *queryMeter) bool {
	if work == nil {
		return a == b
	}
	if !work.charge(1) || len(a) != len(b) {
		return false
	}
	for len(a) != 0 {
		n := min(len(a), 64)
		if !work.charge(int64(n)) || a[:n] != b[:n] {
			return false
		}
		a, b = a[n:], b[n:]
	}
	return true
}
