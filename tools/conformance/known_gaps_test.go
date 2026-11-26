package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	proc "github.com/wotjr1649/go-treesitter/tools/bench/proc"
)

//go:generate go test -run ^TestKnownGapDocs$ -args -update-known-gap-docs

var updateKnownGapDocs = flag.Bool("update-known-gap-docs", false, "regenerate docs/known-gaps from the defect register")
var updateKnownGapProvenance = flag.Bool("update-known-gap-provenance", false, "rewrite the register's ProvenanceSHA256 values from grammars/<lang>/provenance.json (never EvidenceBase or EvidencePlatform)")

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
	Upstream, UpstreamNote                      string
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

type pinsFile struct {
	Runtime, WasiLibc map[string]any
	Grammars          map[string]map[string]any
}

func loadPins(t *testing.T) pinsFile {
	t.Helper()
	var pins pinsFile
	if err := json.Unmarshal(readFile(t, "../gen/pins.json"), &pins); err != nil {
		t.Fatal(err)
	}
	return pins
}

func runtimePin(t *testing.T) map[string]any { t.Helper(); return loadPins(t).Runtime }

// currentPinIdentity is the source identity of one grammar as the register records it.
func currentPinIdentity(t *testing.T, lang string) (commit string, patchCount int, pinSHA string) {
	t.Helper()
	pins := loadPins(t)
	pin := pins.Grammars[lang]
	patches, _ := pin["patches"].([]any)
	identity, err := json.Marshal([]any{pins.Runtime, pins.WasiLibc, pin})
	if err != nil {
		t.Fatal(err)
	}
	commit, _ = pin["commit"].(string)
	return commit, len(patches), sha(identity)
}

// rewriteKnownGapProvenance replaces each listed grammar's ProvenanceSHA256 in register.json text and nothing else:
// EvidenceBase and EvidencePlatform record where the native evidence was gathered and move only by decision.
var sha256Hex = regexp.MustCompile("^[0-9a-f]{64}$")

func rewriteKnownGapProvenance(raw []byte, want map[string]string) ([]byte, error) {
	out := raw
	for _, lang := range slices.Sorted(maps.Keys(want)) {
		if !sha256Hex.MatchString(want[lang]) {
			return nil, fmt.Errorf("%s: %q is not a sha256", lang, want[lang])
		}
		re := regexp.MustCompile(`("` + regexp.QuoteMeta(lang) + `": \{[^{}]*?"ProvenanceSHA256": ")[0-9a-f]{64}(")`)
		if len(re.FindAllIndex(out, -1)) != 1 {
			return nil, fmt.Errorf("%s: expected exactly one ProvenanceSHA256 in its Grammars entry", lang)
		}
		out = re.ReplaceAll(out, []byte("${1}"+want[lang]+"${2}"))
	}
	var before, after knownGapRegister
	if err := errors.Join(json.Unmarshal(raw, &before), json.Unmarshal(out, &after)); err != nil {
		return nil, err
	}
	for _, r := range []*knownGapRegister{&before, &after} { // everything but the rewritten values must be equal
		for lang, id := range r.Grammars {
			id.ProvenanceSHA256 = ""
			r.Grammars[lang] = id
		}
	}
	if !reflect.DeepEqual(before, after) {
		return nil, fmt.Errorf("rewrite changed more than ProvenanceSHA256")
	}
	return out, nil
}

