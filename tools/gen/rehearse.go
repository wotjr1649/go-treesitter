package main

// `gen nightly-rehearsal`: a REPORT-ONLY comparison of the pinned tree-sitter CLI with the CLI of the `nightly` release.
// It is not a gate and not evidence of parity: it regenerates each grammar twice through the same
// grammarpatch.Prepare path (inputs, patches, double generation), once with the pinned, hash-verified CLI and once with
// an unpinned nightly CLI whose identity is recorded and cross-checked instead of pinned, then reports which generated
// files differ. It never writes under grammars/ and never touches pins.json. The product path (regen.sh, gen -lang) is unchanged.
// With -nightly-sandbox every nightly execution goes through that wrapper (tools/gen/nightly-sandbox.sh: bubblewrap, no
// network, no environment, only the grammar's own nightly work directory visible and writable); this trusted process
// prepares the inputs and reads the results.

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/wotjr1649/go-treesitter/tools/bench/proc"
	"github.com/wotjr1649/go-treesitter/tools/gen/grammarpatch"
	"github.com/wotjr1649/go-treesitter/tools/gen/schema"
)

var rehearsalKeyFiles = []string{"parser.c", "grammar.json", "node-types.json"}

// Seams for tests: the real preparation needs both CLIs, and the pinned CLI exists only for the platforms in pins.json.
var (
	prepareGrammar     = grammarpatch.Prepare
	checkGrammarInputs = grammarpatch.CheckInputs
	rehearsalPlatform  = hostPlatform
)

const nightlyAssetPrefix = "https://github.com/tree-sitter/tree-sitter/releases/download/nightly/"

type rehearsalFile struct {
	Name    string `json:"name"`
	Change  string `json:"change"` // changed, added (only nightly), removed (only nightly lacks it)
	Pinned  string `json:"pinnedSHA256,omitempty"`
	Nightly string `json:"nightlySHA256,omitempty"`
}

type rehearsalSchema struct {
	Assessment string         `json:"assessment"` // identical (equal hashes), PASS, FAIL (differences), BLOCKED (a side missing, unreadable or not strict)
	Reason     string         `json:"reason,omitempty"`
	Changes    int            `json:"changes"`
	ByCode     map[string]int `json:"byCode,omitempty"`
	ByRisk     map[string]int `json:"byRisk,omitempty"`
}

type rehearsalRun struct {
	OK      bool    `json:"ok"`
	Error   string  `json:"error,omitempty"`
	Seconds float64 `json:"seconds"`
}

type rehearsalGrammar struct {
	Lang string `json:"lang"`
	// identical, headers-only, differs (both generated); pinned-failed, nightly-failed, both-failed; input-failed (common
	// input check failed, neither CLI ran)
	Status         string           `json:"status"`
	Error          string           `json:"error,omitempty"` // input-failed only
	Pinned         *rehearsalRun    `json:"pinned,omitempty"`
	Nightly        *rehearsalRun    `json:"nightly,omitempty"`
	Files          []rehearsalFile  `json:"files,omitempty"`
	Schema         *rehearsalSchema `json:"schema,omitempty"`
	ArtifactErrors []string         `json:"artifactErrors,omitempty"` // a generated file could not be read or its copy not written
}

// nightlyMeta is what tools/gen/nightly-rehearsal.sh observed through the release API, before running anything.
type nightlyMeta struct {
	URL             string `json:"url"`
	Size            int64  `json:"size"`
	APIDigest       string `json:"apiDigest"`
	PublishedAt     string `json:"publishedAt"`
	TagCommitBefore string `json:"tagCommitBefore"`
	TagCommitAfter  string `json:"tagCommitAfter"`
	TagMoved        bool   `json:"tagMoved"`
}

