// Package grammarpatch prepares the same recorded grammar changes for gen and native-ref.
package grammarpatch

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wotjr1649/go-treesitter/tools/bench/proc"
	"github.com/wotjr1649/go-treesitter/tools/gen/patches"
)

type Config struct {
	Regenerate                                          bool
	GenerateFrom                                        string
	Archive, ArchiveSHA256, PatchRoot, Work, GrammarDir string
	CLI, CLIArchive, CLIArchiveSHA256, CLIVersion       string
	NPMDependencies                                     []string
	NPMCache                                            string
	FetchNPM                                            bool
	Patches                                             []patches.Pin
}

// Provenance excludes host paths and binary hashes; per-platform archives are verified against pins.json.
type Provenance struct {
	Patches            []patches.Pin     `json:"patches"`
	NPM                []NPMInput        `json:"npm,omitempty"`
	CLI                string            `json:"cli,omitempty"`
	JSRuntime          string            `json:"jsRuntime,omitempty"`
	GenerateFrom       string            `json:"generateFrom,omitempty"`
	GenerateFromSHA256 string            `json:"generateFromSHA256,omitempty"`
	InputSHA256        string            `json:"inputSHA256"`
	Outputs            map[string]string `json:"outputs"`
	Deterministic      bool              `json:"deterministic"`
}

func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func fileHash(p string) (string, error) {
	b, err := os.ReadFile(p)
	return hash(b), err
}

func checkFile(p, want string) error {
	got, err := fileHash(p)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("%s: sha256 %s, pinned %s", p, got, want)
	}
	return nil
}

func local(name string) bool {
	return name != "." && filepath.IsLocal(name) && !strings.ContainsAny(name, "\\:\x00") && path.Clean(name) == name
}

// Prepare verifies inputs, patches grammar inputs, generates twice, then applies recorded C patches.
// It never modifies the caller's original extracted inputs.
func Prepare(c Config) (string, *Provenance, error) {
	if len(c.Patches) == 0 && !c.Regenerate {
		return "", nil, fmt.Errorf("no patches")
	}
	if !local(c.GrammarDir) {
		return "", nil, fmt.Errorf("invalid grammar directory %q", c.GrammarDir)
	}
	if err := checkFile(c.Archive, c.ArchiveSHA256); err != nil {
		return "", nil, err
	}
	if err := os.Mkdir(c.Work, 0o755); err != nil {
		return "", nil, err
	}
	root := filepath.Join(c.Work, "first")
	if err := extract(c.Archive, root); err != nil {
		return "", nil, err
	}
	if err := Validate(c.Patches, c.Regenerate, c.GenerateFrom); err != nil {
		return "", nil, err
	}
	var before, after []patches.Pin
	for _, p := range c.Patches {
		if c.Regenerate && (path.Ext(p.Source) == ".c" || path.Ext(p.Source) == ".h") {
			after = append(after, p)
		} else {
			before = append(before, p)
		}
	}
	if err := patches.Apply(c.PatchRoot, root, before); err != nil {
		return "", nil, err
	}
	npm, err := prepareNPM(c, root)
	if err != nil {
		return "", nil, err
	}
	inputs, err := snapshot(root)
	if err != nil {
		return "", nil, err
	}
	prov := &Provenance{Patches: c.Patches, NPM: npm, InputSHA256: treeHash(inputs)}
	if c.GenerateFrom != "" {
		prov.GenerateFrom = c.GenerateFrom
		prov.GenerateFromSHA256 = inputs[path.Join(c.GrammarDir, c.GenerateFrom)]
		if prov.GenerateFromSHA256 == "" {
			return "", nil, fmt.Errorf("generation input %s missing", c.GenerateFrom)
		}
	}
	if c.Regenerate {
		if err := verifyCLI(c); err != nil {
			return "", nil, err
		}
		second := filepath.Join(c.Work, "second")
		if err := os.CopyFS(second, os.DirFS(root)); err != nil {
			return "", nil, err
		}
		for _, d := range []string{root, second} {
			if err := generate(c, d); err != nil {
				return "", nil, err
			}
			if err := patches.ApplyGenerated(c.PatchRoot, d, after); err != nil {
				return "", nil, err
			}
		}
		a, err := snapshot(root)
		if err != nil {
			return "", nil, err
		}
		b, err := snapshot(second)
		if err != nil {
			return "", nil, err
		}
		if !maps.Equal(a, b) {
			return "", nil, fmt.Errorf("tree-sitter generate is not deterministic")
		}
		prov.CLI, prov.JSRuntime = c.CLIVersion, "native"
	}
	prov.Outputs, err = snapshot(filepath.Join(root, filepath.FromSlash(c.GrammarDir)))
	if err != nil {
		return "", nil, err
	}
	if _, ok := prov.Outputs["parser.c"]; !ok {
		return "", nil, fmt.Errorf("generated parser.c missing")
	}
	prov.Deterministic = true
	return root, prov, nil
}

