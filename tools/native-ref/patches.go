package main

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"

	"github.com/wotjr1649/go-treesitter/tools/gen/grammarpatch"
	"github.com/wotjr1649/go-treesitter/tools/gen/patches"
)

func prepareGrammarPatches(pinsFile string, p pins, g source, dl, work, lang string, fetch bool) (string, error) {
	if err := grammarpatch.Validate(g.Patches, g.Regenerate); err != nil {
		return "", err
	}
	if !g.Regenerate {
		root := filepath.Join(work, g.ExtractDir)
		return root, patches.Apply(filepath.Dir(pinsFile), root, g.Patches)
	}
	c := grammarpatch.Config{Archive: filepath.Join(dl, g.Archive), ArchiveSHA256: g.ArchiveSHA256,
		PatchRoot: filepath.Dir(pinsFile), Work: filepath.Join(work, "patches-"+lang), GrammarDir: g.GrammarDir, Patches: g.Patches, Regenerate: true,
		NPMDependencies: g.NPMDependencies, FetchNPM: fetch, NPMCache: filepath.Join(dl, "npm")}
	if g.Regenerate {
		cli, ok := p.Tools.TreeSitterCLIArchives[runtime.GOOS+"-"+runtime.GOARCH]
		if !ok {
			return "", fmt.Errorf("no tree-sitter CLI archive for this platform")
		}
		arc := filepath.Join(dl, path.Base(cli.URL))
		if _, err := os.Stat(arc); errors.Is(err, os.ErrNotExist) && fetch {
			if err := downloadVerified(cli.URL, arc, cli.SHA256, p.Mirrors); err != nil {
				return "", err
			}
		}
		if sum, err := fileSHA(arc); err != nil || sum != cli.SHA256 {
			return "", fmt.Errorf("CLI archive sha256 %s, pinned %s (%v)", sum, cli.SHA256, err)
		}
		bin := filepath.Join(work, "tree-sitter")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		if _, err := os.Stat(bin); errors.Is(err, os.ErrNotExist) {
			if err := unpackCLI(arc, bin); err != nil {
				return "", err
			}
		}
		var err error
		c.CLI, err = filepath.Abs(bin)
		if err != nil {
			return "", err
		}
		c.CLIArchive, c.CLIArchiveSHA256, c.CLIVersion = arc, cli.SHA256, p.Tools.TreeSitterCLI
	}
	root, prov, err := grammarpatch.Prepare(c)
	if err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(prov, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(c.Work, "provenance.json"), append(b, '\n'), 0o644); err != nil {
		return "", err
	}
	return root, nil
}

func unpackCLI(archive, bin string) error {
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
	w, err := os.OpenFile(bin, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, z)
	closeErr := w.Close()
	if err != nil {
		return err
	}
	return closeErr
}
