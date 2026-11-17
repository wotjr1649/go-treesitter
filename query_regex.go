package treesitter

import (
	"fmt"
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
	if !re.boundary && utf8.ValidString(text) {
		return re.re.MatchString(text)
	}
	return regexVMMatch(re.prog, text, re.asciiBoundaries)
}

func compileQueryRegex(pattern string) (*queryRegex, error) {
	p := regexTranslator{src: pattern}
	canonical, err := p.expression(regexFlags{unicode: true}, false)
	if err != nil {
		return nil, err
	}
	ast, err := syntax.Parse(canonical, syntax.Perl)
	if err != nil {
		return nil, fmt.Errorf("query regex: %w", err)
	}
	asciiCaps := map[uint32]bool{}
	var visit func(*syntax.Regexp)
	visit = func(r *syntax.Regexp) {
		if r.Op == syntax.OpCapture && strings.HasPrefix(r.Name, "tsgAsciiBoundary") {
			asciiCaps[uint32(2*r.Cap)] = true
		}
		for _, sub := range r.Sub {
			visit(sub)
		}
	}
	visit(ast)
	prog, err := syntax.Compile(ast.Simplify())
	if err != nil {
		return nil, fmt.Errorf("query regex: %w", err)
	}
	asciiPC := map[uint32]bool{}
	for _, inst := range prog.Inst {
		if inst.Op == syntax.InstCapture && asciiCaps[inst.Arg] {
			asciiPC[inst.Out] = true
		}
	}
	re, err := regexp.Compile(canonical)
	if err != nil {
		return nil, fmt.Errorf("query regex: %w", err)
	}
	return &queryRegex{re: re, prog: prog, boundary: p.boundary, asciiBoundaries: asciiPC}, nil
}

type regexFlags struct{ fold, multi, dot, unicode bool }
type regexTranslator struct {
	src                 string
	pos, asciiID, depth int
	boundary            bool
	names               map[string]bool
}

func (p *regexTranslator) fail(construct string) error {
	return fmt.Errorf("query regex at byte %d: unsupported or invalid %s", p.pos, construct)
}

func (p *regexTranslator) expression(flags regexFlags, grouped bool) (string, error) {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > 100 {
		return "", p.fail("group nesting (limit 100)")
	}
	var out strings.Builder
	lastFlags := false
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		p.pos++
		if lastFlags && strings.ContainsRune("*+?{", rune(c)) {
			return "", p.fail("repetition immediately after flags")
		}
		lastFlags = false
		switch c {
		case ')':
			if !grouped {
				return "", p.fail("unmatched )")
			}
			return out.String(), nil
		case '(':
			next := flags
			if p.pos < len(p.src) && p.src[p.pos] == '?' {
				p.pos++
				if p.pos == len(p.src) {
					return "", p.fail("group")
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
						p.pos++
					}
					if p.pos == len(p.src) {
						return "", p.fail("named group")
					}
					name := p.src[start:p.pos]
					p.pos++
					if name == "" {
						return "", p.fail("empty group name")
					}
					if p.names == nil {
						p.names = map[string]bool{}
					}
					if p.names[name] {
						return "", p.fail("duplicate group name " + name)
					}
					p.names[name] = true
					for i, r := range name {
						if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
							return "", p.fail("named group " + name + " (use ASCII letters, digits and _)")
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
							return "", p.fail("duplicate or invalid flag")
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
							return "", p.fail("flag (?" + string(f) + ")")
						}
					}
					if !seen || p.pos == len(p.src) || p.src[p.pos-1] == '-' {
						return "", p.fail("flags")
					}
					if next.fold && !next.unicode {
						return "", p.fail("(?i-u) byte case folding")
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
			sub, err := p.expression(next, true)
			if err != nil {
				return "", err
			}
			out.WriteString("(?:" + sub + ")")
		case '[':
			set, err := p.class(flags)
			if err != nil {
				return "", err
			}
			emitted, err := p.emit(set, regexFlags{unicode: flags.unicode})
			if err != nil {
				return "", err
			}
			out.WriteString(emitted)
		case '\\':
			set, assertion, err := p.escape(flags, false)
			if err != nil {
				return "", err
			}
			if assertion != "" {
				out.WriteString(assertion)
			} else {
				emitted, err := p.emit(set, flags)
				if err != nil {
					return "", err
				}
				out.WriteString(emitted)
			}
		case '.':
			if !flags.unicode {
				return "", p.fail("(?-u:.) byte wildcard")
			}
			set := regexUniverse(true)
			if !flags.dot {
				set = regexNegate([]regexRange{{'\n', '\n'}}, true)
			}
			out.WriteString(regexClass(set))
		case '^', '$':
			if flags.multi {
				out.WriteString("(?m:" + string(c) + ")")
			} else {
				out.WriteByte(c)
			}
		case '|', '*', '+', '?':
			out.WriteByte(c)
		case '{':
			start := p.pos
			for p.pos < len(p.src) && p.src[p.pos] != '}' {
				if p.src[p.pos] != ',' && (p.src[p.pos] < '0' || p.src[p.pos] > '9') {
					return "", p.fail("repetition")
				}
				p.pos++
			}
			if p.pos == len(p.src) {
				return "", p.fail("unclosed repetition")
			}
			spec := p.src[start:p.pos]
			p.pos++
			parts := strings.Split(spec, ",")
			if len(parts) > 2 || parts[0] == "" {
				return "", p.fail("repetition")
			}
			for i, part := range parts {
				if part == "" {
					continue
				}
				n, e := strconv.Atoi(part)
				if e != nil || n > 1000 {
					return "", p.fail("repetition count (limit 1000; Rust may accept larger counts)")
				}
				parts[i] = strconv.Itoa(n)
			}
			out.WriteString("{" + strings.Join(parts, ",") + "}")
		default:
			p.pos--
			r, size := utf8.DecodeRuneInString(p.src[p.pos:])
			if r == utf8.RuneError && size == 1 {
				return "", p.fail("invalid UTF-8 pattern")
			}
			p.pos += size
			emitted, err := p.emit([]regexRange{{r, r}}, flags)
			if err != nil {
				return "", err
			}
			out.WriteString(emitted)
		}
	}
	if grouped {
		return "", p.fail("unclosed group")
	}
	return out.String(), nil
}