type nightlyIdentity struct {
	Version     string       `json:"version,omitempty"` // first line of `--version`
	BuildCommit string       `json:"buildCommit,omitempty"`
	GzipBytes   int64        `json:"gzipBytes"`
	GzipSHA256  string       `json:"gzipSHA256"`
	BinSHA256   string       `json:"binarySHA256"`
	Sandbox     string       `json:"sandbox"` // bwrap, or none (local run without the wrapper)
	Meta        *nightlyMeta `json:"meta,omitempty"`
	// verified: metadata, bytes, tag and --version agree; unchecked: no metadata given (local run); held: missing or
	// mismatched identity, so no grammar was compared
	Identity string   `json:"identity"`
	Problems []string `json:"identityProblems,omitempty"`
}

type rehearsalReport struct {
	Kind       string `json:"kind"`
	ReportOnly bool   `json:"reportOnly"`
	Platform   string `json:"platform"`
	Pinned     struct {
		Version       string `json:"version"`
		ArchiveSHA256 string `json:"archiveSHA256"`
	} `json:"pinned"`
	Nightly  nightlyIdentity    `json:"nightly"`
	Grammars []rehearsalGrammar `json:"grammars"`
	Digest   string             `json:"digest,omitempty"` // result digest: no nightly identity in it, so it changes only when a result changes
}

