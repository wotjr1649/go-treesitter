package treesitter

import (
	"regexp/syntax"
	"unicode/utf8"
)

// regexVMMatch runs an unanchored boolean search with O(len(text)*len(prog.Inst))
// work and O(len(prog.Inst)) per-call storage. Capture priorities are irrelevant.
func regexVMMatch(prog *syntax.Prog, text string, asciiBoundaries map[uint32]bool) bool {
	seen := make([]int, len(prog.Inst))
	pending := make([]uint32, 0, len(prog.Inst))
	next := make([]uint32, 0, len(prog.Inst))
	previous, previousValid := rune(-1), true
	for pos, generation := 0, 1; ; generation++ {
		current, width := rune(-1), 0
		valid := true
		if pos < len(text) {
			current, width = utf8.DecodeRuneInString(text[pos:])
			valid = current != utf8.RuneError || width != 1
		}
		unicodeFlags, asciiFlags := regexEmptyFlags(text[:pos], previous, current, previousValid, valid)
		next = next[:0]
		for interior := false; ; interior = true {
			if interior {
				// Every interior byte has the same assertions; no Unicode consumer can end there.
				if width < 2 || len(asciiBoundaries) == 0 {
					break
				}
				generation++
				unicodeFlags, asciiFlags = 0, syntax.EmptyNoWordBoundary
			}
			consume := !interior && width != 0 && valid
			pending = append(pending, uint32(prog.Start))
			for len(pending) > 0 {
				last := len(pending) - 1
				pc := pending[last]
				pending = pending[:last]
				if seen[pc] == generation {
					continue
				}
				seen[pc] = generation
				inst := &prog.Inst[pc]
				switch inst.Op {
				case syntax.InstMatch:
					return true
				case syntax.InstAlt, syntax.InstAltMatch:
					pending = append(pending, inst.Out, inst.Arg)
				case syntax.InstCapture, syntax.InstNop:
					pending = append(pending, inst.Out)
				case syntax.InstEmptyWidth:
					flags := unicodeFlags
					if asciiBoundaries[pc] {
						flags = asciiFlags
					}
					if required := syntax.EmptyOp(inst.Arg); flags&required == required {
						pending = append(pending, inst.Out)
					}
				case syntax.InstRune, syntax.InstRune1:
					if consume && inst.MatchRune(current) {
						next = append(next, inst.Out)
					}
				case syntax.InstRuneAny:
					if consume {
						next = append(next, inst.Out)
					}
				case syntax.InstRuneAnyNotNL:
					if consume && current != '\n' {
						next = append(next, inst.Out)
					}
				}
			}
			if interior {
				break
			}
		}
		if width == 0 {
			return false
		}
		pending, next = next, pending
		pos += width
		previous, previousValid = current, valid
	}
}

func regexEmptyFlags(prefix string, previous, current rune, previousValid, currentValid bool) (unicodeFlags, asciiFlags syntax.EmptyOp) {
	if previous == -1 {
		unicodeFlags |= syntax.EmptyBeginText | syntax.EmptyBeginLine
	} else if previous == '\n' {
		unicodeFlags |= syntax.EmptyBeginLine
	}
	if current == -1 {
		unicodeFlags |= syntax.EmptyEndText | syntax.EmptyEndLine
	} else if current == '\n' {
		unicodeFlags |= syntax.EmptyEndLine
	}
	asciiFlags = unicodeFlags
	if syntax.IsWordChar(previous) != syntax.IsWordChar(current) {
		asciiFlags |= syntax.EmptyWordBoundary
	} else {
		asciiFlags |= syntax.EmptyNoWordBoundary
	}
	if !previousValid {
		// regex-automata 0.4.18 decode_last decodes the first rune in this window, even if it ends before the assertion.
		start, limit := len(prefix)-1, max(0, len(prefix)-utf8.UTFMax)
		for start > limit && !utf8.RuneStart(prefix[start]) {
			start--
		}
		var width int
		previous, width = utf8.DecodeRuneInString(prefix[start:])
		previousValid = previous != utf8.RuneError || width != 1
	}
	if (previousValid && regexIsWord(previous)) != (currentValid && regexIsWord(current)) {
		unicodeFlags |= syntax.EmptyWordBoundary
	} else if previousValid && currentValid {
		// Rust's Unicode \B rejects invalid UTF-8 on either side of the assertion.
		unicodeFlags |= syntax.EmptyNoWordBoundary
	}
	return unicodeFlags, asciiFlags
}
