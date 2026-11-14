package treesitter

import (
	"encoding/binary"
	"fmt"
	"regexp"
	"strings"

	"github.com/wotjr1649/go-treesitter/internal/engine"
)

var (
	ErrQueryUnsupported = &sentinel{msg: "treesitter: queries require engine ABI 2"}
	ErrQueryMatchLimit  = &sentinel{msg: "treesitter: query match limit exceeded"}
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

// Query is immutable and may be shared by parsers of its Language. Text predicates use Go regexp (RE2),
// which differs from tree-sitter CLI's Rust regex. Directives, property assertions and the any-eq?/any-not-eq?/
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
	re     *regexp.Regexp
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
	if lang == nil || lang.g == nil {
		return nil, perr(InvalidInput, "nil language", nil)
	}
	if lang.g.ABI == 1 {
		return nil, ErrQueryUnsupported
	}
	if int64(len(pattern)) > hardMaxInput {
		return nil, perr(ResourceLimit, "query source is too large", nil)
	}
	p, err := NewParser(lang)
	if err != nil {
		return nil, err
	}
	defer p.Close()
	c := p.c
	q := &Query{lang: lang, source: pattern}
	ptr, err := c.compileQuery(q)
	if err != nil {
		return nil, err
	}
	m := c.inst.m.(engine.QueryModule)
	var meta int32
	if err := c.queryCall("tsg_query_meta", func() { meta = m.Xtsg_query_meta(ptr) }); err != nil {
		return nil, err
	}
	mem := *c.inst.mem
	if meta <= 0 || uint64(uint32(meta))+24 > uint64(len(mem)) {
		return nil, c.engineFault("invalid query metadata pointer")
	}
	size := binary.LittleEndian.Uint32(mem[int(meta)+8:])
	if size < 24 || uint64(uint32(meta))+uint64(size) > uint64(len(mem)) {
		return nil, c.engineFault("invalid query metadata size")
	}
	if err := q.readMeta(mem[int(meta) : int(meta)+int(size)]); err != nil {
		if _, ok := err.(*QueryError); !ok {
			return nil, c.engineFault(err.Error())
		}
		return nil, err
	}
	if err := c.queryCall("query metadata cleanup", func() { m.Xfree(meta); m.Xtsg_query_delete(ptr) }); err != nil {
		return nil, err
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

func (c *core) compileQuery(q *Query) (int32, error) {
	in := c.inst
	m, ok := in.m.(engine.QueryModule)
	if !ok || c.lang.g.ABI != 2 {
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
	copy((*in.mem)[src:], q.source)
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
	if ptr == 0 {
		if offset < 0 || int64(offset) > int64(len(q.source)) || typ < 1 || typ > 6 {
			return 0, c.engineFault("invalid query compilation error")
		}
		return 0, &QueryError{Offset: uint32(offset), Type: QueryErrorType(typ), reason: "C query compiler rejected the pattern"}
	}
	return ptr, nil
}

func (q *Query) readMeta(b []byte) error {
	bad := func() error { return fmt.Errorf("invalid query metadata") }
	if len(b) < 24 || string(b[:4]) != "TSGQ" || binary.LittleEndian.Uint32(b[4:]) != 1 || int(binary.LittleEndian.Uint32(b[8:])) != len(b) {
		return bad()
	}
	np, nc, ns := int(binary.LittleEndian.Uint32(b[12:])), int(binary.LittleEndian.Uint32(b[16:])), int(binary.LittleEndian.Uint32(b[20:]))
	if uint64(np)*8+uint64(nc)*4+uint64(ns)*4 > uint64(len(b)-24) {
		return bad()
	}
	pos := 24
	take := func() (uint32, bool) {
		if pos+4 > len(b) {
			return 0, false
		}
		v := binary.LittleEndian.Uint32(b[pos:])
		pos += 4
		return v, true
	}
	readStrings := func(n int) ([]string, error) {
		out := make([]string, n)
		for i := range out {
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
			typ, _ := take()
			id, _ := take()
			switch typ {
			case 0:
				if len(steps) == 0 || steps[0].capture {
					return bad()
				}
				if err := q.addPredicate(p, steps); err != nil {
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
	return nil
}

func (q *Query) addPredicate(p *queryPattern, steps []queryArg) error {
	op, args := q.strings[steps[0].id], steps[1:]
	fail := func(why string) error {
		return &QueryError{Offset: p.start, Type: QueryErrorPredicate, reason: op + ": " + why}
	}
	opaqueAny := op == "any-eq?" || op == "any-not-eq?" || op == "any-match?" || op == "any-not-match?"
	base := op
	if opaqueAny {
		base = strings.TrimPrefix(base, "any-")
	}
	pred := textPredicate{op: strings.TrimPrefix(base, "not-"), args: append([]queryArg(nil), args...), negate: strings.HasPrefix(base, "not-")}
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
		pred.re, err = regexp.Compile(q.strings[args[1].id])
		if err != nil {
			return fail(err.Error())
		}
	case "any-of?":
		if len(args) < 1 || !args[0].capture {
			return fail("expected a capture and optional strings")
		}
		for _, a := range args[1:] {
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
		var values []string
		for _, a := range args {
			v := ""
			if a.capture {
				v = "@" + q.names[a.id]
			} else {
				v = q.strings[a.id]
			}
			values = append(values, v)
		}
		p.properties[op] = strings.Join(values, " ")
		return nil
	}
	p.predicates = append(p.predicates, pred)
	return nil
}

func (q *Query) accepts(m QueryMatch) bool {
	for _, p := range q.patterns[m.Pattern].predicates {
		var left, right []string
		for _, c := range m.Captures {
			if c.Index == p.args[0].id {
				left = append(left, c.Node.Text())
			}
		}
		if p.op == "eq?" && p.args[1].capture {
			for _, c := range m.Captures {
				if c.Index == p.args[1].id {
					right = append(right, c.Node.Text())
				}
			}
			if len(left) != len(right) {
				return false
			}
		}
		for i, text := range left {
			matched := false
			switch p.op {
			case "eq?":
				if p.args[1].capture {
					matched = text == right[i]
				} else {
					matched = text == q.strings[p.args[1].id]
				}
			case "match?":
				matched = p.re.MatchString(text)
			case "any-of?":
				for _, a := range p.args[1:] {
					if text == q.strings[a.id] {
						matched = true
						break
					}
				}
			}
			if matched == p.negate {
				return false
			}
		}
	}
	return true
}
