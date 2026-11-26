package main

import (
	"fmt"
	"go/format"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ---- guard-page memory access (ADR-0031) ----
//
// patchGuardPages rewrites the wasm2go v0.4.22 output so that no load/store checks the address in software. That is
// only memory safe because engine.InitGuardedMemory makes StartMemory reserve 4 GiB + engine.GuardSize for the instance
// and commit only its memory: any other address faults, and engine.Call turns the fault into *engine.Trap. The call
// sites keep their form (load32(mem, addr), mem[addr]); only the helper bodies and the byte accesses change, so the
// local "mem := m.memory" copies of v0.4.22 stay (the base never moves). memory_copy/fill/init keep their slice checks.
// The patch fails closed: any text it does not recognise is an error, never a pass-through.

// helperBlockSHA256 is the sha256 of the original load16..store64 helper text (wasm2go v0.4.22).
const helperBlockSHA256 = "7d5b4ec86e171f41363f1552300f319d5058f7cfc8d89cba1f700ae1790eeb20"

const guardHelpers = "//go:nosplit\n" +
	"func load16[T uint32 | uint64](mem []byte, addr T) uint16 {\n\tval := *(*uint16)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(mem)), uintptr(addr)))\n\tif big {\n\t\treturn bits.ReverseBytes16(val)\n\t}\n\treturn val\n}\n\n" +
	"//go:nosplit\n" +
	"func store16[T uint32 | uint64](mem []byte, addr T, val uint16) {\n\tif big {\n\t\tval = bits.ReverseBytes16(val)\n\t}\n\t*(*uint16)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(mem)), uintptr(addr))) = val\n}\n\n" +
	"//go:nosplit\n" +
	"func load32[T uint32 | uint64](mem []byte, addr T) uint32 {\n\tval := *(*uint32)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(mem)), uintptr(addr)))\n\tif big {\n\t\treturn bits.ReverseBytes32(val)\n\t}\n\treturn val\n}\n\n" +
	"//go:nosplit\n" +
	"func store32[T uint32 | uint64](mem []byte, addr T, val uint32) {\n\tif big {\n\t\tval = bits.ReverseBytes32(val)\n\t}\n\t*(*uint32)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(mem)), uintptr(addr))) = val\n}\n\n" +
	"//go:nosplit\n" +
	"func load64[T uint32 | uint64](mem []byte, addr T) uint64 {\n\tval := *(*uint64)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(mem)), uintptr(addr)))\n\tif big {\n\t\treturn bits.ReverseBytes64(val)\n\t}\n\treturn val\n}\n\n" +
	"//go:nosplit\n" +
	"func store64[T uint32 | uint64](mem []byte, addr T, val uint64) {\n\tif big {\n\t\tval = bits.ReverseBytes64(val)\n\t}\n\t*(*uint64)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(mem)), uintptr(addr))) = val\n}\n\n"

var (
	initMemRx = regexp.MustCompile("(?m)^\\tengine\\.InitMemory\\(&m\\.memory, ")
	// an access is a load/store helper call or a byte index on the memory slice (local copy or field)
	callRx   = regexp.MustCompile("(?:load|store)(?:16|32|64)\\((?:mem|m\\.memory), ")
	indexRx  = regexp.MustCompile("(?:^|[^\\w.])((?:mem|m\\.memory)\\[)")
	addrRx   = regexp.MustCompile("^uint64\\(uint32\\(.*\\)\\)(?:\\+(0x[0-9a-fA-F]+|[0-9]+))?$")
	funcRx   = regexp.MustCompile("^func (\\(m \\*Module\\) )?(\\w+)")
	maxMemRx = regexp.MustCompile("(?m)^const maxMem = ")
)

// patchGuardPages returns the rewritten source and the largest static offset any access uses.
func patchGuardPages(src string) (string, int, error) {
	a := strings.Index(src, "//go:nosplit\nfunc load16[")
	b := strings.Index(src, "//go:nosplit\nfunc i32(")
	if a < 0 || b < a {
		return "", 0, fmt.Errorf("guard pages: load/store helpers not found")
	}
	if got := sumBytes([]byte(src[a:b])); got != helperBlockSHA256 {
		return "", 0, fmt.Errorf("guard pages: helper block sha256 %s, pinned %s (wasm2go output changed)", got, helperBlockSHA256)
	}
	src = src[:a] + guardHelpers + src[b:]
	if !strings.Contains(src, "binary.") {
		src = strings.Replace(src, "\t\"encoding/binary\"\n", "", 1)
	}
	if strings.Count(src, "engine.InitMemory(") != 1 || len(initMemRx.FindAllStringIndex(src, -1)) != 1 || strings.Contains(src, "make([]byte") {
		return "", 0, fmt.Errorf("guard pages: want exactly one engine.InitMemory call in New and no heap memory")
	}
	src = initMemRx.ReplaceAllString(src, "\tif !unalignedOK {\n\t\tpanic(\"guard pages need unaligned memory access\")\n\t}\n\tengine.InitGuardedMemory(&m.memory, ")

	// Rewrite method by method; memory_copy/fill/init keep their slice checks, any other top-level code must not touch memory.
	lines := strings.Split(src, "\n")
	maxOff, inMethod, fn := 0, false, ""
	for i, ln := range lines {
		if m := funcRx.FindStringSubmatch(ln); m != nil {
			inMethod, fn = m[1] != "", m[2]
		}
		if !callRx.MatchString(ln) && !indexRx.MatchString(ln) {
			continue
		}
		if !inMethod {
			if strings.HasPrefix(fn, "memory_") {
				continue
			}
			return "", 0, fmt.Errorf("guard pages: memory access outside a generated method (%s): %q", fn, ln)
		}
		nl, off, err := rewriteAccesses(ln)
		if err != nil {
			return "", 0, err
		}
		maxOff, lines[i] = max(maxOff, off), nl
	}
	src = strings.Join(lines, "\n")
	// The module no longer checks addresses: any static offset + access size must fit the guard.
	if len(maxMemRx.FindAllStringIndex(src, -1)) != 1 {
		return "", 0, fmt.Errorf("guard pages: const maxMem not found exactly once")
	}
	src = maxMemRx.ReplaceAllString(src, fmt.Sprintf("// Largest static offset in this module is %d; an access is at most 8 bytes. A smaller engine.GuardSize fails to compile.\nconst _ = uint(engine.GuardSize - %d)\n\nconst maxMem = ", maxOff, maxOff+8))
	out, err := format.Source([]byte(src))
	if err != nil {
		return "", 0, fmt.Errorf("guard pages: patched source does not parse: %w", err)
	}
	return string(out), maxOff, nil
}

// balancedEnd returns the index of the first top-level ',' ']' or ')' at or after k (the end of an address argument).
func balancedEnd(ln string, k int) int {
	depth := 0
	for ; k < len(ln); k++ {
		switch c := ln[k]; {
		case c == '(' || c == '[':
			depth++
		case c == ')' || c == ']':
			if depth == 0 {
				return k
			}
			depth--
		case c == ',' && depth == 0:
			return k
		}
	}
	return -1
}

// staticOffset checks the address form and returns its static offset.
func staticOffset(addr string) (int, error) {
	m := addrRx.FindStringSubmatch(addr)
	if m == nil {
		return 0, fmt.Errorf("guard pages: unrecognised address form %q", addr)
	}
	if m[1] == "" {
		return 0, nil
	}
	off, err := strconv.ParseInt(m[1], 0, 64)
	return int(off), err
}

// rewriteAccesses validates every access on one line, turns byte indexes into base-pointer form and returns the
// largest static offset. Load/store helper calls keep their text: the helper bodies were replaced.
func rewriteAccesses(ln string) (string, int, error) {
	maxOff := 0
	for _, loc := range callRx.FindAllStringIndex(ln, -1) {
		k := balancedEnd(ln, loc[1])
		if k < 0 || ln[k] == ']' { // a load ends at ')', a store at ','
			return "", 0, fmt.Errorf("guard pages: unterminated access in %q", ln)
		}
		off, err := staticOffset(ln[loc[1]:k])
		if err != nil {
			return "", 0, err
		}
		maxOff = max(maxOff, off)
	}
	var sb strings.Builder
	for {
		loc := indexRx.FindStringSubmatchIndex(ln)
		if loc == nil {
			sb.WriteString(ln)
			return sb.String(), maxOff, nil
		}
		k := balancedEnd(ln, loc[3])
		if k < 0 || ln[k] != ']' {
			return "", 0, fmt.Errorf("guard pages: byte access not closed by ] in %q", ln)
		}
		addr, mem := ln[loc[3]:k], ln[loc[2]:loc[3]-1]
		off, err := staticOffset(addr)
		if err != nil {
			return "", 0, err
		}
		// the address may hold further accesses: rewrite it as its own line
		inner, innerOff, err := rewriteAccesses(addr)
		if err != nil {
			return "", 0, err
		}
		maxOff = max(maxOff, off, innerOff)
		sb.WriteString(ln[:loc[2]] + "*(*byte)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(" + mem + ")), " + inner + "))")
		ln = ln[k+1:]
	}
}

// runGuardPagesCommand applies patchGuardPages to a generated engine_gen.go: gen guard-pages <in> <out>.
func runGuardPagesCommand(args []string, w io.Writer) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: gen guard-pages <engine_gen.go in> <out>")
	}
	raw, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	patched, off, err := patchGuardPages(string(raw))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "max static offset %d\n", off)
	return os.WriteFile(args[1], []byte(patched), 0o644)
}
