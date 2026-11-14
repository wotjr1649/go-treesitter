package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const noticeDir = "LICENSES/wasi-libc/"

// wasiLibcNotices returns the license texts of the libc linked into every module, keyed by module-relative slash path.
// Each source file is checked against its pinned sha256 first, so the copies are the same bytes on every host.
func wasiLibcNotices(o opts, pins *Pins) (map[string][]byte, error) {
	wl := pins.WasiLibc
	out := map[string][]byte{}
	for _, n := range wl.Notices {
		b, err := os.ReadFile(filepath.Join(o.src, wl.ExtractDir, filepath.FromSlash(n.Path)))
		if err != nil {
			return nil, err
		}
		if err := expect("wasi-libc "+n.Path, sumBytes(b), n.SHA256); err != nil {
			return nil, err
		}
		if n.LeadingLines > 0 {
			parts := bytes.SplitAfterN(b, []byte("\n"), n.LeadingLines+1)
			if len(parts) <= n.LeadingLines {
				return nil, fmt.Errorf("wasi-libc %s has fewer than %d lines", n.Path, n.LeadingLines)
			}
			b = bytes.Join(parts[:n.LeadingLines], nil)
		}
		out[noticeDir+n.Name] = b
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("pins.json lists no wasi-libc notices")
	}
	return out, nil
}

// checkWasiLibcCommit ties the notice source to the sdk that is actually used: sdk VERSION has "wasi-libc: <12 hex>".
func checkWasiLibcCommit(sdkVersion string, pins *Pins) error {
	for _, l := range strings.Split(sdkVersion, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), "wasi-libc:"); ok {
			v = strings.TrimSpace(v)
			if len(v) < 12 || !strings.HasPrefix(pins.WasiLibc.Commit, v) {
				return fmt.Errorf("pin mismatch: wasi-sdk VERSION wasi-libc = %s, pinned %s", v, pins.WasiLibc.Commit)
			}
			return nil
		}
	}
	return fmt.Errorf("wasi-sdk VERSION has no wasi-libc line")
}

const runtimeNoticePath = "LICENSES/tree-sitter.txt"

// runtimeNotice is the MIT license of the tree-sitter runtime C that every module contains.
func runtimeNotice(o opts, pins *Pins) (map[string][]byte, error) {
	rt := pins.Runtime
	b, err := os.ReadFile(filepath.Join(o.src, rt.ExtractDir, "LICENSE"))
	if err != nil {
		return nil, err
	}
	if err := expect("runtime LICENSE", sumBytes(b), rt.LicenseSHA256); err != nil {
		return nil, err
	}
	return map[string][]byte{runtimeNoticePath: b}, nil
}

// nonLibcMembers parses `wasm-ld -t` output (via clang -Wl,-t) and returns the archive members linked from any
// archive other than libc.a, plus how many libc.a members were linked. Object files are not archive members.
func nonLibcMembers(trace string) (others []string, libc int) {
	for _, l := range strings.Split(trace, "\n") {
		l = strings.TrimSpace(l)
		i := strings.LastIndex(l, ".a(")
		if i < 0 || !strings.HasSuffix(l, ")") {
			continue
		}
		arch := l[:i+2]
		if j := strings.LastIndexAny(arch, `/\`); j >= 0 {
			arch = arch[j+1:]
		}
		if arch == "libc.a" {
			libc++
		} else {
			others = append(others, l)
		}
	}
	return others, libc
}

// checkLink re-links the grammar with the linker trace on, in a scratch file outside the recipe (the trace does not change
// the output and is not part of RecipeSHA256), and fails if anything but libc.a members is linked. The notices only cover
// wasi-libc; a linked libclang_rt.builtins (Apache-2.0 WITH LLVM-exception) or libm member would need its own.
func checkLink(o opts, pins *Pins, g GrammarPin, scratch string) error {
	out, err := compile(o, pins, g, scratch, "-Wl,-t")
	if err != nil {
		return err
	}
	others, libc := nonLibcMembers(out)
	if libc == 0 {
		return fmt.Errorf("link check: no libc.a member in the linker trace (trace format changed?)")
	}
	if len(others) > 0 {
		return fmt.Errorf("link check: %s links archive members other than libc.a, whose license notices are not shipped: %s", o.lang, strings.Join(others, ", "))
	}
	return nil
}