// verifyKnownGaps checks source/module identity and fixture hashes of the given grammars only, so an identity move
// of one grammar fails that grammar's shard and the documents test, not every shard.
func verifyKnownGaps(t *testing.T, r knownGapRegister, grammars []string) {
	t.Helper()
	if runtimePin(t)["release"] != r.Runtime {
		t.Fatal("known-gap runtime identity drift")
	}
	for _, lang := range grammars {
		id := r.Grammars[lang]
		commit, patchCount, pinSHA := currentPinIdentity(t, lang)
		provenance := sha(readFile(t, filepath.Join("../../grammars", lang, "provenance.json")))
		if commit != id.Commit || patchCount != id.PatchCount || pinSHA != id.PinSHA256 || provenance != id.ProvenanceSHA256 {
			t.Fatalf("%s: known-gap source/module identity drift (pin sha256 %s, provenance sha256 %s); reproduce every entry on the engine and native C, then update this grammar's register identity (the provenance value alone: go test -run ^TestKnownGapProvenanceUpdate$ -args -update-known-gap-provenance)",
				lang, pinSHA, provenance)
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
				cmd := proc.CommandContext(ctx, exe, "dump", input)
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
			} else if strings.Contains(g.Upstream, "/pull/") {
				fmt.Fprintf(&b, "- [Upstream pull request](%s)\n", g.Upstream)
			} else {
				fmt.Fprintf(&b, "- [Upstream issue](%s)\n", g.Upstream)
			}
			if g.UpstreamNote != "" {
				fmt.Fprintf(&b, "- %s\n", g.UpstreamNote)
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

func TestKnownGapProvenanceRewrite(t *testing.T) {
	raw := readFile(t, filepath.Join(knownGapDir, "register.json"))
	r := loadKnownGaps(t)
	same := map[string]string{}
	for lang, id := range r.Grammars {
		same[lang] = id.ProvenanceSHA256
	}
	if got, err := rewriteKnownGapProvenance(raw, same); err != nil || !bytes.Equal(got, raw) {
		t.Fatalf("rewriting a register with its own values must be a no-op: %v", err)
	}
	fresh := strings.Repeat("ab", 32)
	changed, err := rewriteKnownGapProvenance(raw, map[string]string{"swift": fresh})
	if err != nil {
		t.Fatal(err)
	}
	var after knownGapRegister
	if err := json.Unmarshal(changed, &after); err != nil {
		t.Fatal(err)
	}
	if after.Grammars["swift"].ProvenanceSHA256 != fresh || after.Grammars["python"] != r.Grammars["python"] ||
		after.EvidenceBase != r.EvidenceBase || after.EvidencePlatform != r.EvidencePlatform {
		t.Fatal("rewrite touched more than the requested provenance")
	}
	for name, want := range map[string]map[string]string{"unknown grammar": {"nope": fresh}, "not a sha256": {"swift": "xyz"}} {
		if _, err := rewriteKnownGapProvenance(raw, want); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// TestKnownGapProvenanceUpdate is the -update-known-gap-provenance tool. It runs only with the flag, rewrites
// register.json for the grammars whose source identity (commit, patches, pins hash) still matches, and refuses any other
// drift: a grammar pin move needs the evidence reproduced by hand. It does not reproduce anything itself, so run the
// engine and native E5 checks afterwards (TSG_NATIVE_EXE_DIR, TSG_REQUIRE_FIELD_LOOKUP=1), then go generate.
func TestKnownGapProvenanceUpdate(t *testing.T) {
	if !*updateKnownGapProvenance {
		t.Skip("set -update-known-gap-provenance to rewrite the register's provenance hashes")
	}
	r := loadKnownGaps(t)
	if runtimePin(t)["release"] != r.Runtime {
		t.Fatal("known-gap runtime identity drift: not a provenance-only change")
	}
	want := map[string]string{}
	for lang, id := range r.Grammars {
		if commit, patchCount, pinSHA := currentPinIdentity(t, lang); commit != id.Commit || patchCount != id.PatchCount || pinSHA != id.PinSHA256 {
			t.Fatalf("%s: source identity drift (pin sha256 %s); not a provenance-only change", lang, pinSHA)
		}
		want[lang] = sha(readFile(t, filepath.Join("../../grammars", lang, "provenance.json")))
	}
	path := filepath.Join(knownGapDir, "register.json")
	out, err := rewriteKnownGapProvenance(readFile(t, path), want)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		t.Fatal(err)
	}
	t.Log("register provenance updated; EvidenceBase and EvidencePlatform are unchanged. Next: native E5 for these grammars, then go generate")
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

// The support table is maintained by hand. Only its development links track the live register, and each must sit in a table row.
func TestKnownGapDocsSupportLinks(t *testing.T) {
	want := map[string]bool{}
	for _, g := range loadKnownGaps(t).Entries {
		want[fmt.Sprintf("../known-gaps/%s.md#%s", g.Grammar, g.ID)] = true
	}
	path := "../../docs/reports/grammar-support.md"
	link := regexp.MustCompile(`\]\((\.\./known-gaps/[^#)]+\.md#[^)]+)\)`)
	for _, line := range strings.Split(string(readFile(t, path)), "\n") {
		if !strings.HasPrefix(line, "|") {
			continue
		}
		for _, match := range link.FindAllStringSubmatch(line, -1) {
			if !want[match[1]] {
				t.Errorf("%s: stale or duplicate register link %s", path, match[1])
			}
			delete(want, match[1])
		}
	}
	for _, target := range slices.Sorted(maps.Keys(want)) {
		t.Errorf("%s: missing register link %s", path, target)
	}
}

var taskCode = regexp.MustCompile(`\b[PQ][0-9]{1,2}\b`)

// A tracked file may name a scratch path only where it is a rule (the files below), a frozen record, or a line that also
// says the evidence moved to a "maintainer-local" archive. This file is listed because it holds the needle.
func TestKnownGapDocsNoDeadScratchRefs(t *testing.T) {
	out, err := proc.Command("git", "-C", "../..", "ls-files").Output()
	if err != nil {
		t.Skipf("git ls-files: %v", err)
	}
	for _, rel := range strings.Fields(string(out)) { // no tracked path has a space
		switch {
		case rel == ".gitignore", rel == "AGENTS.md", rel == "docs/specs/validation.md",
			rel == "tools/conformance/known_gaps_test.go", strings.HasPrefix(rel, "artifacts/release-candidate/"):
			continue
		}
		for i, line := range strings.Split(string(readFile(t, filepath.Join("../..", rel))), "\n") {
			if strings.Contains(line, ".scratch/") && !strings.Contains(line, "maintainer-local") {
				t.Errorf("%s:%d: .scratch path without a maintainer-local archive name", rel, i+1)
			}
		}
	}
}

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