func (p *regexTranslator) emit(set []regexRange, flags regexFlags) (string, error) {
	set = regexNormalize(set)
	if !flags.unicode && len(set) > 0 && set[len(set)-1].hi > 127 {
		return "", p.fail("(?-u:...) non-ASCII byte class")
	}
	if flags.fold {
		set = regexFoldSet(set)
	}
	return regexClass(set), nil
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
		if p.src[p.pos] == ']' && !first {
			p.pos++
			set = regexNormalize(set)
			if flags.fold {
				set = regexFoldSet(set)
			}
			if negate {
				set = regexNegate(set, flags.unicode)
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
		end := strings.Index(p.src[p.pos+2:], ":]") + 2
		if end < 2 {
			return nil, p.fail("unclosed POSIX class")
		}
		name := p.src[p.pos+2 : p.pos+end]
		p.pos += end + 2
		name, negate := strings.CutPrefix(name, "^")
		switch name {
		case "alnum", "alpha", "ascii", "blank", "cntrl", "digit", "graph", "lower", "print", "punct", "space", "upper", "word", "xdigit":
		default:
			return nil, p.fail("POSIX class " + name)
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
			set = regexFoldSet(set)
		}
		if negate {
			set = regexNegate(set, flags.unicode)
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
				set = regexFoldSet(set)
			}
			set = regexNegate(set, flags.unicode)
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
			set = regexFoldSet(set)
		}
		if c == 'P' {
			set = regexNegate(set, true)
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
			set = append(set, regexRange{rune(r.Lo), rune(r.Hi)})
			continue
		}
		for v := r.Lo; v <= r.Hi; {
			set = append(set, regexRange{rune(v), rune(v)})
			if r.Hi-v < r.Stride {
				break
			}
			v += r.Stride
		}
	}
	for _, r := range tab.R32 {
		if r.Stride == 1 {
			set = append(set, regexRange{rune(r.Lo), rune(r.Hi)})
			continue
		}
		for v := r.Lo; v <= r.Hi; v += r.Stride {
			set = append(set, regexRange{rune(v), rune(v)})
		}
	}
	return regexNormalize(set), nil
}

func regexNormalize(set []regexRange) []regexRange {
	clean := make([]regexRange, 0, len(set))
	for _, r := range set {
		if r.lo < 0 || r.hi > utf8.MaxRune || r.lo > r.hi {
			continue
		}
		if r.lo <= 0xDFFF && r.hi >= 0xD800 {
			if r.lo < 0xD800 {
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
func regexFoldSet(set []regexRange) []regexRange {
	set = regexNormalize(set)
	out := append([]regexRange(nil), set...)
	for _, f := range rustFold {
		if regexContains(set, f.from) {
			out = append(out, regexRange{f.to, f.to})
		}
	}
	return regexNormalize(out)
}
func regexUniverse(unicode bool) []regexRange {
	if !unicode {
		return []regexRange{{0, 255}}
	}
	return []regexRange{{0, 0xD7FF}, {0xE000, utf8.MaxRune}}
}
func regexNegate(set []regexRange, unicode bool) []regexRange {
	set = regexNormalize(set)
	var out []regexRange
	for _, u := range regexUniverse(unicode) {
		start := u.lo
		for _, r := range set {
			if r.hi < start {
				continue
			}
			if r.lo > u.hi {
				break
			}
			if r.lo > start {
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
			out = append(out, regexRange{start, u.hi})
		}
	}
	return out
}
func regexClass(set []regexRange) string {
	if len(set) == 1 && set[0].lo == set[0].hi {
		return regexp.QuoteMeta(string(set[0].lo))
	}
	if len(set) == 0 {
		return `[^\x{0}-\x{10FFFF}]`
	}
	var b strings.Builder
	b.WriteString("(?-i:[")
	for _, r := range set {
		fmt.Fprintf(&b, `\x{%X}`, r.lo)
		if r.hi != r.lo {
			fmt.Fprintf(&b, `-\x{%X}`, r.hi)
		}
	}
	b.WriteString("])")
	return b.String()
}