// compareOutputs lists every file that differs between the two generated trees, sorted by name.
func compareOutputs(pinned, nightly map[string]string) []rehearsalFile {
	var out []rehearsalFile
	for n, p := range pinned {
		switch q, ok := nightly[n]; {
		case !ok:
			out = append(out, rehearsalFile{Name: n, Change: "removed", Pinned: p})
		case p != q:
			out = append(out, rehearsalFile{Name: n, Change: "changed", Pinned: p, Nightly: q})
		}
	}
	for n, q := range nightly {
		if _, ok := pinned[n]; !ok {
			out = append(out, rehearsalFile{Name: n, Change: "added", Nightly: q})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func schemaSummary(pinned, nightly []byte) *rehearsalSchema {
	if sumBytes(pinned) == sumBytes(nightly) {
		return &rehearsalSchema{Assessment: "identical"}
	}
	a, b := schema.Check(pinned), schema.Check(nightly)
	if a.Assessment != "PASS" || b.Assessment != "PASS" {
		return &rehearsalSchema{Assessment: "BLOCKED", Reason: fmt.Sprintf("strict check: pinned %s, nightly %s", a.Assessment, b.Assessment)}
	}
	s := &rehearsalSchema{Assessment: "PASS", ByCode: map[string]int{}, ByRisk: map[string]int{}}
	for _, c := range schema.Diff(a.Nodes, b.Nodes) {
		s.Changes++
		s.ByCode[c.Code]++
		s.ByRisk[c.Risk]++
	}
	if s.Changes > 0 {
		s.Assessment = "FAIL"
	}
	return s
}

func sortedCounts(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		keys[i] = fmt.Sprintf("%s=%d", k, m[k])
	}
	return strings.Join(keys, ",")
}

// outcome is the one line per grammar that the digest hashes: status, every differing file with its change kind and both
// SHA-256s, the schema verdict and its per-code and per-risk counts. Never timings, error text or the CLI's identity.
func (g rehearsalGrammar) outcome() string {
	s := g.Lang + "\t" + g.Status
	for _, f := range g.Files { // sorted by compareOutputs
		s += "\t" + f.Name + ":" + f.Change + ":" + f.Pinned + ":" + f.Nightly
	}
	if g.Schema != nil {
		s += "\tschema=" + g.Schema.Assessment + ":" + sortedCounts(g.Schema.ByCode) + ":" + sortedCounts(g.Schema.ByRisk)
	}
	return s
}

func (r *rehearsalReport) computeDigest() string {
	var b strings.Builder
	for _, g := range r.Grammars {
		b.WriteString(g.outcome() + "\n")
	}
	return sumBytes([]byte(b.String()))
}

func yesNo(files []rehearsalFile, name string) string {
	for _, f := range files {
		if f.Name == name {
			return f.Change
		}
	}
	return "same"
}

func (r *rehearsalReport) markdown() string {
	var b strings.Builder
	short := func(s string) string {
		if len(s) > 12 {
			return s[:12]
		}
		return s
	}
	fmt.Fprintf(&b, "### Nightly CLI rehearsal (report only, %s)\n\n", r.Platform)
	fmt.Fprintf(&b, "Not a gate and not parity evidence: the pinned CLI `%s` against the unpinned nightly CLI `%s` "+
		"(asset sha256 `%s`, binary sha256 `%s`, %d bytes; identity **%s**, sandbox `%s`). Same inputs, patches and double generation for both.\n\n",
		r.Pinned.Version, r.Nightly.Version, short(r.Nightly.GzipSHA256), short(r.Nightly.BinSHA256), r.Nightly.GzipBytes,
		r.Nightly.Identity, r.Nightly.Sandbox)
	if r.Nightly.Identity == "held" {
		b.WriteString("**Comparison held**: the nightly CLI's identity is missing or inconsistent, so no grammar was compared and the tracking issue is not updated.\n\n")
		for _, p := range r.Nightly.Problems {
			fmt.Fprintf(&b, "- %s\n", p)
		}
		return b.String()
	}
	b.WriteString("| grammar | result | parser.c | grammar.json | node-types.json | other files | schema diff | seconds pinned/nightly |\n|---|---|---|---|---|---|---|---|\n")
	counts := map[string]int{}
	secs := func(r *rehearsalRun) string {
		if r == nil {
			return "-"
		}
		return fmt.Sprintf("%.0f", r.Seconds)
	}
	for _, g := range r.Grammars {
		counts[g.Status]++
		if g.Pinned == nil || g.Nightly == nil || !g.Pinned.OK || !g.Nightly.OK {
			fmt.Fprintf(&b, "| %s | %s | - | - | - | - | - | %s / %s |\n", g.Lang, g.Status, secs(g.Pinned), secs(g.Nightly))
			continue
		}
		other := 0
		for _, f := range g.Files {
			if !slices.Contains(rehearsalKeyFiles, f.Name) {
				other++
			}
		}
		sch := "-"
		if g.Schema != nil {
			sch = g.Schema.Assessment
			if g.Schema.Changes > 0 {
				sch = fmt.Sprintf("%d changes (%s)", g.Schema.Changes, sortedCounts(g.Schema.ByRisk))
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %d | %s | %s / %s |\n", g.Lang, g.Status,
			yesNo(g.Files, "parser.c"), yesNo(g.Files, "grammar.json"), yesNo(g.Files, "node-types.json"), other, sch,
			secs(g.Pinned), secs(g.Nightly))
	}
	var parts []string
	for _, s := range []string{"identical", "headers-only", "differs", "pinned-failed", "nightly-failed", "both-failed", "input-failed"} {
		if counts[s] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
		}
	}
	fmt.Fprintf(&b, "\nTotal: %s. Result digest `%s`.\n", strings.Join(parts, ", "), short(r.Digest))
	for _, g := range r.Grammars {
		for _, e := range []struct{ what, text string }{{"inputs", g.Error}, {"pinned", runError(g.Pinned)}, {"nightly", runError(g.Nightly)}} {
			if e.text != "" {
				fmt.Fprintf(&b, "\n- **%s** %s: `%s`", g.Lang, e.what, e.text)
			}
		}
		if g.Schema != nil && g.Schema.Reason != "" {
			fmt.Fprintf(&b, "\n- **%s** schema %s: %s", g.Lang, g.Schema.Assessment, g.Schema.Reason)
		}
		for _, e := range g.ArtifactErrors {
			fmt.Fprintf(&b, "\n- **%s** artifact error: `%s`", g.Lang, e)
		}
	}
	b.WriteString("\n")
	return b.String()
}

func runError(r *rehearsalRun) string {
	if r == nil {
		return ""
	}
	return r.Error
}

func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		s = s[:n] + "..."
	}
	return strings.ReplaceAll(s, "`", "'")
}

// gunzipTo decompresses gz into dst (executable) and returns the SHA-256 of the decompressed bytes.
func gunzipTo(gz []byte, dst string) (string, error) {
	z, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), z)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return hex.EncodeToString(h.Sum(nil)), err
}

