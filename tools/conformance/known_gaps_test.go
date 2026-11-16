package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

//go:generate go test -run ^TestKnownGapDocs$ -args -update-known-gap-docs

var updateKnownGapDocs = flag.Bool("update-known-gap-docs", false, "regenerate docs/known-gaps from the defect register")

const knownGapDir = "testdata/known-gaps"

type knownGapRegister struct {
	Schema           int
	EvidenceBase     string
	EvidencePlatform string
	Runtime          string
	Grammars         map[string]knownGapIdentity
	Entries          []knownGap
}

type knownGapIdentity struct {
	Name             string // Display name in generated documents.
	Commit           string
	PatchCount       int
	PinSHA256        string // Canonical JSON [runtime pin, wasi-libc pin, grammar pin], including patches.
	ProvenanceSHA256 string
}

type knownGap struct {
	ID, Grammar, Input, InputSHA256, DumpSHA256 string
	Observed, Expected                          string
	Specs                                       []struct{ Title, URL string }
	Upstream                                    string
}

func loadKnownGaps(t *testing.T) knownGapRegister {
	t.Helper()
	var r knownGapRegister
	dec := json.NewDecoder(bytes.NewReader(readFile(t, filepath.Join(knownGapDir, "register.json"))))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		t.Fatal(err)
	}
	if r.Schema != 1 || len(r.Entries) == 0 || r.EvidenceBase == "" || r.EvidencePlatform == "" {
		t.Fatal("incomplete known-gap register")
	}
	for lang, id := range r.Grammars {
		if id.Name == "" || !slices.ContainsFunc(r.Entries, func(g knownGap) bool { return g.Grammar == lang }) {
			t.Fatalf("%s: grammar without display name or entries", lang)
		}
	}
	seen := map[string]bool{}
	for _, g := range r.Entries {
		_, hasIdentity := r.Grammars[g.Grammar]
		if !hasIdentity || g.ID == "" || filepath.Base(g.ID) != g.ID || seen[g.ID] || g.Input == "" ||
			g.Observed == "" || g.Expected == "" || len(g.Specs) == 0 {
			t.Fatalf("incomplete or duplicate known gap: %q", g.ID)
		}
		seen[g.ID] = true
		for _, spec := range g.Specs {
			if spec.Title == "" || !strings.HasPrefix(spec.URL, "https://") {
				t.Fatalf("%s: missing language-spec citation", g.ID)
			}
		}
	}
	return r
}

// verifyKnownGaps checks source/module identity and fixture hashes of the given grammars only, so an identity move
// of one grammar fails that grammar's shard and the documents test, not every shard.
func verifyKnownGaps(t *testing.T, r knownGapRegister, grammars []string) {
	t.Helper()
	var pins struct {
		Runtime, WasiLibc map[string]any
		Grammars          map[string]map[string]any
	}
	if err := json.Unmarshal(readFile(t, "../gen/pins.json"), &pins); err != nil {
		t.Fatal(err)
	}
	if pins.Runtime["release"] != r.Runtime {
		t.Fatal("known-gap runtime identity drift")
	}
	for _, lang := range grammars {
		id := r.Grammars[lang]
		pin := pins.Grammars[lang]
		patches, _ := pin["patches"].([]any)
		identity, err := json.Marshal([]any{pins.Runtime, pins.WasiLibc, pin})
		if err != nil {
			t.Fatal(err)
		}
		provenance := sha(readFile(t, filepath.Join("../../grammars", lang, "provenance.json")))
		if pin["commit"] != id.Commit || len(patches) != id.PatchCount || sha(identity) != id.PinSHA256 || provenance != id.ProvenanceSHA256 {
			t.Fatalf("%s: known-gap source/module identity drift (pin sha256 %s, provenance sha256 %s); reproduce every entry on the engine and native C, then update this grammar's register identity",
				lang, sha(identity), provenance)
		}
		for _, g := range r.Entries {
			if g.Grammar == lang && (sha([]byte(g.Input)) != g.InputSHA256 || sha(knownGapDump(t, g)) != g.DumpSHA256) {
				t.Fatalf("%s: fixture identity drift", g.ID)
			}
		}
	}
}

