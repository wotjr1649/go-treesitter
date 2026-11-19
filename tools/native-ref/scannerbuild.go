package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wotjr1649/go-treesitter/tools/bench/proc"
	"github.com/wotjr1649/go-treesitter/tools/gen/patches"
)

type scannerComparison struct {
	Patch               string `json:"patch"`
	Before              string `json:"before"`
	After               string `json:"after"`
	BeforeSHA256        string `json:"beforeSHA256"`
	AfterSHA256         string `json:"afterSHA256"`
	BeforeSourcesSHA256 string `json:"beforeSourcesSHA256"`
	AfterSourcesSHA256  string `json:"afterSourcesSHA256"`
}

type scannerStage struct {
	exe, exeSHA, sourcesSHA string
	files                   map[string]string
}

type scannerBuild struct {
	Grammar     string              `json:"grammar"`
	Identity    map[string]any      `json:"identity"`
	Comparisons []scannerComparison `json:"comparisons"`
	Platform    string              `json:"platform"`
	Compiler    string              `json:"compiler"`

	dir    string
	final  int
	pairs  []int
	stages map[int]scannerStage
}

func newScannerBuild(dir, lang, cc, opt, here string, p pins, g source) (*scannerBuild, error) {
	if dir == "" {
		return nil, nil
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	postGenerate := referenceCPatches(g.Patches)
	s := &scannerBuild{
		dir: dir, Grammar: lang, final: len(postGenerate),
		Platform: runtime.GOOS + "/" + runtime.GOARCH, Comparisons: []scannerComparison{},
		stages: map[int]scannerStage{},
	}
	if err := s.selectStages(g, postGenerate); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if len(s.Comparisons) == 0 {
		return s, nil
	}
	version, err := proc.Command(cc, "--version").Output()
	if err != nil {
		return nil, fmt.Errorf("%s: C compiler version: %w", lang, err)
	}
	s.Compiler = strings.TrimSpace(strings.SplitN(string(version), "\n", 2)[0])
	driver, _, err := sourceDigest(here, func(name string) bool {
		return name == "native_ref.c" || strings.HasPrefix(name, "shim/") || strings.HasPrefix(name, "ctype/")
	})
	if err != nil {
		return nil, err
	}
	cli := map[string]string{}
	for platform, archive := range p.Tools.TreeSitterCLIArchives {
		cli[platform] = archive.SHA256
	}
	s.Identity = map[string]any{
		"grammarCommit": g.Commit, "grammarArchiveSHA256": g.ArchiveSHA256,
		"runtimeCommit": p.Runtime.Commit, "runtimeArchiveSHA256": p.Runtime.ArchiveSHA256,
		"wasiLibcCommit": p.WasiLibc.Commit, "wasiLibcArchiveSHA256": p.WasiLibc.ArchiveSHA256,
		"cli": p.Tools.TreeSitterCLI, "cliArchivesSHA256": cli,
		"grammarDir": g.GrammarDir, "cSymbol": g.CSymbol, "generateFrom": g.GenerateFrom, "npmDependencies": g.NPMDependencies,
		"patches": g.Patches, "nativeDriverSourcesSHA256": driver,
		"compileFlags": []string{"-std=c11", "-fsigned-char", "-DNDEBUG", opt},
	}
	return s, nil
}

func (s *scannerBuild) selectStages(g source, postGenerate []patches.Pin) error {
	for i, patch := range postGenerate {
		if path.Base(patch.Source) != "scanner.c" {
			continue
		}
		if !g.Regenerate || patch.Source != path.Join(g.GrammarDir, "scanner.c") || g.Files["scanner.c"] == "" {
			return fmt.Errorf("%s: scanner patch %s does not target the regenerated scanner used for compilation", s.Grammar, patch.File)
		}
		s.Comparisons = append(s.Comparisons, scannerComparison{Patch: patch.File})
		s.pairs = append(s.pairs, i)
		s.stages[i], s.stages[i+1] = scannerStage{}, scannerStage{}
	}
	return nil
}

func (s *scannerBuild) capture(i int, dir string, build func(string, string) error) error {
	if _, wanted := s.stages[i]; !wanted {
		return nil
	}
	exe := nativeExePath(s.dir, fmt.Sprintf("%s-stage-%d", s.Grammar, i))
	if err := build(dir, exe); err != nil {
		return err
	}
	return s.record(i, dir, exe)
}

func (s *scannerBuild) record(i int, dir, exe string) error {
	abs, err := filepath.Abs(exe)
	if err != nil {
		return err
	}
	hash, err := fileSHA(abs)
	if err != nil {
		return err
	}
	sources, files, err := sourceDigest(dir, func(string) bool { return true })
	if err != nil {
		return err
	}
	s.stages[i] = scannerStage{abs, hash, sources, files}
	return nil
}

func (s *scannerBuild) finish(dir, exe string) error {
	if _, wanted := s.stages[s.final]; wanted {
		if err := s.record(s.final, dir, exe); err != nil {
			return err
		}
	}
	for j, i := range s.pairs {
		before, after := s.stages[i], s.stages[i+1]
		if before.exe == "" || after.exe == "" || before.sourcesSHA == after.sourcesSHA {
			return fmt.Errorf("%s: missing or unchanged scanner build stages for %s", s.Grammar, s.Comparisons[j].Patch)
		}
		// Only the target scanner may differ: parser, headers and every other prepared file stay identical.
		for name := range before.files {
			if before.files[name] != after.files[name] && name != "scanner.c" {
				return fmt.Errorf("%s: %s also changes %s", s.Grammar, s.Comparisons[j].Patch, name)
			}
		}
		if len(before.files) != len(after.files) {
			return fmt.Errorf("%s: %s changes the prepared file set", s.Grammar, s.Comparisons[j].Patch)
		}
		c := &s.Comparisons[j]
		c.Before, c.After = before.exe, after.exe
		c.BeforeSHA256, c.AfterSHA256 = before.exeSHA, after.exeSHA
		c.BeforeSourcesSHA256, c.AfterSourcesSHA256 = before.sourcesSHA, after.sourcesSHA
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, s.Grammar+".json"), append(data, '\n'), 0o644)
}

func sourceDigest(dir string, include func(string) bool) (string, map[string]string, error) {
	files := map[string]string{}
	err := fs.WalkDir(os.DirFS(dir), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !include(name) {
			return err
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unexpected native source type: %s/%s", dir, name)
		}
		hash, err := fileSHA(filepath.Join(dir, filepath.FromSlash(name)))
		files[name] = hash
		return err
	})
	if err != nil {
		return "", nil, err
	}
	if len(files) == 0 {
		return "", nil, fmt.Errorf("no native sources at %s", dir)
	}
	data, err := json.Marshal(files)
	if err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), files, nil
}
