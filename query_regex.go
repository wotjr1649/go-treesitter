package treesitter

import (
	"fmt"
	"math"
	"regexp"
	"regexp/syntax"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type regexRange struct{ lo, hi rune }

type queryRegex struct {
	re              *regexp.Regexp
	prog            *syntax.Prog
	asciiBoundaries map[uint32]bool
	boundary        bool
}

func (re *queryRegex) MatchString(text string) bool {
	if re.re != nil && !re.boundary && utf8.ValidString(text) {
		return re.re.MatchString(text)
	}
	return regexVMMatch(re.prog, text, re.asciiBoundaries)
}

func compileQueryRegex(pattern string) (*queryRegex, error) {
	return compileQueryRegexWithMeter(pattern, nil)
}

func compileQueryRegexWithMeter(pattern string, meter *queryMeter) (*queryRegex, error) {
	p := regexTranslator{src: pattern, meter: meter}
	var out strings.Builder
	if err := p.expression(&out, regexFlags{unicode: true}, false); err != nil {
		return nil, err
	}
	canonical := out.String()
	// Canonical syntax has explicit ranges and no implicit Unicode or fold tables.
	// Allow for parser nodes, literal/range runes and alternation factoring.
	if meter != nil {
		units := int64(-1)
		if int64(len(canonical)) < math.MaxInt64/16 {
			units = 16 * (int64(len(canonical)) + 1)
		}
		if !meter.charge(units) {
			return nil, meter.err
		}
	}
	ast, err := syntax.Parse(canonical, syntax.Perl)
	if !meter.charge(0) {
		return nil, meter.err
	}
	if err != nil {
		return nil, fmt.Errorf("query regex: %w", err)
	}
	if meter != nil && meter.limited {
		cost := expandedRegexUnits(ast, meter.remaining, meter)
		if !meter.charge(cost) {
			return nil, meter.err
		}
	}
	asciiCaps := map[uint32]bool{}
	var visit func(*syntax.Regexp)
	visit = func(r *syntax.Regexp) {
		if !meter.charge(0) {
			return
		}
		if r.Op == syntax.OpCapture && strings.HasPrefix(r.Name, "tsgAsciiBoundary") {
			asciiCaps[uint32(2*r.Cap)] = true
		}
		for _, sub := range r.Sub {
			visit(sub)
			if meter != nil && meter.err != nil {
				return
			}
		}
	}
	visit(ast)
	if !meter.charge(0) {
		return nil, meter.err
	}
	simplified := ast.Simplify()
	if !meter.charge(0) {
		return nil, meter.err
	}
	prog, err := syntax.Compile(simplified)
	if !meter.charge(0) {
		return nil, meter.err
	}
	if err != nil {
		return nil, fmt.Errorf("query regex: %w", err)
	}
	asciiPC := map[uint32]bool{}
	for _, inst := range prog.Inst {
		if !meter.charge(0) {
			return nil, meter.err
		}
		if inst.Op == syntax.InstCapture && asciiCaps[inst.Arg] {
			asciiPC[inst.Out] = true
		}
	}
	var re *regexp.Regexp
	if meter == nil {
		re, err = regexp.Compile(canonical)
		if err != nil {
			return nil, fmt.Errorf("query regex: %w", err)
		}
	}
	return &queryRegex{re: re, prog: prog, boundary: p.boundary, asciiBoundaries: asciiPC}, nil
}

// expandedRegexUnits bounds the simplified tree and program as if shared subtrees
// and rune slices were copied at each use. A negative result denotes overflow.
func expandedRegexUnits(ast *syntax.Regexp, limit int64, meter *queryMeter) int64 {
	var size func(*syntax.Regexp) int64
	size = func(r *syntax.Regexp) int64 {
		if !meter.charge(0) {
			return -1
		}
		// One node, up to two instructions, and up to four implicit runes.
		// Concatenation and alternation allow control instructions per child.
		cost := 7 + 2*int64(len(r.Rune)) + 2*int64(len(r.Sub))
		for _, sub := range r.Sub {
			n := size(sub)
			if n < 0 || n > limit-cost {
				return -1
			}
			cost += n
		}
		if r.Op == syntax.OpRepeat {
			// Each optional repetition may add a concat, a question and an
			// instruction. The unbounded form uses a final plus or star.
			copies := max(1, int64(r.Min), int64(r.Max))
			if cost > limit/copies-4 {
				return -1
			}
			cost = copies * (cost + 4)
		}
		return cost
	}
	cost := size(ast)
	if cost < 0 || cost > limit-2 {
		return -1
	}
	return cost + 2 // leading fail and final match
}

type regexFlags struct{ fold, multi, dot, unicode bool }
type regexTranslator struct {
	meter               *queryMeter
	src                 string
	pos, asciiID, depth int
	boundary            bool
	names               map[string]bool
}

func (p *regexTranslator) fail(construct string) error {
	return fmt.Errorf("query regex at byte %d: unsupported or invalid %s", p.pos, construct)
}

func (p *regexTranslator) expression(out *strings.Builder, flags regexFlags, grouped bool) error {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > 100 {
		return p.fail("group nesting (limit 100)")
	}
	lastFlags := false
	for p.pos < len(p.src) {
		if !p.meter.charge(0) {
			return p.meter.err
		}
		c := p.src[p.pos]
		p.pos++
		if lastFlags && strings.ContainsRune("*+?{", rune(c)) {
			return p.fail("repetition immediately after flags")
		}
		lastFlags = false
		var token string
		switch c {
		case ')':
			if !grouped {
				return p.fail("unmatched )")
			}
			return nil
		case '(':
			next := flags
			if p.pos < len(p.src) && p.src[p.pos] == '?' {
				p.pos++
				if p.pos == len(p.src) {
					return p.fail("group")
				}
				if p.src[p.pos] == ':' {
					p.pos++
				} else if strings.HasPrefix(p.src[p.pos:], "P<") || p.src[p.pos] == '<' {
					if p.src[p.pos] == 'P' {
						p.pos++
					}
					p.pos++
					start := p.pos
					for p.pos < len(p.src) && p.src[p.pos] != '>' {
						if !p.meter.charge(1) {
							return p.meter.err
						}
						p.pos++
					}
					if p.pos == len(p.src) {
						return p.fail("named group")
					}
					name := p.src[start:p.pos]
					p.pos++
					if name == "" {
						return p.fail("empty group name")
					}
					if !p.meter.charge(1) {
						return p.meter.err
					}
					if p.names == nil {
						p.names = map[string]bool{}
					}
					if p.names[name] {
						return p.fail("duplicate group name " + name)
					}
					p.names[name] = true
					for i, r := range name {
						if !p.meter.charge(0) {
							return p.meter.err
						}
						if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
							return p.fail("named group " + name + " (use ASCII letters, digits and _)")
						}
					}
				} else {
					negative, seen := false, false
					var used [128]bool
					for p.pos < len(p.src) && p.src[p.pos] != ':' && p.src[p.pos] != ')' {
						f := p.src[p.pos]
						p.pos++
						if f == '-' && !negative {
							negative = true
							continue
						}
						if f >= 128 || used[f] {
							return p.fail("duplicate or invalid flag")
						}
						used[f] = true
						seen = true
						switch f {
						case 'i':
							next.fold = !negative
						case 'm':
							next.multi = !negative
						case 's':
							next.dot = !negative
						case 'u':
							next.unicode = !negative
						case 'U': // Greed changes no boolean match result.
						default:
							return p.fail("flag (?" + string(f) + ")")
						}
					}
					if !seen || p.pos == len(p.src) || p.src[p.pos-1] == '-' {
						return p.fail("flags")
					}
					if next.fold && !next.unicode {
						return p.fail("(?i-u) byte case folding")
					}
					if p.src[p.pos] == ')' {
						p.pos++
						flags = next
						lastFlags = true
						continue
					}
					p.pos++
				}
			}
			if !p.meter.charge(3) {
				return p.meter.err
			}
			out.WriteString("(?:")
			if err := p.expression(out, next, true); err != nil {
				return err
			}
			token = ")"
		case '[':
			set, err := p.class(flags)
			if err != nil {
				return err
			}
			if err := p.emit(out, set, regexFlags{unicode: flags.unicode}); err != nil {
				return err
			}
		case '\\':
			set, assertion, err := p.escape(flags, false)
			if err != nil {
				return err
			}
			if assertion != "" {
				token = assertion
			} else if err := p.emit(out, set, flags); err != nil {
				return err
			}
		case '.':
			if !flags.unicode {
				return p.fail("(?-u:.) byte wildcard")
			}
			set := regexUniverse(true)
			if !flags.dot {
				set = p.negate([]regexRange{{'\n', '\n'}}, true)
			}
			if err := p.writeClass(out, set); err != nil {
				return err
			}
		case '^', '$':
			if flags.multi {
				token = "(?m:" + string(c) + ")"
			} else {
				token = string(c)
			}
		case '|', '*', '+', '?':
			token = string(c)
		case '{':
			var counts [2]int
			part, digits := 0, 0
			for p.pos < len(p.src) && p.src[p.pos] != '}' {
				if !p.meter.charge(1) {
					return p.meter.err
				}
				digit := p.src[p.pos]
				p.pos++
				if digit == ',' && part == 0 && digits > 0 {
					part, digits = 1, 0
					continue
				}
				if digit < '0' || digit > '9' {
					return p.fail("repetition")
				}
				digits++
				counts[part] = counts[part]*10 + int(digit-'0')
				if counts[part] > 1000 {
					return p.fail("repetition count (limit 1000; Rust may accept larger counts)")
				}
			}
			if p.pos == len(p.src) {
				return p.fail("unclosed repetition")
			}
			p.pos++
			if part == 0 && digits == 0 {
				return p.fail("repetition")
			}
			token = "{" + strconv.Itoa(counts[0])
			if part == 1 {
				token += ","
				if digits > 0 {
					token += strconv.Itoa(counts[1])
				}
			}
			token += "}"
		default:
			p.pos--
			r, size := utf8.DecodeRuneInString(p.src[p.pos:])
			if r == utf8.RuneError && size == 1 {
				return p.fail("invalid UTF-8 pattern")
			}
			p.pos += size
			if err := p.emit(out, []regexRange{{r, r}}, flags); err != nil {
				return err
			}
		}
		if token != "" {
			if !p.meter.charge(int64(len(token))) {
				return p.meter.err
			}
			out.WriteString(token)
		}
	}
	if grouped {
		return p.fail("unclosed group")
	}
	return nil
}

