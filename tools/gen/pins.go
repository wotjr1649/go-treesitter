package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// Pins is tools/gen/pins.json: the machine-readable source of truth for everything a generation consumes.
type Pins struct {
	Schema           int                   `json:"schema"`
	ABI              int                   `json:"abi"`
	Recipe           string                `json:"recipe"` // build variant, a key of recipes in recipe.go (-recipe overrides it for comparisons)
	Runtime          RuntimePin            `json:"runtime"`
	Tools            ToolPins              `json:"tools"`
	MemoryGrowSHA256 string                `json:"memoryGrowSHA256"` // sha256 of the original wasm2go memory_grow function text
	ShimSHA256       string                `json:"shimSHA256"`       // shim/tsg_abi.c
	WasiLibc         WasiLibcPin           `json:"wasiLibc"`         // source of the libc linked into every module: notices only
	Grammars         map[string]GrammarPin `json:"grammars"`
}

type RuntimePin struct {
	Release       string   `json:"release"`
	Repo          string   `json:"repo"`
	Commit        string   `json:"commit"`
	ExtractDir    string   `json:"extractDir"` // under -src
	Archive       string   `json:"archive"`    // file name under -dl
	ArchiveSHA256 string   `json:"archiveSHA256"`
	Dirs          []string `json:"dirs"` // hashed trees, relative to ExtractDir
	TreeSHA256    string   `json:"treeSHA256"`
	LicenseSHA256 string   `json:"licenseSHA256"` // LICENSE of the release archive, copied to LICENSES/tree-sitter.txt in every module
}

// WasiLibcPin is the wasi-libc source tree the pinned wasi-sdk's libc.a was built from (the sdk's VERSION file names
// the commit; the sdk ships no license text). Only its license files are used: gen copies them into every module.
type WasiLibcPin struct {
	Repo          string      `json:"repo"`
	Commit        string      `json:"commit"` // full commit; its first 12 hex digits must equal the sdk's VERSION "wasi-libc:" line
	ExtractDir    string      `json:"extractDir"`
	Archive       string      `json:"archive"`
	ArchiveSHA256 string      `json:"archiveSHA256"`
	Notices       []NoticePin `json:"notices"`
}

// NoticePin is one license text copied to LICENSES/wasi-libc/<Name> in every module.
type NoticePin struct {
	Name         string `json:"name"`
	Path         string `json:"path"`                   // relative to ExtractDir
	SHA256       string `json:"sha256"`                 // of the whole source file
	LeadingLines int    `json:"leadingLines,omitempty"` // >0: only the first N lines (a notice embedded in a source file's header)
}

// Archive is one upstream release archive; its sha256 is verified before anything inside it is used.
type Archive struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// ToolPins identify tools by upstream source and version, never by a host binary hash: a generated module's
// identity must not depend on the host that generated it. Archives are keyed per host platform "<GOOS>-<GOARCH>".
type ToolPins struct {
	WasiSDK         string             `json:"wasi-sdk"`
	Clang           string             `json:"clang"`      // clang release number (host strings differ in the suffix)
	LLVMCommit      string             `json:"llvmCommit"` // llvm-project commit clang was built from
	WasiSDKArchives map[string]Archive `json:"wasiSDKArchives"`
	Wasm2go         string             `json:"wasm2go"`       // module version
	Wasm2goModule   string             `json:"wasm2goModule"` // built with: go install -trimpath <module>@<version>
	Wasm2goSum      string             `json:"wasm2goSum"`    // go.sum h1: hash of that module version (read back from the binary's build info)
	// Binaryen is needed only by the recipes that run wasm-opt.
	Binaryen         string             `json:"binaryen,omitempty"` // binaryen release number (version_N)
	BinaryenArchives map[string]Archive `json:"binaryenArchives,omitempty"`
	// LocalBinarySHA256 holds host binary hashes (platform -> tool -> sha256). They are checked only with -verify-binaries
	// and are not part of any identity.
	LocalBinarySHA256 map[string]map[string]string `json:"localBinarySHA256,omitempty"`
}

type GrammarPin struct {
	Repo          string            `json:"repo"`
	Commit        string            `json:"commit"` // full commit
	ExtractDir    string            `json:"extractDir"`
	Archive       string            `json:"archive"`
	ArchiveSHA256 string            `json:"archiveSHA256"`
	ArchiveURL    string            `json:"archiveURL,omitempty"` // explicit upstream archive URL when its bytes differ from the API tarball
	GrammarDir    string            `json:"grammarDir"`           // directory with parser.c, relative to ExtractDir
	Dirs          []string          `json:"dirs"`                 // hashed trees, relative to ExtractDir
	TreeSHA256    string            `json:"treeSHA256"`
	Files         map[string]string `json:"files"` // parser.c / scanner.c -> sha256
	LicenseSHA256 string            `json:"licenseSHA256"`
	NoticeSHA256  string            `json:"noticeSHA256,omitempty"` // upstream NOTICE file, when the grammar repository has one
	CSymbol       string            `json:"cSymbol"`                // tree_sitter_xxx
}

func loadPins(path string) (*Pins, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Pins
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &p, nil
}

func sumBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func sumFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// treeSHA256 hashes every regular file under the given directories of root: sha256 over the sorted lines
// "<slash path relative to root>\x00<file sha256>\n". Directory names are inputs, so moving a file changes the hash.
func treeSHA256(root string, dirs []string) (string, error) {
	var lines []string
	for _, d := range dirs {
		base := filepath.Join(root, filepath.FromSlash(d))
		err := filepath.WalkDir(base, func(p string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return err
			}
			if !e.Type().IsRegular() {
				return fmt.Errorf("%s: not a regular file", p)
			}
			s, err := sumFile(p)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			lines = append(lines, filepath.ToSlash(rel)+"\x00"+s+"\n")
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	if len(lines) == 0 {
		return "", fmt.Errorf("no files under %v", dirs)
	}
	sort.Strings(lines)
	return sumBytes([]byte(strings.Join(lines, ""))), nil
}

func expect(what, got, want string) error {
	if got != want {
		return fmt.Errorf("pin mismatch: %s = %s, pinned %s", what, got, want)
	}
	return nil
}

// tarballURL is the GitHub API tarball of a full commit. Its bytes are what archiveSHA256 pins.
func tarballURL(repo, commit string) (string, error) {
	const p = "https://github.com/"
	if !strings.HasPrefix(repo, p) || len(commit) != 40 {
		return "", fmt.Errorf("cannot derive a tarball url from %q @ %q", repo, commit)
	}
	return "https://api.github.com/repos/" + strings.TrimPrefix(repo, p) + "/tarball/" + commit, nil
}

// plan lists everything a host must fetch, one tab-separated line each: kind, file, url, sha256, destination.
// Archives are extracted with their top directory stripped into <work>/<destination>; a "goinstall" line names a
// module version to build with go install -trimpath. tools/gen/regen.sh executes it.
func (p *Pins) plan(platform string) ([]string, error) {
	var out []string
	add := func(kind, file, url, sum, dest string) {
		out = append(out, strings.Join([]string{kind, file, url, sum, dest}, "\t"))
	}
	sdk, ok := p.Tools.WasiSDKArchives[platform]
	if !ok {
		return nil, fmt.Errorf("no wasi-sdk archive pinned for platform %q", platform)
	}
	add("tool", path.Base(sdk.URL), sdk.URL, sdk.SHA256, "sdk")
	if p.Tools.BinaryenArchives != nil {
		bin, ok := p.Tools.BinaryenArchives[platform]
		if !ok {
			return nil, fmt.Errorf("no binaryen archive pinned for platform %q", platform)
		}
		add("tool", path.Base(bin.URL), bin.URL, bin.SHA256, "binaryen")
	}
	add("goinstall", p.Tools.Wasm2goModule+"@"+p.Tools.Wasm2go, "-", "-", "-")
	rt := p.Runtime
	u, err := tarballURL(rt.Repo, rt.Commit)
	if err != nil {
		return nil, err
	}
	add("source", rt.Archive, u, rt.ArchiveSHA256, "src/"+rt.ExtractDir)
	wl := p.WasiLibc
	if u, err = tarballURL(wl.Repo, wl.Commit); err != nil {
		return nil, err
	}
	add("source", wl.Archive, u, wl.ArchiveSHA256, "src/"+wl.ExtractDir)
	seen := map[string]bool{}
	names := make([]string, 0, len(p.Grammars))
	for n := range p.Grammars {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		g := p.Grammars[n]
		if seen[g.Archive] { // typescript and tsx share one repository archive
			continue
		}
		seen[g.Archive] = true
		if u, err = tarballURL(g.Repo, g.Commit); err != nil {
			return nil, err
		}
		if g.ArchiveURL != "" {
			u = g.ArchiveURL
		}
		add("source", g.Archive, u, g.ArchiveSHA256, "src/"+g.ExtractDir)
	}
	return out, nil
}

// hostPlatform is the key of the per-platform archive tables.
func hostPlatform() string { return runtime.GOOS + "-" + runtime.GOARCH }

var (
	clangRx   = regexp.MustCompile(`^clang version ([0-9]+[.][0-9]+[.][0-9]+)[^ ]* [(][^ ]+ ([0-9a-f]{40})[)]`)
	wasmOptRx = regexp.MustCompile(`^wasm-opt version ([0-9]+)( |$)`)
)

// parseClangVersion reduces the first line of `clang --version` to the host-independent parts. wasi-sdk 34 reports
// "23.1.0-rc3" on Windows and "23.1.0-wasi-sdk" on Linux for the same LLVM commit; only the number and the commit count.
func parseClangVersion(line string) (version, commit string, err error) {
	m := clangRx.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return "", "", fmt.Errorf("unrecognised clang version line %q", line)
	}
	return m[1], m[2], nil
}

// parseWasmOptVersion returns the release number of `wasm-opt --version` (binaryen version_N).
func parseWasmOptVersion(line string) (string, error) {
	m := wasmOptRx.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return "", fmt.Errorf("unrecognised wasm-opt version line %q", line)
	}
	return m[1], nil
}
