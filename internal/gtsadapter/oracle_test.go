package gtsadapter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/go-treesitter/internal/provenance"
	gts "github.com/wotjr1649/go-treesitter/internal/runtime"
	"github.com/wotjr1649/go-treesitter/internal/runtime/grammars"
	"github.com/wotjr1649/go-treesitter/syntax"
)

type oracleCase struct {
	ID, Language, Filename, Group, SHA256 string
	Source, Path                          *string
	Previous                              string
	Edit                                  *syntax.Edit
}

type oracleRecord struct {
	Schema            int
	Fixture, Language string
	ABI               int
	InputBytes        int    `json:"input_bytes"`
	SourceSHA256      string `json:"source_sha256"`
	BuildSHA256       string `json:"build_sha256"`
	NodesSHA256       string `json:"nodes_sha256"`
	HasError          bool   `json:"has_error"`
	Start, End        uint32
	Nodes             []syntax.Node
}

type oracleABI struct {
	Min    int    `json:"runtime_abi_min"`
	Max    int    `json:"runtime_abi_max"`
	Header string `json:"runtime_header_sha256"`
}

func checkOracleABI(abi int, artifact, anchor oracleABI) error {
	if artifact != anchor || abi < anchor.Min || abi > anchor.Max {
		return fmt.Errorf("oracle runtime ABI identity mismatch")
	}
	return nil
}

