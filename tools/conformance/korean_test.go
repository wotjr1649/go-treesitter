package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	proc "github.com/wotjr1649/go-treesitter/tools/bench/proc"

	treesitter "github.com/wotjr1649/go-treesitter"
)

const koreanDir = "testdata/korean"

// TestCorpusKorean: the retained Korean (Hangul) inputs of testdata/korean, per grammar, through the product.
//
//   - class clean: no ERROR/MISSING node, and every manifest expectation holds (koreanCheck). "s-e:kind" = the deepest
//     node covering the non-ASCII run [s,e) is a named node of that kind; "s-e=kind" = a node of that kind spans exactly
//     [s,e) (heredoc delimiters, and the extent of every covering node wider than its run). The ":" spans must be exactly
//     the input's runs (koreanRuns), so a run left out of the manifest, or a node grown past its recorded extent, fails.
//   - class known-gap: the tree has errors and its dump is byte-identical to the recorded one (dump_sha256): fixing the
//     upstream grammar, or any other change, fails the test; then move the row to clean (or retire it).
//
// Without TSG_NATIVE_EXE_DIR this is E3 (engine only). With it, each product dump must equal `<exe> dump` byte for byte:
// E5 for these inputs on that platform and identity. CI shards set it and require it (TSG_REQUIRE_FIELD_LOOKUP=1).
// The name starts with TestCorpus so the conformance.yml -run 'TestCorpus|...' filter runs it.
func TestCorpusKorean(t *testing.T) {
	rows := readCSV(t, filepath.Join(koreanDir, "manifest.csv"))
	if err := koreanManifestGrammars(rows, readFile(t, "../gen/pins.json")); err != nil {
		t.Fatal(err)
	}
	if err := koreanManifestGaps(rows, loadKnownGaps(t).Entries); err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, r := range rows {
		listed[filepath.ToSlash(r[2])] = true
	}
	files, err := filepath.Glob(filepath.Join(koreanDir, "*", "*.in"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if rel, _ := filepath.Rel(koreanDir, f); !listed[filepath.ToSlash(rel)] {
			t.Errorf("%s is not in the manifest", f)
		}
	}
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			p := newParser(t, lang)
			var clean, gaps int
			for _, r := range rows {
				if r[1] != lang {
					continue
				}
				id, file, wantSHA, class, expect, dumpSHA := r[0], r[2], r[3], r[6], r[7], r[8]
				src := readFile(t, filepath.Join(koreanDir, file))
				if sha(src) != wantSHA || strconv.Itoa(len(src)) != r[4] {
					t.Errorf("%s: input differs from the manifest (sha256 %s)", id, sha(src))
					continue
				}
				s, err := p.ParseSnapshot(bg, src)
				if err != nil {
					t.Errorf("%s: %v", id, err)
					continue
				}
				got := Dump(s)
				switch class {
				case "clean":
					clean++
					if bad := koreanBadNode(s); bad != "" {
						t.Errorf("%s: clean input has %s", id, bad)
					}
					if expect == "" {
						t.Errorf("%s: clean input without expectations", id)
					}
					for _, msg := range koreanCheck(s, src, expect) {
						t.Errorf("%s: %s", id, msg)
					}
				case "known-gap":
					gaps++
					if !s.Root().HasError() || sha(got) != dumpSHA {
						t.Errorf("%s: known-gap tree changed (root error %v, dump sha256 %s, recorded %s): investigate; a fixed gap moves to clean",
							id, s.Root().HasError(), sha(got), dumpSHA)
					}
				default:
					t.Errorf("%s: unknown class %q", id, class)
				}
				if t.Failed() {
					preserve("korean/"+id+".go.dump", got)
				}
				koreanNative(t, lang, id, filepath.Join(koreanDir, file), got)
			}
			if clean == 0 {
				t.Errorf("%s: no clean Korean rows in the manifest", lang)
			}
			summary(t, map[string]any{"lang": lang, "clean": clean, "knownGap": gaps})
		})
	}
}

func koreanManifestGrammars(rows [][]string, pinJSON []byte) error {
	var pins struct{ Grammars map[string]json.RawMessage }
	if err := json.Unmarshal(pinJSON, &pins); err != nil {
		return err
	}
	known := map[string]bool{}
	for lang := range pins.Grammars {
		if lang == "golang" {
			lang = "go"
		}
		known[lang] = true
	}
	for _, r := range rows {
		if !known[r[1]] {
			return fmt.Errorf("%s: unknown grammar %q", r[0], r[1])
		}
		if dir := filepath.ToSlash(filepath.Dir(r[2])); dir != r[1] {
			return fmt.Errorf("%s: grammar %q does not match input directory %q", r[0], r[1], dir)
		}
	}
	return nil
}