// knownGapShard returns the register grammars this run covers: those named by TSG_S15_LANGS (default all linked).
// Under TSG_REQUIRE_FIELD_LOOKUP=1 (CI) a named grammar that is not linked fails instead of silently running nothing.
func knownGapShard(t *testing.T, r knownGapRegister) []string {
	t.Helper()
	linked := Linked()
	requested := linked
	if v := os.Getenv("TSG_S15_LANGS"); v != "" {
		requested = strings.Split(v, ",")
	}
	var sel []string
	for _, lang := range requested {
		if !slices.Contains(linked, lang) && strictKnownGaps() {
			t.Fatalf("selected grammar %q is not linked into this binary (a renamed shard would silently run nothing)", lang)
		}
		if _, ok := r.Grammars[lang]; ok && slices.Contains(linked, lang) {
			sel = append(sel, lang)
		}
	}
	return sel
}

func strictKnownGaps() bool { return os.Getenv("TSG_REQUIRE_FIELD_LOOKUP") == "1" }

func knownGapDump(t *testing.T, g knownGap) []byte {
	t.Helper()
	return readFile(t, filepath.Join(knownGapDir, g.ID+".dump"))
}

// Any change fails: fixing the defect must retire the entry, and worsening it cannot widen the waiver.
func checkKnownGap(got, observed []byte) error {
	if !bytes.Equal(got, observed) {
		return fmt.Errorf("STALE/CHANGED known defect: %s; investigate, then remove a fixed entry or convert it to a regression test", firstDiff(got, observed))
	}
	return nil
}

func TestKnownGaps(t *testing.T) {
	r := loadKnownGaps(t)
	selected := knownGapShard(t, r)
	if len(selected) == 0 {
		t.Skip("no known-gap grammar in this shard")
	}
	verifyKnownGaps(t, r, selected)
	ran := map[string]int{}
	defer func() {
		for _, lang := range selected {
			if ran[lang] == 0 {
				t.Errorf("%s: shard grammar has register entries but none ran", lang)
			}
		}
	}()
	for _, g := range r.Entries {
		if !slices.Contains(selected, g.Grammar) {
			continue
		}
		ran[g.Grammar]++
		t.Run(g.ID, func(t *testing.T) {
			want := knownGapDump(t, g)
			p := newParser(t, g.Grammar)
			s, err := p.ParseSnapshot(bg, []byte(g.Input))
			if err != nil {
				t.Fatal(err)
			}
			got := Dump(s)
			if err := checkKnownGap(got, want); err != nil {
				preserve("known-gaps/"+g.ID+".go.dump", got)
				t.Fatal(err)
			}
			t.Run("native", func(t *testing.T) {
				if os.Getenv("TSG_NATIVE_EXE_DIR") == "" {
					if strictKnownGaps() {
						t.Fatal("TSG_NATIVE_EXE_DIR is required under TSG_REQUIRE_FIELD_LOOKUP=1 (E5 needs a freshly built native executable)")
					}
					t.Skip("E3 only; set TSG_NATIVE_EXE_DIR to freshly built tools/native-ref executables for E5")
				}
				exe, ok := nativeExe(g.Grammar)
				if !ok {
					t.Fatal("no native executable for " + g.Grammar)
				}
				input := filepath.Join(t.TempDir(), g.ID+".in")
				if err := os.WriteFile(input, []byte(g.Input), 0644); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(bg, 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, exe, "dump", input)
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				native, err := cmd.Output()
				if err != nil {
					t.Fatalf("native dump: %v: %s", err, &stderr)
				}
				if err := checkKnownGap(native, want); err != nil || !bytes.Equal(got, native) {
					preserve("known-gaps/"+g.ID+".native.dump", native)
					t.Fatalf("native signature or engine/native comparison failed: %v; %s", err, firstDiff(got, native))
				}
				summary(t, map[string]any{"level": "E5", "id": g.ID, "inputSHA256": g.InputSHA256,
					"dumpSHA256": sha(native), "nativeSHA256": sha(readFile(t, exe)),
					"pinSHA256": r.Grammars[g.Grammar].PinSHA256, "provenanceSHA256": r.Grammars[g.Grammar].ProvenanceSHA256,
					"platform": runtime.GOOS + "/" + runtime.GOARCH})
			})
		})
	}
}