func nodeDigest(nodes []syntax.Node) string {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(nodes); err != nil {
		panic(err) // Only fixed scalar fields are encoded.
	}
	return fmt.Sprintf("%x", sha256.Sum256(bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'})))
}

func logNodeDifference(t *testing.T, label string, left, right []syntax.Node) int {
	t.Helper()
	first := -1
	for i := 0; i < min(len(left), len(right)); i++ {
		if left[i] != right[i] {
			first = i
			break
		}
	}
	if first < 0 && len(left) != len(right) {
		first = min(len(left), len(right))
	}
	t.Logf("%s left_len=%d right_len=%d left_digest=%s right_digest=%s first=%d", label, len(left), len(right), nodeDigest(left), nodeDigest(right), first)
	if first >= 0 {
		if first < len(left) {
			t.Logf("left[%d]=%+v", first, left[first])
		}
		if first < len(right) {
			t.Logf("right[%d]=%+v", first, right[first])
		}
	}
	return first
}

func validateOracle(c oracleCase, source []byte, r oracleRecord, buildHash string, abi int) error {
	if r.Schema != 1 || r.Fixture != c.ID || r.Language != c.Language || r.ABI != abi ||
		r.SourceSHA256 != c.SHA256 || fmt.Sprintf("%x", sha256.Sum256(source)) != c.SHA256 ||
		r.BuildSHA256 != buildHash || r.InputBytes != len(source) {
		return fmt.Errorf("oracle identity mismatch for %s", c.ID)
	}
	if r.Start != 0 || uint64(r.End) != uint64(len(source)) || len(r.Nodes) == 0 {
		return fmt.Errorf("incomplete C receipt for %s", c.ID)
	}
	for i, n := range r.Nodes {
		if n.StartByte > n.EndByte || n.EndByte > r.End || n.Type == "" ||
			(i == 0 && n.Parent != -1) || (i > 0 && (n.Parent < 0 || n.Parent >= i)) {
			return fmt.Errorf("invalid ordered C node for %s at %d", c.ID, i)
		}
	}
	if r.NodesSHA256 != nodeDigest(r.Nodes) {
		return fmt.Errorf("C node digest mismatch for %s", c.ID)
	}
	return nil
}

func TestOracleRecords(t *testing.T) {
	runOracleRecords(t)
}

func TestExtendedOracleRecords(t *testing.T) {
	runOracleCorpus(t, "testdata/oracle/extended-cases.json", "testdata/oracle/windows-c-v2/extended", "")
}

func TestTypeScriptContextualOracleRecords(t *testing.T) {
	runOracleCorpus(t, "testdata/oracle/typescript-contextual-cases.json", "testdata/oracle/windows-c-v2/ts-contextual", "")
}

func TestCSharpRecoveryVariantOracleRecords(t *testing.T) {
	runOracleCorpus(t, "testdata/oracle/csharp-recovery-cases.json", "testdata/oracle/windows-c-v2/cs-recovery", "testdata/oracle/csharp-recovery-differences.json")
}

func runOracleRecords(t *testing.T) {
	runOracleCorpus(t, "testdata/oracle/cases.json", "testdata/oracle/windows-c-v2/base", "testdata/oracle/active-differences.json")
}

func runOracleCorpus(t *testing.T, casesPath, recordsDir, differencesPath string) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	read := func(path string, target any) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, target); err != nil {
			t.Fatal(err)
		}
		return data
	}
	pins, err := provenance.Read()
	if err != nil {
		t.Fatal(err)
	}
	var patchInputs struct {
		Commit string `json:"upstream_patch_commit"`
		SHA256 string `json:"upstream_patch_sha256"`
	}
	patchJSON := read("testdata/oracle/typescript-patched/manifest.json", &patchInputs)
	if pins.Oracle.TypeScriptPatch == nil ||
		fmt.Sprintf("%x", sha256.Sum256(patchJSON)) != pins.Oracle.TypeScriptPatch.InputsSHA256 ||
		patchInputs.Commit != pins.Oracle.TypeScriptPatch.Commit || patchInputs.SHA256 != pins.Oracle.TypeScriptPatch.SHA256 {
		t.Fatal("TypeScript oracle patch identity mismatch")
	}
	var build struct {
		Schema   int
		Pins     provenance.Identities
		OS       string
		Grammars map[string]struct {
			ABI int
			oracleABI
		}
	}
	manifest := read(recordsDir+"/build.json", &build)
	// C generation depends on the upstream origin, grammar and oracle epoch.
	// The separately verified Go carrier is not an input to the C build.
	if build.Schema != 1 || build.OS != "Windows" || build.Pins.Baseline != pins.Baseline ||
		!reflect.DeepEqual(build.Pins.Grammars, pins.Grammars) || !reflect.DeepEqual(build.Pins.Oracle, pins.Oracle) ||
		len(build.Grammars) != len(pins.Grammars) {
		t.Fatal("oracle epoch/build identity mismatch")
	}
	var anchor struct {
		Schema        int
		RuntimeCommit string `json:"runtime_commit"`
		oracleABI
	}
	read("testdata/oracle/runtime-abi.json", &anchor)
	if anchor.Schema != 1 || anchor.RuntimeCommit != pins.Oracle.RuntimeCommit {
		t.Fatal("oracle ABI anchor epoch mismatch")
	}
	for language := range pins.Grammars {
		artifact, exists := build.Grammars[language]
		if !exists {
			t.Fatal("missing oracle grammar")
		}
		if err := checkOracleABI(artifact.ABI, artifact.oracleABI, anchor.oracleABI); err != nil {
			t.Fatal(err)
		}
	}
	if err := provenance.VerifyRuntime(root); err != nil {
		t.Fatal(err)
	}
	t.Logf("runtime origin=%s manifest=%s", pins.Baseline.Version, pins.Runtime.ManifestSHA256)
	for language, grammar := range pins.Grammars {
		blob, err := os.ReadFile(filepath.Join(root, "internal/runtime/grammars", "grammar_blobs", language+".bin"))
		if err != nil || fmt.Sprintf("%x", sha256.Sum256(blob)) != grammar.BlobSHA256 {
			t.Fatalf("grammar identity mismatch: %s", language)
		}
	}
	var cases []oracleCase
	casesJSON := read(casesPath, &cases)
	var set struct {
		Schema      int
		CasesSHA256 string `json:"cases_sha256"`
		Files       map[string]string
	}
	read(recordsDir+"/set.json", &set)
	if set.Schema != 1 || set.CasesSHA256 != fmt.Sprintf("%x", sha256.Sum256(casesJSON)) || len(set.Files) != len(cases)+1 {
		t.Fatal("fixture inventory identity mismatch")
	}
	entries, err := os.ReadDir(filepath.Join(root, recordsDir))
	if err != nil || len(entries) != len(set.Files)+1 {
		t.Fatal("oracle file set changed")
	}
	for name, hash := range set.Files {
		if filepath.Base(name) != name {
			t.Fatal("invalid record path")
		}
		data, err := os.ReadFile(filepath.Join(root, recordsDir, name))
		if err != nil || fmt.Sprintf("%x", sha256.Sum256(data)) != hash {
			t.Fatalf("record file identity mismatch: %s", name)
		}
	}
	var differences map[string]struct {
		Record, Source, GoDigest, CDigest string
	}
	if differencesPath != "" {
		read(differencesPath, &differences)
	}
	if len(cases) == 0 {
		t.Fatal("missing fixture set")
	}
	seen := map[string]bool{}
	byID := map[string]oracleCase{}
	for _, c := range cases {
		byID[c.ID] = c
	}
	idPattern := regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	buildHash := fmt.Sprintf("%x", sha256.Sum256(manifest))
	for _, c := range cases {
		if !idPattern.MatchString(c.ID) || seen[c.ID] || (c.Source == nil) == (c.Path == nil) {
			t.Fatal("duplicate/invalid fixture")
		}
		seen[c.ID] = true
		t.Run(c.ID, func(t *testing.T) {
			var source []byte
			if c.Source != nil {
				source = []byte(*c.Source)
			} else {
				if !filepath.IsLocal(*c.Path) || !strings.HasPrefix(*c.Path, "testdata/") {
					t.Fatal("unbounded fixture path")
				}
				path, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(*c.Path)))
				if err != nil {
					t.Fatal(err)
				}
				relative, err := filepath.Rel(root, path)
				if err != nil || !filepath.IsLocal(relative) {
					t.Fatal("fixture escapes repository")
				}
				source, err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			var receipt oracleRecord
			read(recordsDir+"/"+c.ID+".json", &receipt)
			if err := validateOracle(c, source, receipt, buildHash, build.Grammars[c.Language].ABI); err != nil {
				t.Fatal(err)
			}
			result, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: c.Filename, Source: source, Timeout: 10 * time.Second})
			if result.Tree != nil {
				defer result.Tree.Close()
			}
			if err != nil || result.Language != c.Language || !result.Complete() || result.Tree == nil {
				t.Fatalf("incomplete Go receipt: %+v error=%T", result.Diagnostics, err)
			}
			goNodes := result.Tree.Nodes()
			if (c.Previous == "") != (c.Edit == nil) {
				t.Fatal("incomplete edit fixture")
			}
			if c.Previous != "" {
				previous, ok := byID[c.Previous]
				if !ok || previous.Source == nil || previous.Language != c.Language ||
					fmt.Sprintf("%x", sha256.Sum256([]byte(*previous.Source))) != previous.SHA256 {
					t.Fatal("invalid previous fixture identity")
				}
				old, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: previous.Filename, Source: []byte(*previous.Source), Timeout: 10 * time.Second})
				if old.Tree != nil {
					defer old.Tree.Close()
				}
				if err != nil || !old.Complete() || old.Outcome != syntax.AcceptedClean {
					t.Fatal("incomplete previous tree")
				}
				inc, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: c.Filename, Source: source, Previous: old.Tree, Edit: c.Edit, Timeout: 10 * time.Second})
				if inc.Tree != nil {
					defer inc.Tree.Close()
				}
				if err != nil || !inc.Complete() {
					t.Fatalf("incomplete incremental tree: %+v", inc.Diagnostics)
				}
				t.Logf("incremental source=%s nodes=%d digest=%s diagnostics=%+v", c.SHA256, len(inc.Tree.Nodes()), nodeDigest(inc.Tree.Nodes()), inc.Diagnostics)
				incFresh := logNodeDifference(t, "incremental/fresh", inc.Tree.Nodes(), goNodes)
				incC := logNodeDifference(t, "incremental/C", inc.Tree.Nodes(), receipt.Nodes)
				if incFresh >= 0 || incC >= 0 || (inc.HasError || inc.HasMissing) != receipt.HasError {
					t.Fatal("incremental/fresh/C ordered tree or error-state difference")
				}
			}
			first := -1
			for i := 0; i < min(len(goNodes), len(receipt.Nodes)); i++ {
				if goNodes[i] != receipt.Nodes[i] {
					first = i
					break
				}
			}
			if first < 0 && len(goNodes) != len(receipt.Nodes) {
				first = min(len(goNodes), len(receipt.Nodes))
			}
			t.Logf("fixture=%s source=%s build=%s go_digest=%s c_digest=%s go_len=%d c_len=%d first=%d go=%+v c_error=%t", c.ID, c.SHA256, buildHash, nodeDigest(goNodes), receipt.NodesSHA256, len(goNodes), len(receipt.Nodes), first, result.Diagnostics, receipt.HasError)
			if first >= 0 {
				if first < len(goNodes) {
					t.Logf("Go[%d]=%+v", first, goNodes[first])
				}
				if first < len(receipt.Nodes) {
					t.Logf("C[%d]=%+v", first, receipt.Nodes[first])
				}
			}
			if c.Group == "KR-0001b" {
				if first >= 0 || receipt.HasError || result.Outcome != syntax.AcceptedClean {
					t.Fatal("bundled scanner regressed on JSX equals text")
				}
				return
			}
			if known, ok := differences[c.ID]; ok {
				if known.Source != c.SHA256 || known.CDigest != receipt.NodesSHA256 || known.GoDigest != nodeDigest(goNodes) {
					t.Fatalf("STALE or NEW REGRESSION %s: exact recorded difference changed", known.Record)
				}
				if c.ID == "CS-JsonTextReader-excerpt" {
					if known.Record != "KR-0002" || !receipt.HasError || result.HasError || !result.HasMissing || result.Outcome != syntax.AcceptedWithErrors {
						t.Fatal("KR-0002 recovery signature changed")
					}
					return
				}
				if c.Group == "KR-0003" || c.Group == "KR-0004" {
					if known.Record != c.Group || first < 0 || !receipt.HasError || result.HasError || result.HasMissing || result.Outcome != syntax.AcceptedClean {
						t.Fatalf("%s clean-Go/error-C signature changed", c.Group)
					}
					return
				}
			} else if c.Group == "KR-0001a" || c.Group == "KR-0004" {
				t.Fatal("missing known-difference identity")
			}
			if c.Group == "KR-0001a" {
				if !receipt.HasError || result.Outcome != syntax.AcceptedWithErrors || !result.HasError {
					t.Fatal("NEW REGRESSION: bare ampersand characterization")
				}
				return // Recovered-shape differences are explicitly documented for these inputs.
			}
			if first >= 0 {
				t.Fatal("unregistered ordered tree difference")
			}
			if receipt.HasError || result.Outcome != syntax.AcceptedClean {
				t.Fatal("clean control outcome changed")
			}
		})
	}
	for id := range differences {
		if !seen[id] {
			t.Fatalf("recorded fixture missing: %s", id)
		}
	}
}