// koreanManifestGaps: a row's gap column is empty or the id of a register entry of the row's grammar, and only
// known-gap rows have one.
func koreanManifestGaps(rows [][]string, entries []knownGap) error {
	for _, r := range rows {
		if r[10] == "" {
			continue
		}
		if r[6] != "known-gap" {
			return fmt.Errorf("%s: gap %q on a %s row", r[0], r[10], r[6])
		}
		i := slices.IndexFunc(entries, func(g knownGap) bool { return g.ID == r[10] })
		if i < 0 || strings.ReplaceAll(entries[i].Grammar, "golang", "go") != r[1] {
			return fmt.Errorf("%s: gap %q is not a register entry of grammar %q", r[0], r[10], r[1])
		}
	}
	return nil
}

func TestCorpusKoreanManifestGaps(t *testing.T) {
	entries := []knownGap{{ID: "css-x", Grammar: "css"}}
	for _, tc := range []struct{ class, grammar, gap, message string }{
		{"known-gap", "css", "", ""},
		{"known-gap", "css", "css-x", ""},
		{"known-gap", "css", "css-y", "not a register entry"},
		{"known-gap", "json", "css-x", "not a register entry"},
		{"clean", "css", "css-x", "on a clean row"},
	} {
		t.Run(tc.class+"/"+tc.grammar+"/"+tc.gap, func(t *testing.T) {
			row := make([]string, 11)
			row[0], row[1], row[6], row[10] = "probe", tc.grammar, tc.class, tc.gap
			err := koreanManifestGaps([][]string{row}, entries)
			if tc.message == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("got %v, want %q", err, tc.message)
			}
		})
	}
}

func TestCorpusKoreanManifestGrammars(t *testing.T) {
	pins := readFile(t, "../gen/pins.json")
	for _, tc := range []struct{ grammar, file, message string }{
		{"css", "css/probe-class-selector.in", ""},
		{"go", "go/id-var-func-type-field.in", ""},
		{"csx", "css/probe-class-selector.in", "unknown grammar"},
		{"csx", "csx/probe-class-selector.in", "unknown grammar"},
		{"json", "css/probe-class-selector.in", "does not match input directory"},
		{"golang", "golang/probe.in", "unknown grammar"},
	} {
		t.Run(tc.grammar+"/"+tc.file, func(t *testing.T) {
			err := koreanManifestGrammars([][]string{{"probe", tc.grammar, tc.file}}, pins)
			if tc.message == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("got %v, want %q", err, tc.message)
			}
		})
	}
}