func (p *regexTranslator) emit(out *strings.Builder, set []regexRange, flags regexFlags) error {
	set = p.normalize(set)
	if !flags.unicode && len(set) > 0 && set[len(set)-1].hi > 127 {
		return p.fail("(?-u:...) non-ASCII byte class")
	}
	if flags.fold {
		set = p.fold(set)
	}
	return p.writeClass(out, set)
}

func (p *regexTranslator) class(flags regexFlags) ([]regexRange, error) {
	negate := false
	if p.pos < len(p.src) && p.src[p.pos] == '^' {
		negate = true
		p.pos++
	}
	var set []regexRange
	first := true
	for p.pos < len(p.src) {
		if !p.meter.charge(0) {
			return nil, p.meter.err
		}
		if p.src[p.pos] == ']' && !first {
			p.pos++
			set = p.normalize(set)
			if flags.fold {
				set = p.fold(set)
			}
			if negate {
				set = p.negate(set, flags.unicode)
			}
			return set, nil
		}
		if strings.HasPrefix(p.src[p.pos:], "&&") || strings.HasPrefix(p.src[p.pos:], "--") || strings.HasPrefix(p.src[p.pos:], "~~") {
			return nil, p.fail("class set operation")
		}
		left, err := p.classItem(flags)
		if err != nil {
			return nil, err
		}
		first = false
		if p.pos+1 < len(p.src) && p.src[p.pos] == '-' && p.src[p.pos+1] != ']' {
			if p.src[p.pos+1] == '-' {
				return nil, p.fail("class set operation --")
			}
			p.pos++
			right, err := p.classItem(flags)
			if err != nil {
				return nil, err
			}
			if len(left) != 1 || left[0].lo != left[0].hi || len(right) != 1 || right[0].lo != right[0].hi || left[0].lo > right[0].lo {
				return nil, p.fail("character class range")
			}
			left = []regexRange{{left[0].lo, right[0].lo}}
		}
		if !p.meter.charge(int64(len(left))) {
			return nil, p.meter.err
		}
		set = append(set, left...)
	}
	return nil, p.fail("unclosed character class")
}