// Validate requires regeneration for non-C grammar inputs, including imported JS/JSON sources.
func Validate(ps []patches.Pin, regenerate bool, generateFrom string) error {
	if generateFrom != "" && (generateFrom != "grammar.json" || !regenerate) {
		return fmt.Errorf("generateFrom must be grammar.json with regenerate true")
	}
	for _, p := range ps {
		ext := path.Ext(p.Source)
		if ext != ".c" && ext != ".h" && !regenerate {
			return fmt.Errorf("grammar source patch %q must set regenerate", p.Source)
		}
	}
	return nil
}

func verifyCLI(c Config) error {
	if c.CLIVersion == "" || c.CLI == "" || c.CLIArchive == "" {
		return fmt.Errorf("pinned tree-sitter CLI and archive are required")
	}
	if err := checkFile(c.CLIArchive, c.CLIArchiveSHA256); err != nil {
		return err
	}
	f, err := os.Open(c.CLIArchive)
	if err != nil {
		return err
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer z.Close()
	h := sha256.New()
	if _, err := io.Copy(h, z); err != nil {
		return err
	}
	if err := checkFile(c.CLI, hex.EncodeToString(h.Sum(nil))); err != nil {
		return fmt.Errorf("CLI differs from pinned archive: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := proc.CommandContext(ctx, c.CLI, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("tree-sitter --version: %w: %s", err, out)
	}
	if fields := strings.Fields(string(out)); len(fields) < 2 || fields[0] != "tree-sitter" || fields[1] != c.CLIVersion {
		return fmt.Errorf("CLI version %q, pinned %s", out, c.CLIVersion)
	}
	return nil
}

func generate(c Config, root string) error {
	dir := filepath.Join(root, filepath.Dir(filepath.FromSlash(c.GrammarDir)))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := proc.CommandContext(ctx, c.CLI, "generate", "--abi", "15", "--js-runtime", "native")
	if c.GenerateFrom != "" {
		cmd.Args = append(cmd.Args, filepath.Join(filepath.Base(c.GrammarDir), c.GenerateFrom))
	}
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tree-sitter generate: %w: %s", err, out)
	}
	return nil
}

func snapshot(root string) (map[string]string, error) {
	m := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s: not a regular file", p)
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		sum, err := fileHash(p)
		if err == nil {
			m[filepath.ToSlash(rel)] = sum
		}
		return err
	})
	return m, err
}

func treeHash(m map[string]string) string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "%s\x00%s\n", n, m[n])
	}
	return hash([]byte(b.String()))
}

func extract(archive, root string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer z.Close()
	t := tar.NewReader(z)
	for {
		h, err := t.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		_, rel, ok := strings.Cut(h.Name, "/")
		if !ok || rel == "" {
			continue
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		if !local(rel) {
			return fmt.Errorf("unsafe archive entry %q", h.Name)
		}
		if h.Typeflag != tar.TypeReg {
			return fmt.Errorf("unsupported archive entry %q", h.Name)
		}
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		w, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, t)
		closeErr := w.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
}