func TestKnownGapRatchet(t *testing.T) {
	r := loadKnownGaps(t)
	for _, g := range r.Entries {
		if err := checkKnownGap(knownGapDump(t, g), knownGapDump(t, g)); err != nil {
			t.Fatal(err)
		}
	}
	// Simulate the fixed Python precedence for the same input, without editing the grammar or golden dump.
	fixed := []byte("#tsg-dump/1 root_has_error=0 nodes=8\n" +
		"0\t-\tmodule\t1\t0\t0\t0\t0\t0\t9\t0:0\t0:9\n" +
		"1\t-\tbinary_operator\t1\t0\t0\t0\t0\t0\t9\t0:0\t0:9\n" +
		"2\tleft\tidentifier\t1\t0\t0\t0\t0\t0\t1\t0:0\t0:1\n" +
		"2\toperator\t^\t0\t0\t0\t0\t0\t2\t3\t0:2\t0:3\n" +
		"2\tright\tbinary_operator\t1\t0\t0\t0\t0\t4\t9\t0:4\t0:9\n" +
		"3\tleft\tidentifier\t1\t0\t0\t0\t0\t4\t5\t0:4\t0:5\n" +
		"3\toperator\t&\t0\t0\t0\t0\t0\t6\t7\t0:6\t0:7\n" +
		"3\tright\tidentifier\t1\t0\t0\t0\t0\t8\t9\t0:8\t0:9\n")
	var observed []byte
	for _, g := range r.Entries {
		if g.ID == "python-bitwise-precedence" {
			observed = knownGapDump(t, g)
		}
	}
	if observed == nil {
		t.Fatal("retire this flip check with its fixed register entry")
	}
	for name, changed := range map[string][]byte{"fixed": fixed, "worsened": []byte("#tsg-dump/1 root_has_error=1 nodes=0\n")} {
		t.Run(name, func(t *testing.T) {
			if err := checkKnownGap(changed, observed); err == nil || !strings.Contains(err.Error(), "STALE/CHANGED") {
				t.Fatalf("ratchet accepted %s shape: %v", name, err)
			}
		})
	}
}

func knownGapDocs(r knownGapRegister) map[string][]byte {
	docs := map[string][]byte{}
	for lang, id := range r.Grammars {
		var b strings.Builder
		fmt.Fprintf(&b, "<!-- Generated from tools/conformance/testdata/known-gaps/register.json; do not edit. -->\n\n# %s known grammar gaps\n\n", id.Name)
		b.WriteString("These upstream grammar defects are reproduced by the engine. The retained strict expected-failure test\npasses only while the entire recorded wrong tree remains unchanged; a fix or any other shape change fails it.\nRemove fixed entries or turn them into regression tests. This is a bounded defect list, not a language validator.\n\n")
		fmt.Fprintf(&b, "Initial E5 evidence: Windows native reference differential (%s), base commit `%s`, with this register applied on top of it.\nOnly the exact inputs below are covered. `TestKnownGaps` rechecks engine dumps (E3) and, with a freshly built\n`TSG_NATIVE_EXE_DIR`, compares them byte-for-byte with native C (E5). CI requires that comparison.\n\n", r.EvidencePlatform, r.EvidenceBase)
		fmt.Fprintf(&b, "Identity: grammar `%s`; runtime `%s`; grammar patches: %d.\nThe source identity hashes canonical JSON `[runtime, wasiLibc, grammar]` from [`pins.json`](../../tools/gen/pins.json),\nincluding source hashes and patch records. The module identity hashes its [`provenance.json`](../../grammars/%s/provenance.json).\n\n- Source identity SHA-256: `%s`\n- Module provenance SHA-256: `%s`\n\n", id.Commit, r.Runtime, id.PatchCount, lang, id.PinSHA256, id.ProvenanceSHA256)
		for _, g := range r.Entries {
			if g.Grammar != lang {
				continue
			}
			fmt.Fprintf(&b, "## %s\n\nInput (JSON string; escapes and final newline are significant):\n\n%s\n\nObserved: %s\n\nExpected rule: %s\n\n", g.ID, jsonFence(g.Input), g.Observed, g.Expected)
			for _, spec := range g.Specs {
				fmt.Fprintf(&b, "- Specification: [%s](%s)\n", spec.Title, spec.URL)
			}
			if g.Upstream == "" {
				b.WriteString("- Upstream issue: none recorded.\n")
			} else {
				fmt.Fprintf(&b, "- [Upstream issue](%s)\n", g.Upstream)
			}
			fmt.Fprintf(&b, "- Input SHA-256: `%s`\n- [Recorded native dump](../../tools/conformance/testdata/known-gaps/%s.dump), SHA-256: `%s`\n\n", g.InputSHA256, g.ID, g.DumpSHA256)
		}
		docs[lang+".md"] = []byte(strings.TrimRight(b.String(), "\n") + "\n")
	}
	return docs
}