func (p *regexTranslator) classItem(flags regexFlags) ([]regexRange, error) {
	if p.pos == len(p.src) {
		return nil, p.fail("character class")
	}
	if p.src[p.pos] == '[' {
		if !strings.HasPrefix(p.src[p.pos:], "[:") {
			return nil, p.fail("nested character class")
		}
		start := p.pos + 2
		p.pos = start
		for !strings.HasPrefix(p.src[p.pos:], ":]") {
			if p.pos == len(p.src) {
				return nil, p.fail("unclosed POSIX class")
			}
			if !p.meter.charge(1) {
				return nil, p.meter.err
			}
			p.pos++
		}
		name := p.src[start:p.pos]
		p.pos += 2
		name, negate := strings.CutPrefix(name, "^")
		switch name {
		case "alnum", "alpha", "ascii", "blank", "cntrl", "digit", "graph", "lower", "print", "punct", "space", "upper", "word", "xdigit":
		default:
			return nil, p.fail("POSIX class " + name)
		}
		// POSIX class names above have a fixed alphabet and size.
		if !p.meter.charge(64) {
			return nil, p.meter.err
		}
		ast, err := syntax.Parse("[[:"+name+":]]", syntax.Perl)
		if err != nil {
			return nil, p.fail("POSIX class " + name)
		}
		var set []regexRange
		for i := 0; i < len(ast.Rune); i += 2 {
			set = append(set, regexRange{ast.Rune[i], ast.Rune[i+1]})
		}
		if flags.fold {
			set = p.fold(set)
		}
		if negate {
			set = p.negate(set, flags.unicode)
		}
		return set, nil
	}
	if p.src[p.pos] == '\\' {
		p.pos++
		set, _, err := p.escape(flags, true)
		return set, err
	}
	r, size := utf8.DecodeRuneInString(p.src[p.pos:])
	if r == utf8.RuneError && size == 1 {
		return nil, p.fail("invalid UTF-8 pattern")
	}
	p.pos += size
	return []regexRange{{r, r}}, nil
}