func saveGz(dst string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	z := gzip.NewWriter(f)
	_, err = z.Write(data)
	if cerr := z.Close(); err == nil {
		err = cerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

var (
	versionCommit = regexp.MustCompile(`^tree-sitter \S+ \(([0-9a-f]{7,40})\)`)
	fullCommit    = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// assetProblems checks the API metadata against the bytes on disk; it runs before the binary is executed.
func assetProblems(m *nightlyMeta, gzBytes int64, gzSum string) []string {
	var p []string
	if !strings.HasPrefix(m.URL, nightlyAssetPrefix) {
		p = append(p, fmt.Sprintf("asset URL %q is not under %s", m.URL, nightlyAssetPrefix))
	}
	if m.Size != gzBytes {
		p = append(p, fmt.Sprintf("release API size %d, downloaded %d bytes", m.Size, gzBytes))
	}
	if m.APIDigest != "sha256:"+gzSum {
		p = append(p, fmt.Sprintf("release API digest %q, downloaded sha256:%s", m.APIDigest, gzSum))
	}
	return p
}

// commitProblems binds the build commit printed by --version to the `nightly` tag read before and after the download.
func commitProblems(m *nightlyMeta, commit string) []string {
	var p []string
	if commit == "" {
		p = append(p, "--version names no build commit")
	}
	if m == nil {
		return p
	}
	if !fullCommit.MatchString(m.TagCommitBefore) || !fullCommit.MatchString(m.TagCommitAfter) {
		p = append(p, fmt.Sprintf("tag lookups %q / %q are not commits", m.TagCommitBefore, m.TagCommitAfter))
	} else if m.TagCommitBefore != m.TagCommitAfter || m.TagMoved {
		p = append(p, fmt.Sprintf("nightly tag moved during the download: %s -> %s", m.TagCommitBefore, m.TagCommitAfter))
	} else if commit != "" && !strings.HasPrefix(m.TagCommitBefore, commit) {
		p = append(p, fmt.Sprintf("--version build commit %s is not the tag commit %s", commit, m.TagCommitBefore))
	}
	return p
}

type rehearsal struct {
	pins           *Pins
	root, dl, out  string
	pinnedBin      string
	nightlyCLI     string // the binary, or the sandbox wrapper that runs it
	nightlyVersion string
	sandboxed      bool
}

func runRehearsalCommand(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("nightly-rehearsal", flag.ContinueOnError)
	root := fs.String("root", ".", "tools/gen directory (pins.json, patches/)")
	dl := fs.String("dl", "", "directory with the SHA-256-verified pinned archives (grammar sources and the pinned CLI .gz)")
	pinnedCLI := fs.String("tree-sitter", "", "pinned tree-sitter CLI binary (verified against pins.json)")
	nightlyGz := fs.String("nightly-gz", "", "downloaded nightly CLI .gz (no pin: its identity is recorded and cross-checked)")
	nightlyMetaPath := fs.String("nightly-meta", "", "JSON object with the release API facts (url, size, apiDigest \"sha256:...\", tag commits); without it the identity is 'unchecked'")
	sandbox := fs.String("nightly-sandbox", "", "wrapper that runs the nightly CLI isolated (tools/gen/nightly-sandbox.sh); required unless -nightly-unsandboxed")
	unsandboxed := fs.Bool("nightly-unsandboxed", false, "local opt-in: run the nightly CLI directly, with this user's access to files, credentials and network")
	out := fs.String("out", "", "new empty output directory (report.json, summary.md, digest.txt, differing generated files)")
	fetchNPM := fs.Bool("fetch-npm", false, "download missing lockfile-pinned npm grammar tarballs during the common input check (no install scripts)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// Exactly one of -nightly-sandbox and -nightly-unsandboxed: without the wrapper nothing downloaded runs by default.
	if *dl == "" || *pinnedCLI == "" || *nightlyGz == "" || *out == "" || (*sandbox == "") == !*unsandboxed {
		return errors.New("usage: gen nightly-rehearsal -root DIR -dl DIR -tree-sitter PINNED_CLI -nightly-gz FILE.gz -out NEW_DIR (-nightly-sandbox WRAPPER | -nightly-unsandboxed) [-nightly-meta FILE.json] [-fetch-npm] [lang ...]")
	}
	pins, err := loadPins(filepath.Join(*root, "pins.json"))
	if err != nil {
		return err
	}
	if entries, err := os.ReadDir(*out); err == nil && len(entries) > 0 {
		return fmt.Errorf("-out %s is not empty", *out)
	}
	absOut, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(absOut, "work"), 0o755); err != nil {
		return err
	}
	cliArchive, ok := pins.Tools.TreeSitterCLIArchives[rehearsalPlatform()]
	if !ok {
		return fmt.Errorf("no pinned CLI for %s", rehearsalPlatform())
	}
	langs := fs.Args()
	if len(langs) == 0 {
		for n := range pins.Grammars {
			langs = append(langs, n)
		}
		sort.Strings(langs)
	}
	for _, lang := range langs {
		if _, ok := pins.Grammars[lang]; !ok {
			return fmt.Errorf("unknown grammar %q", lang)
		}
	}
	r := &rehearsal{pins: pins, root: *root, dl: *dl, out: absOut, sandboxed: *sandbox != ""}
	if r.pinnedBin, err = filepath.Abs(*pinnedCLI); err != nil {
		return err
	}
	rep := &rehearsalReport{Kind: "nightly-rehearsal", ReportOnly: true, Platform: rehearsalPlatform()}
	rep.Pinned.Version, rep.Pinned.ArchiveSHA256 = pins.Tools.TreeSitterCLI, cliArchive.SHA256
	rep.Nightly.Sandbox = "none"

	// Common input check first (trusted, may use the network for npm inputs), before any nightly byte runs.
	inputErr := map[string]error{}
	for _, lang := range langs {
		c := r.config(pins.Grammars[lang], "", false)
		c.FetchNPM = *fetchNPM
		inputErr[lang] = checkGrammarInputs(c)
	}

	// Nightly identity: the bytes on disk, the API facts, then the build commit of --version against the tag.
	nightlyBin := filepath.Join(absOut, "nightly-bin", "tree-sitter")
	if runtime.GOOS == "windows" {
		nightlyBin += ".exe"
	}
	if err := os.MkdirAll(filepath.Dir(nightlyBin), 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(filepath.Dir(nightlyBin)) // the reports are the product, not the binary
	defer os.RemoveAll(filepath.Join(absOut, "work"))
	r.nightlyCLI = nightlyBin
	if r.sandboxed {
		if r.nightlyCLI, err = filepath.Abs(*sandbox); err != nil {
			return err
		}
		rep.Nightly.Sandbox = "bwrap"
		os.Setenv("NIGHTLY_BIN", nightlyBin)
	}
	// Every failure from here on is a recorded identity problem (held report, no digest), not an early exit.
	held := func(format string, a ...any) {
		rep.Nightly.Problems = append(rep.Nightly.Problems, oneLine(fmt.Sprintf(format, a...), 600))
	}
	gz, gzErr := os.ReadFile(*nightlyGz)
	if gzErr != nil {
		held("reading the asset: %v", gzErr)
	} else {
		rep.Nightly.GzipBytes, rep.Nightly.GzipSHA256 = int64(len(gz)), sumBytes(gz)
	}
	var meta *nightlyMeta
	if *nightlyMetaPath != "" {
		if b, err := os.ReadFile(*nightlyMetaPath); err != nil {
			held("reading -nightly-meta: %v", err)
		} else if err := json.Unmarshal(b, &meta); err != nil || meta == nil {
			meta = nil
			held("-nightly-meta is not a JSON object: %v", err)
		} else {
			rep.Nightly.Meta = meta
			if gzErr == nil { // size and digest of the compressed bytes, before decompressing them
				rep.Nightly.Problems = append(rep.Nightly.Problems, assetProblems(meta, rep.Nightly.GzipBytes, rep.Nightly.GzipSHA256)...)
			}
		}
	}
	if len(rep.Nightly.Problems) == 0 {
		if rep.Nightly.BinSHA256, err = gunzipTo(gz, nightlyBin); err != nil {
			held("decompressing the asset: %v", err)
		}
	}
	if len(rep.Nightly.Problems) == 0 { // never run bytes that do not match what the API described
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		cmd := proc.CommandContext(ctx, r.nightlyCLI, "--version")
		cmd.Env = grammarpatch.UnpinnedEnv() // no GitHub token, no generate mount variables (also with -nightly-unsandboxed)
		vout, err := cmd.CombinedOutput()
		cancel()
		rep.Nightly.Version = oneLine(strings.SplitN(string(vout), "\n", 2)[0], 200)
		if err != nil {
			rep.Nightly.Problems = append(rep.Nightly.Problems, oneLine(fmt.Sprintf("--version: %v: %s", err, vout), 600))
		} else {
			if m := versionCommit.FindStringSubmatch(rep.Nightly.Version); m != nil {
				rep.Nightly.BuildCommit = m[1]
			}
			rep.Nightly.Problems = append(rep.Nightly.Problems, commitProblems(meta, rep.Nightly.BuildCommit)...)
		}
	}
	switch {
	case len(rep.Nightly.Problems) > 0:
		rep.Nightly.Identity = "held"
	case meta == nil:
		rep.Nightly.Identity = "unchecked"
	default:
		rep.Nightly.Identity = "verified"
	}
	r.nightlyVersion = rep.Nightly.Version

	if rep.Nightly.Identity != "held" {
		for _, lang := range langs {
			rep.Grammars = append(rep.Grammars, r.grammar(lang, pins.Grammars[lang], inputErr[lang]))
		}
		rep.Digest = rep.computeDigest()
	}
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	files := map[string][]byte{"report.json": append(b, '\n'), "summary.md": []byte(rep.markdown())}
	if rep.Digest != "" {
		files["digest.txt"] = []byte(rep.Digest + "\n") // no digest for a held run: nothing to post
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(absOut, name), data, 0o644); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(stdout, rep.markdown()); err != nil {
		return err
	}
	if rep.Nightly.Identity == "held" {
		return fmt.Errorf("nightly identity held, comparison not run: %s", strings.Join(rep.Nightly.Problems, "; "))
	}
	for _, g := range rep.Grammars {
		if len(g.ArtifactErrors) > 0 {
			return fmt.Errorf("%s: artifact errors (see report.json)", g.Lang)
		}
	}
	return nil
}

func (r *rehearsal) config(g GrammarPin, work string, nightly bool) grammarpatch.Config {
	c := grammarpatch.Config{Archive: filepath.Join(r.dl, g.Archive), ArchiveSHA256: g.ArchiveSHA256,
		PatchRoot: r.root, Work: work, GrammarDir: g.GrammarDir, Patches: g.Patches, Regenerate: true,
		GenerateFrom: g.GenerateFrom, NPMDependencies: g.NPMDependencies, NPMCache: filepath.Join(r.dl, "npm")}
	if nightly {
		c.CLI, c.Unpinned, c.CLIVersion = r.nightlyCLI, true, r.nightlyVersion
	} else {
		cli := r.pins.Tools.TreeSitterCLIArchives[rehearsalPlatform()]
		c.CLI, c.CLIArchive, c.CLIArchiveSHA256, c.CLIVersion = r.pinnedBin, filepath.Join(r.dl, path.Base(cli.URL)), cli.SHA256, r.pins.Tools.TreeSitterCLI
	}
	return c
}

// grammar never returns an error: every failure is the grammar's own recorded result. Both CLIs run independently once
// the common inputs passed; files are compared only when both succeeded.
func (r *rehearsal) grammar(lang string, g GrammarPin, inputErr error) rehearsalGrammar {
	res := rehearsalGrammar{Lang: lang}
	if inputErr != nil {
		res.Status, res.Error = "input-failed", oneLine(inputErr.Error(), 600)
		return res
	}
	// A grammar with a generator exception is regenerated too: the rehearsal compares CLI against CLI, never with the committed reference C.
	scratch := filepath.Join(r.out, "work", lang)
	defer os.RemoveAll(scratch)
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		res.Status, res.Error = "input-failed", oneLine(err.Error(), 300)
		return res
	}
	run := func(nightly bool) (string, *grammarpatch.Provenance, *rehearsalRun) {
		work := filepath.Join(scratch, map[bool]string{false: "pinned", true: "nightly"}[nightly])
		t0 := time.Now()
		root, prov, err := prepareGrammar(r.config(g, work, nightly))
		rr := &rehearsalRun{OK: err == nil, Seconds: time.Since(t0).Seconds()}
		if err != nil {
			rr.Error = oneLine(err.Error(), 600)
		}
		return root, prov, rr
	}
	var pRoot, nRoot string
	var pProv, nProv *grammarpatch.Provenance
	pRoot, pProv, res.Pinned = run(false)
	nRoot, nProv, res.Nightly = run(true)
	switch {
	case !res.Pinned.OK && !res.Nightly.OK:
		res.Status = "both-failed"
		return res
	case !res.Pinned.OK:
		res.Status = "pinned-failed"
		return res
	case !res.Nightly.OK:
		res.Status = "nightly-failed"
		return res
	}
	res.Files = compareOutputs(pProv.Outputs, nProv.Outputs)
	res.Status = "identical"
	if len(res.Files) > 0 {
		res.Status = "headers-only" // only files other than parser.c, grammar.json, node-types.json differ (tree_sitter/*.h come from the CLI)
	}
	for _, f := range res.Files {
		if slices.Contains(rehearsalKeyFiles, f.Name) {
			res.Status = "differs"
		}
	}
	dir := func(root string) string { return filepath.Join(root, filepath.FromSlash(g.GrammarDir)) }
	var nodes [2][]byte
	var nodesRead [2]bool
	for _, f := range res.Files {
		if !slices.Contains(rehearsalKeyFiles, f.Name) {
			continue
		}
		// Copy only the sides the comparison recorded as present: removed has no nightly file, added no pinned one.
		for i, side := range []struct {
			name, root string
			present    bool
		}{{"pinned", pRoot, f.Change != "added"}, {"nightly", nRoot, f.Change != "removed"}} {
			if !side.present {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir(side.root), f.Name))
			if err != nil {
				res.ArtifactErrors = append(res.ArtifactErrors, oneLine("reading "+side.name+" "+f.Name+": "+err.Error(), 300))
				continue
			}
			if f.Name == "node-types.json" { // read is what the schema verdict needs; a failed copy is only an artifact error
				nodes[i], nodesRead[i] = b, true
			}
			if err := saveGz(filepath.Join(r.out, "files", lang, side.name, f.Name+".gz"), b); err != nil {
				res.ArtifactErrors = append(res.ArtifactErrors, oneLine("saving "+side.name+" "+f.Name+": "+err.Error(), 300))
			}
		}
	}
	// Always, also when no file differs: a pair without node-types.json is BLOCKED, never identical.
	res.Schema = nodeTypesSchema(res.Files, pProv.Outputs, nodes, nodesRead)
	return res
}

// nodeTypesSchema reports identical only for equal hashes, and BLOCKED with the reason when a side is missing or unreadable.
func nodeTypesSchema(files []rehearsalFile, pinnedOutputs map[string]string, nodes [2][]byte, read [2]bool) *rehearsalSchema {
	const nt = "node-types.json"
	i := slices.IndexFunc(files, func(f rehearsalFile) bool { return f.Name == nt })
	switch {
	case i < 0 && pinnedOutputs[nt] != "": // not listed: both sides have it with the same hash
		return &rehearsalSchema{Assessment: "identical"}
	case i < 0:
		return &rehearsalSchema{Assessment: "BLOCKED", Reason: nt + " missing on both sides"}
	case files[i].Change == "added":
		return &rehearsalSchema{Assessment: "BLOCKED", Reason: nt + " missing on the pinned side"}
	case files[i].Change == "removed":
		return &rehearsalSchema{Assessment: "BLOCKED", Reason: nt + " missing on the nightly side"}
	case !read[0] || !read[1]:
		return &rehearsalSchema{Assessment: "BLOCKED", Reason: nt + " unreadable (see artifact errors)"}
	}
	return schemaSummary(nodes[0], nodes[1])
}