func TestCorpusKoreanRuns(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want [][2]uint32
	}{
		{"", nil},
		{"abc", nil},
		{"a한글b", [][2]uint32{{1, 7}}},
		{"한글😀끝", [][2]uint32{{0, 6}, {6, 10}, {10, 13}}},
		{"\u1112\u1161\u11ab", [][2]uint32{{0, 9}}},       // NFD conjoining jamo
		{"ㄱ\u3000ㄴ", [][2]uint32{{0, 3}, {3, 6}, {6, 9}}}, // compatibility jamo around an ideographic space
		{"é😀 x", [][2]uint32{{0, 6}}},                     // other code points form one run
		{"a\xffb\xea\xb0", [][2]uint32{{1, 2}, {3, 5}}},   // invalid bytes count as other
	} {
		if got := koreanRuns([]byte(tc.in)); !slices.Equal(got, tc.want) {
			t.Errorf("koreanRuns(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// koreanCheckJSON runs koreanCheck on a JSON input; `["a한글"]` has string_content [2,9) and the Hangul run [3,9).
func koreanCheckJSON(t *testing.T, expect string, want ...string) []string {
	t.Helper()
	if !slices.Contains(Linked(), "json") {
		t.Skip("json is not linked into this binary")
	}
	src := []byte(`["a한글"]`)
	s, err := newParser(t, "json").ParseSnapshot(bg, src)
	if err != nil {
		t.Fatal(err)
	}
	got := koreanCheck(s, src, expect)
	if len(got) != len(want) {
		t.Fatalf("koreanCheck(%q) = %q, want %d messages containing %q", expect, got, len(want), want)
	}
	for i, w := range want {
		if !strings.Contains(got[i], w) {
			t.Errorf("koreanCheck(%q) message %d = %q, want it to contain %q", expect, i, got[i], w)
		}
	}
	return got
}

// A run without an expectation, or an expectation on something that is not a run, fails.
func TestCorpusKoreanCheckRuns(t *testing.T) {
	koreanCheckJSON(t, "3-9:string_content;2-9=string_content")
	// An extent already in the row is not suggested again.
	if got := koreanCheckJSON(t, "2-9=string_content", `non-ASCII run [3,9) "한글" has no expectation: add 3-9:string_content`); len(got) == 1 && strings.Contains(got[0], " and ") {
		t.Errorf("suggested an extent the row already records: %q", got[0])
	}
	koreanCheckJSON(t, "1-10=string", `non-ASCII run [3,9) "한글" has no expectation: add 3-9:string_content and 2-9=string_content`)
	koreanCheckJSON(t, "3-9:string_content;2-9=string_content;3-6:string_content", "3-6:string_content: [3,6) is not a non-ASCII run")
	koreanCheckJSON(t, "3-9:string_content;2-9=string_content;3-9:string_content", "a second expectation for [3,9)")
}

// A covering node wider than its run fails unless its extent is recorded exactly.
func TestCorpusKoreanCheckExtent(t *testing.T) {
	koreanCheckJSON(t, "3-9:string_content", "the string_content node spans [2,9); record its extent: add 2-9=string_content")
	koreanCheckJSON(t, "3-9:string_content;02-09=string_content") // the extent is matched as parsed, not as written
	koreanCheckJSON(t, "3-9:string_content;2-8=string_content",
		"record its extent: add 2-9=string_content", "expected string_content spanning exactly [2,8), found none")
	koreanCheckJSON(t, "3-9:string;2-9=string_content", `expected string at [3,9), found "string_content"`)
}

// koreanBadNode names the first ERROR or MISSING node of s, or "".
func koreanBadNode(s *treesitter.Snapshot) (bad string) {
	s.Walk(func(n treesitter.Node, _ int) treesitter.WalkAction {
		if bad == "" && (n.IsError() || n.IsMissing()) {
			bad = fmt.Sprintf("%s node [%d,%d) (error=%v missing=%v)", n.Kind(), n.StartByte(), n.EndByte(), n.IsError(), n.IsMissing())
		}
		return treesitter.Continue
	})
	if bad == "" && s.Root().HasError() {
		bad = "root has_error"
	}
	return bad
}

// koreanCheck checks the expectations of a clean row (see TestCorpusKorean) and returns one message per violation.
func koreanCheck(s *treesitter.Snapshot, src []byte, expect string) []string {
	type span struct{ start, end uint32 }
	type entry struct {
		raw, kind string
		exact     bool
		sp        span
	}
	type extent struct {
		sp   span
		kind string
	}
	var msgs []string
	var entries []entry
	exact := map[extent]bool{} // parsed "s-e=kind" entries: exact spans, and the recorded extents of wide covering nodes
	recorded := func(n treesitter.Node) bool { return exact[extent{span{n.StartByte(), n.EndByte()}, n.Kind()}] }
	for _, e := range strings.Split(expect, ";") {
		isExact := strings.Contains(e, "=")
		rng, kind, ok := strings.Cut(e, map[bool]string{true: "=", false: ":"}[isExact])
		a, b, ok2 := strings.Cut(rng, "-")
		start, err1 := strconv.ParseUint(a, 10, 32)
		end, err2 := strconv.ParseUint(b, 10, 32)
		if !ok || !ok2 || err1 != nil || err2 != nil || kind == "" {
			msgs = append(msgs, fmt.Sprintf("bad expectation %q", e))
			continue
		}
		entries = append(entries, entry{e, kind, isExact, span{uint32(start), uint32(end)}})
		if isExact {
			exact[extent{span{uint32(start), uint32(end)}, kind}] = true
		}
	}
	colons := map[span]string{} // ":" spans to their raw entry
	var order []span
	for _, x := range entries {
		e, kind, sp := x.raw, x.kind, x.sp
		if x.exact {
			found := false
			s.Walk(func(n treesitter.Node, _ int) treesitter.WalkAction {
				found = found || n.StartByte() == sp.start && n.EndByte() == sp.end && n.Kind() == kind
				return treesitter.Continue
			})
			if !found {
				msgs = append(msgs, fmt.Sprintf("expected %s spanning exactly [%d,%d), found none", kind, sp.start, sp.end))
			}
			continue
		}
		if _, dup := colons[sp]; dup {
			msgs = append(msgs, fmt.Sprintf("%s: a second expectation for [%d,%d)", e, sp.start, sp.end))
			continue
		}
		colons[sp] = e
		order = append(order, sp)
		d, ok := koreanDeepest(s, sp.start, sp.end)
		switch {
		case !ok || d.Kind() != kind:
			found := ""
			if ok {
				found = d.Kind()
			}
			msgs = append(msgs, fmt.Sprintf("expected %s at [%d,%d), found %q", kind, sp.start, sp.end, found))
		case !d.IsNamed():
			msgs = append(msgs, fmt.Sprintf("%s: the covering node %q is anonymous", e, kind))
		case (d.StartByte() != sp.start || d.EndByte() != sp.end) && !recorded(d):
			msgs = append(msgs, fmt.Sprintf("%s: the %s node spans [%d,%d); record its extent: add %s", e, kind, d.StartByte(), d.EndByte(), koreanExtent(d)))
		}
	}
	runs := map[span]bool{}
	for _, r := range koreanRuns(src) {
		sp := span{r[0], r[1]}
		runs[sp] = true
		if _, ok := colons[sp]; ok {
			continue
		}
		hint := ""
		if d, ok := koreanDeepest(s, sp.start, sp.end); ok {
			hint = fmt.Sprintf(": add %d-%d:%s", sp.start, sp.end, d.Kind())
			if (d.StartByte() != sp.start || d.EndByte() != sp.end) && !recorded(d) {
				hint += " and " + koreanExtent(d)
			}
		}
		msgs = append(msgs, fmt.Sprintf("non-ASCII run [%d,%d) %q has no expectation%s", sp.start, sp.end, src[sp.start:sp.end], hint))
	}
	for _, sp := range order {
		if !runs[sp] {
			msgs = append(msgs, fmt.Sprintf("%s: [%d,%d) is not a non-ASCII run of the input", colons[sp], sp.start, sp.end))
		}
	}
	return msgs
}

// koreanDeepest returns the deepest node covering [start,end), the first in preorder at that depth.
func koreanDeepest(s *treesitter.Snapshot, start, end uint32) (d treesitter.Node, ok bool) {
	best := -1
	s.Walk(func(n treesitter.Node, depth int) treesitter.WalkAction {
		if n.StartByte() <= start && n.EndByte() >= end && depth > best {
			best, d, ok = depth, n, true
		}
		return treesitter.Continue
	})
	return d, ok
}

// koreanExtent is the "s-e=kind" entry recording n's extent.
func koreanExtent(n treesitter.Node) string {
	return fmt.Sprintf("%d-%d=%s", n.StartByte(), n.EndByte(), n.Kind())
}

// koreanRuns returns the maximal runs of Hangul code points, and of other non-ASCII code points (an invalid UTF-8
// byte counts as one), as byte ranges [start,end). A change between the two classes starts a new run.
func koreanRuns(src []byte) [][2]uint32 {
	hangul := func(r rune) bool {
		return r >= 0xAC00 && r <= 0xD7A3 || r >= 0x1100 && r <= 0x11FF || r >= 0x3130 && r <= 0x318F ||
			r >= 0xA960 && r <= 0xA97F || r >= 0xD7B0 && r <= 0xD7FF || r >= 0xFFA0 && r <= 0xFFDC
	}
	var runs [][2]uint32
	prev := 0 // class of the previous code point: 0 ASCII, 1 Hangul, 2 other
	for i := 0; i < len(src); {
		r, n := utf8.DecodeRune(src[i:])
		class := 0
		if r >= 0x80 {
			class = 2
			if hangul(r) {
				class = 1
			}
		}
		if class != 0 && class == prev {
			runs[len(runs)-1][1] += uint32(n)
		} else if class != 0 {
			runs = append(runs, [2]uint32{uint32(i), uint32(i + n)})
		}
		prev = class
		i += n
	}
	return runs
}

// koreanNative compares the product dump with the native executable's, when there is one (required in CI).
func koreanNative(t *testing.T, lang, id, input string, got []byte) {
	t.Helper()
	if os.Getenv("TSG_NATIVE_EXE_DIR") == "" {
		if strictKnownGaps() {
			t.Errorf("%s: TSG_NATIVE_EXE_DIR is required under TSG_REQUIRE_FIELD_LOOKUP=1", id)
		}
		return
	}
	exe, ok := nativeExe(lang)
	if !ok {
		t.Errorf("%s: no native executable for %s", id, lang)
		return
	}
	ctx, cancel := context.WithTimeout(bg, 30*time.Second)
	defer cancel()
	cmd := proc.CommandContext(ctx, exe, "dump", input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	native, err := cmd.Output()
	if err != nil {
		t.Errorf("%s: native dump: %v: %s", id, err, &stderr)
		return
	}
	if !bytes.Equal(got, native) {
		preserve("korean/"+id+".native.dump", native)
		t.Errorf("%s: engine dump differs from native (%s/%s): %s", id, runtime.GOOS, runtime.GOARCH, firstDiff(got, native))
	}
}