func (p *regexTranslator) escape(flags regexFlags, inClass bool) ([]regexRange, string, error) {
	if p.pos == len(p.src) {
		return nil, "", p.fail("trailing escape")
	}
	c := p.src[p.pos]
	p.pos++
	var set []regexRange
	switch c {
	case 'w', 'W', 'd', 'D', 's', 'S':
		if flags.unicode {
			switch c {
			case 'w', 'W':
				set = rustWord
			case 'd', 'D':
				set = rustDecimal
			default:
				set = rustSpace
			}
		} else {
			switch c {
			case 'w', 'W':
				set = []regexRange{{'0', '9'}, {'A', 'Z'}, {'_', '_'}, {'a', 'z'}}
			case 'd', 'D':
				set = []regexRange{{'0', '9'}}
			default:
				set = []regexRange{{9, 10}, {12, 13}, {32, 32}}
			}
		}
		if c == 'W' || c == 'D' || c == 'S' {
			if flags.fold {
				set = p.fold(set)
			}
			set = p.negate(set, flags.unicode)
		}
	case 'b', 'B':
		if inClass {
			return nil, "", p.fail("word assertion in character class")
		}
		if p.pos < len(p.src) && p.src[p.pos] == '{' {
			return nil, "", p.fail("\\" + string(c) + "{...} word assertion")
		}
		p.boundary = true
		assertion := "\\" + string(c)
		if !flags.unicode {
			p.asciiID++
			assertion = fmt.Sprintf("(?P<tsgAsciiBoundary%d>%s)", p.asciiID, assertion)
		}
		return nil, assertion, nil
	case 'A', 'z':
		if inClass {
			return nil, "", p.fail("text assertion in character class")
		}
		return nil, "\\" + string(c), nil
	case 'p', 'P':
		if !flags.unicode {
			return nil, "", p.fail("Unicode property with (?-u)")
		}
		start := p.pos
		if p.pos < len(p.src) && p.src[p.pos] == '{' {
			p.pos++
			start = p.pos
			for p.pos < len(p.src) && p.src[p.pos] != '}' {
				if !p.meter.charge(1) {
					return nil, "", p.meter.err
				}
				p.pos++
			}
			if p.pos == len(p.src) {
				return nil, "", p.fail("Unicode property")
			}
			name := p.src[start:p.pos]
			p.pos++
			var err error
			set, err = p.property(name)
			if err != nil {
				return nil, "", err
			}
		} else {
			if p.pos == len(p.src) {
				return nil, "", p.fail("Unicode property")
			}
			p.pos++
			var err error
			set, err = p.property(p.src[start:p.pos])
			if err != nil {
				return nil, "", err
			}
		}
		if flags.fold {
			set = p.fold(set)
		}
		if c == 'P' {
			set = p.negate(set, true)
		}
	case 'x', 'u', 'U':
		n := 2
		if c == 'u' {
			n = 4
		} else if c == 'U' {
			n = 8
		}
		start := p.pos
		if p.pos < len(p.src) && p.src[p.pos] == '{' {
			p.pos++
			start = p.pos
			for p.pos < len(p.src) && p.src[p.pos] != '}' {
				if !p.meter.charge(1) {
					return nil, "", p.meter.err
				}
				p.pos++
			}
			if p.pos == len(p.src) {
				return nil, "", p.fail("hex escape")
			}
			n = p.pos - start
			p.pos++
		} else {
			if p.pos+n > len(p.src) {
				return nil, "", p.fail("hex escape")
			}
			p.pos += n
		}
		if n < 1 || n > 8 {
			return nil, "", p.fail("hex escape")
		}
		v, err := strconv.ParseUint(p.src[start:start+n], 16, 32)
		if err != nil || v > utf8.MaxRune || v >= 0xD800 && v <= 0xDFFF {
			return nil, "", p.fail("Unicode scalar escape")
		}
		r := rune(v)
		set = []regexRange{{r, r}}
	case 'a':
		set = []regexRange{{7, 7}}
	case 'f':
		set = []regexRange{{12, 12}}
	case 't':
		set = []regexRange{{9, 9}}
	case 'n':
		set = []regexRange{{10, 10}}
	case 'r':
		set = []regexRange{{13, 13}}
	case 'v':
		set = []regexRange{{11, 11}}
	default:
		if c > 127 || c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '<' || c == '>' {
			return nil, "", p.fail("escape \\" + string(c))
		}
		set = []regexRange{{rune(c), rune(c)}}
	}
	return set, "", nil
}

