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

	proc "github.com/wotjr1649/go-treesitter/tools/bench/proc"

	treesitter "github.com/wotjr1649/go-treesitter"
)

const koreanDir = "testdata/korean"

// TestCorpusKorean: the retained Korean (Hangul) inputs of testdata/korean, per grammar, through the product.
//
//   - class clean: no ERROR/MISSING node, and every manifest expectation holds. "s-e:kind" = the deepest node covering the
//     Hangul run [s,e) has that kind; "s-e=kind" = a node of that kind spans exactly [s,e) (heredoc delimiters, so an end
//     position that is wrong cannot pass).
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
					for _, e := range strings.Split(expect, ";") {
						if msg := koreanExpect(s, e); msg != "" {
							t.Errorf("%s: %s", id, msg)
						}
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

// koreanExpect checks one "s-e:kind" or "s-e=kind" expectation; "" when it holds.
func koreanExpect(s *treesitter.Snapshot, e string) string {
	exact := strings.Contains(e, "=")
	span, kind, ok := strings.Cut(e, map[bool]string{true: "=", false: ":"}[exact])
	a, b, ok2 := strings.Cut(span, "-")
	start, err1 := strconv.ParseUint(a, 10, 32)
	end, err2 := strconv.ParseUint(b, 10, 32)
	if !ok || !ok2 || err1 != nil || err2 != nil {
		return fmt.Sprintf("bad expectation %q", e)
	}
	var found string // kind of the exact-span node, or of the deepest covering node
	deepest := -1
	s.Walk(func(n treesitter.Node, depth int) treesitter.WalkAction {
		switch {
		case exact && uint64(n.StartByte()) == start && uint64(n.EndByte()) == end && n.Kind() == kind:
			found = kind
		case !exact && uint64(n.StartByte()) <= start && uint64(n.EndByte()) >= end && depth > deepest:
			deepest, found = depth, n.Kind()
		}
		return treesitter.Continue
	})
	if found != kind {
		return fmt.Sprintf("expected %s at [%d,%d), found %q", kind, start, end, found)
	}
	return ""
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