// jsonFence renders s as a JSON string in a code fence, without HTML escaping and fenced beyond any backtick run in s.
func jsonFence(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil { // A string is always JSON-encodable.
		panic(err)
	}
	fence, run := "```", 0
	for _, c := range s + "x" {
		if c == '`' {
			run++
			continue
		}
		if run >= len(fence) {
			fence = strings.Repeat("`", run+1)
		}
		run = 0
	}
	return fence + "json\n" + strings.TrimSuffix(b.String(), "\n") + "\n" + fence
}

var trailingSpace = regexp.MustCompile(`[ \t]\n`)

func TestKnownGapFence(t *testing.T) {
	if got, want := jsonFence("a ^ b & c <T>\n"), "```json\n\"a ^ b & c <T>\\n\"\n```"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got, want := jsonFence("x ``` y"), "````json\n\"x ``` y\"\n````"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestKnownGapDocs(t *testing.T) {
	r := loadKnownGaps(t)
	verifyKnownGaps(t, r, slices.Sorted(maps.Keys(r.Grammars)))
	docs := knownGapDocs(r)
	dir := "../../docs/known-gaps"
	if *updateKnownGapDocs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for name, generated := range docs {
		path := filepath.Join(dir, name)
		if bytes.HasSuffix(generated, []byte("\n\n")) || !bytes.HasSuffix(generated, []byte("\n")) || trailingSpace.Match(generated) {
			t.Errorf("%s: generated text must end in one newline and have no trailing whitespace (git diff --check)", name)
		}
		if *updateKnownGapDocs {
			if err := os.WriteFile(path, generated, 0644); err != nil {
				t.Fatal(err)
			}
		} else if !bytes.Equal(readFile(t, path), generated) {
			t.Errorf("%s is stale; run go generate ./... in tools/conformance", path)
		}
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if _, ok := docs[filepath.Base(path)]; !ok {
			t.Errorf("remove obsolete generated document %s", path)
		}
	}
}

var taskCode = regexp.MustCompile(`\b[PQ][0-9]{1,2}\b`)

// Owner decision D8: user documents cite dates, evidence paths or commits, never internal task codes (ADRs, plans and
// prompts are exempt). Named TestKnownGap* so the existing conformance.yml -run filter runs it.
func TestKnownGapDocsNoTaskCodes(t *testing.T) {
	root := "../.."
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch {
		case d.IsDir():
			if rel != "." && strings.HasPrefix(rel, ".") || rel == "docs/design/decisions" || rel == "docs/plans" || rel == "docs/prompts" {
				return filepath.SkipDir
			}
			return nil
		case rel == "README.md" || rel == "MIGRATING.md" || rel == "SECURITY.md",
			strings.HasPrefix(rel, "docs/") && strings.HasSuffix(rel, ".md"),
			strings.HasPrefix(rel, "grammars/") && strings.HasSuffix(rel, "/README.md"),
			strings.HasPrefix(rel, "tools/") && strings.HasSuffix(rel, "/README.md"):
		default:
			return nil
		}
		for i, line := range strings.Split(string(readFile(t, path)), "\n") {
			if m := taskCode.FindString(line); m != "" {
				t.Errorf("%s:%d: internal task code %q; cite a date, evidence path or commit instead", rel, i+1, m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