func (p *regexTranslator) property(name string) ([]regexRange, error) {
	tab := unicode.Categories[name]
	if tab == nil {
		tab = unicode.Scripts[name]
	}
	if tab == nil {
		return nil, p.fail("Unicode property \\p{" + name + "} (only Go general categories and scripts are supported)")
	}
	var set []regexRange
	for _, r := range tab.R16 {
		if r.Stride == 1 {
			if !p.meter.charge(1) {
				return nil, p.meter.err
			}
			set = append(set, regexRange{rune(r.Lo), rune(r.Hi)})
			continue
		}
		for v := r.Lo; v <= r.Hi; {
			if !p.meter.charge(1) {
				return nil, p.meter.err
			}
			set = append(set, regexRange{rune(v), rune(v)})
			if r.Hi-v < r.Stride {
				break
			}
			v += r.Stride
		}
	}
	for _, r := range tab.R32 {
		if r.Stride == 1 {
			if !p.meter.charge(1) {
				return nil, p.meter.err
			}
			set = append(set, regexRange{rune(r.Lo), rune(r.Hi)})
			continue
		}
		for v := r.Lo; v <= r.Hi; v += r.Stride {
			if !p.meter.charge(1) {
				return nil, p.meter.err
			}
			set = append(set, regexRange{rune(v), rune(v)})
		}
	}
	return p.normalize(set), nil
}