func TestCSharpRecoveryOrigin(t *testing.T) {
	source, err := os.ReadFile("../../testdata/newtonsoft/JsonTextReader-excerpt.cs")
	if err != nil {
		t.Fatal(err)
	}
	lang := grammars.DetectLanguageByName("c_sharp").Language()
	parser := gts.NewParser(lang)
	parser.SetTimeoutMicros(10000000)
	raw, err := parser.Parse(source)
	if raw != nil {
		defer raw.Release()
	}
	if err != nil || raw == nil || raw.RootNode() == nil {
		t.Fatalf("raw runtime failure: %T", err)
	}
	nodes := snapshot(raw.RootNode(), lang)
	if nodeDigest(nodes) != "a52dc375a5c88da82a979a2f2c62fb2a854900d22f24aea82309d1e3ce1a6f79" {
		t.Fatal("raw runtime recovery signature changed")
	}
	t.Logf("raw runtime nodes=%d digest=%s error=%t stop=%s", len(nodes), nodeDigest(nodes), raw.RootNode().HasError(), raw.ParseRuntime().StopReason)
}

func TestOracleRejectsChangedEvidence(t *testing.T) {
	anchor := oracleABI{Min: 13, Max: 15, Header: "fixed-header"}
	if err := checkOracleABI(15, anchor, anchor); err != nil {
		t.Fatal(err)
	}
	for _, artifact := range []oracleABI{{Min: 999, Max: 999, Header: "fixed-header"}, {Min: 13, Max: 15, Header: "changed-header"}} {
		if checkOracleABI(artifact.Max, artifact, anchor) == nil {
			t.Fatal("coherently changed ABI accepted")
		}
	}
	if checkOracleABI(16, anchor, anchor) == nil {
		t.Fatal("unsupported ABI accepted")
	}
	source := []byte("x")
	c := oracleCase{ID: "check", Language: "go", SHA256: fmt.Sprintf("%x", sha256.Sum256(source))}
	r := oracleRecord{Schema: 1, Fixture: c.ID, Language: c.Language, ABI: 15, InputBytes: 1, SourceSHA256: c.SHA256, BuildSHA256: "build", End: 1, Nodes: []syntax.Node{{Type: "root", Parent: -1, EndByte: 1}}}
	r.NodesSHA256 = nodeDigest(r.Nodes)
	if err := validateOracle(c, source, r, "build", 15); err != nil {
		t.Fatal(err)
	}
	for _, alter := range []func(*oracleRecord){
		func(r *oracleRecord) { r.ABI++ }, func(r *oracleRecord) { r.BuildSHA256 = "other" },
		func(r *oracleRecord) { r.SourceSHA256 = "other" }, func(r *oracleRecord) { r.End = 0 },
		func(r *oracleRecord) { r.NodesSHA256 = "other" }, func(r *oracleRecord) { r.Nodes = nil },
	} {
		changed := r
		alter(&changed)
		if validateOracle(c, source, changed, "build", 15) == nil {
			t.Fatal("changed evidence accepted")
		}
	}
}