func (p *regexTranslator) normalize(set []regexRange) []regexRange {
	if !p.meter.charge(int64(len(set))) {
		return nil
	}
	clean := make([]regexRange, 0, len(set))
	for _, r := range set {
		if !p.meter.charge(0) {
			return nil
		}
		if r.lo < 0 || r.hi > utf8.MaxRune || r.lo > r.hi {
			continue
		}
		if r.lo <= 0xDFFF && r.hi >= 0xD800 {
			if r.lo < 0xD800 {
				if !p.meter.charge(1) {
					return nil
				}
				clean = append(clean, regexRange{r.lo, 0xD7FF})
			}
			if r.hi <= 0xDFFF {
				continue
			}
			r.lo = 0xE000
		}
		clean = append(clean, r)
	}
	set = clean
	sort.Slice(set, func(i, j int) bool { return set[i].lo < set[j].lo || set[i].lo == set[j].lo && set[i].hi < set[j].hi })
	out := set[:0]
	for _, r := range set {
		if !p.meter.charge(0) {
			return nil
		}
		if len(out) > 0 && r.lo <= out[len(out)-1].hi+1 {
			if r.hi > out[len(out)-1].hi {
				out[len(out)-1].hi = r.hi
			}
		} else {
			out = append(out, r)
		}
	}
	return out
}

func regexContains(set []regexRange, r rune) bool {
	i := sort.Search(len(set), func(i int) bool { return set[i].hi >= r })
	return i < len(set) && set[i].lo <= r
}
func regexIsWord(r rune) bool { return regexContains(rustWord, r) }
func (p *regexTranslator) fold(set []regexRange) []regexRange {
	set = p.normalize(set)
	if !p.meter.charge(int64(len(set))) {
		return nil
	}
	out := append([]regexRange(nil), set...)
	for _, f := range rustFold {
		if !p.meter.charge(0) {
			return nil
		}
		if regexContains(set, f.from) {
			if !p.meter.charge(1) {
				return nil
			}
			out = append(out, regexRange{f.to, f.to})
		}
	}
	return p.normalize(out)
}
func regexUniverse(unicode bool) []regexRange {
	if !unicode {
		return []regexRange{{0, 255}}
	}
	return []regexRange{{0, 0xD7FF}, {0xE000, utf8.MaxRune}}
}
func (p *regexTranslator) negate(set []regexRange, unicode bool) []regexRange {
	set = p.normalize(set)
	var out []regexRange
	for _, u := range regexUniverse(unicode) {
		start := u.lo
		for _, r := range set {
			if !p.meter.charge(0) {
				return nil
			}
			if r.hi < start {
				continue
			}
			if r.lo > u.hi {
				break
			}
			if r.lo > start {
				if !p.meter.charge(1) {
					return nil
				}
				out = append(out, regexRange{start, r.lo - 1})
			}
			if r.hi >= start {
				start = r.hi + 1
			}
			if start > u.hi {
				break
			}
		}
		if start <= u.hi {
			if !p.meter.charge(1) {
				return nil
			}
			out = append(out, regexRange{start, u.hi})
		}
	}
	return out
}
func (p *regexTranslator) writeClass(out *strings.Builder, set []regexRange) error {
	if len(set) == 1 && set[0].lo == set[0].hi {
		s := regexp.QuoteMeta(string(set[0].lo))
		if !p.meter.charge(int64(len(s))) {
			return p.meter.err
		}
		out.WriteString(s)
		return nil
	}
	if len(set) == 0 {
		const empty = `[^\x{0}-\x{10FFFF}]`
		if !p.meter.charge(int64(len(empty))) {
			return p.meter.err
		}
		out.WriteString(empty)
		return nil
	}
	if !p.meter.charge(int64(len("(?-i:[])"))) {
		return p.meter.err
	}
	out.WriteString("(?-i:[")
	for _, r := range set {
		// A scalar escape takes at most ten bytes for each endpoint.
		n := int64(10)
		if r.hi != r.lo {
			n = 21
		}
		if !p.meter.charge(n) {
			return p.meter.err
		}
		fmt.Fprintf(out, `\x{%X}`, r.lo)
		if r.hi != r.lo {
			fmt.Fprintf(out, `-\x{%X}`, r.hi)
		}
	}
	out.WriteString("])")
	return nil
}
