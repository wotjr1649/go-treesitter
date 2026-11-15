package main

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"

	"github.com/wotjr1649/go-treesitter/tools/gen/grammarpatch"
)

func patchStatus(name string, count, threshold int) string {
	s := fmt.Sprintf("%s patches=%d alert-threshold=%d", name, count, threshold)
	if count > threshold {
		s += " WARNING: review temporary patch burden"
	}
	return s
}

func printPatchStatus(p *Pins, threshold int) error {
	names := make([]string, 0, len(p.Grammars))
	for n := range p.Grammars {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Println(patchStatus(n, len(p.Grammars[n].Patches), threshold))
	}
	return nil
}

func preparePatches(o *opts, p *Pins, g GrammarPin, work string, id *Identity) (*grammarpatch.Provenance, error) {
	if len(g.Patches) == 0 && !g.Regenerate {
		return nil, nil
	}
	if err := grammarpatch.Validate(g.Patches, g.Regenerate); err != nil {
		return nil, err
	}
	if !g.Regenerate {
		root, err := patchedGrammar(*o, g, work, id)
		o.grammarSrc = root
		return nil, err
	}
	if o.dl == "" {
		return nil, fmt.Errorf("-dl is required for grammar regeneration")
	}
	cli := p.Tools.TreeSitterCLIArchives[hostPlatform()]
	bin := o.treeSitter
	if bin != "" {
		var err error
		bin, err = filepath.Abs(bin)
		if err != nil {
			return nil, err
		}
	}
	c := grammarpatch.Config{Archive: filepath.Join(o.dl, g.Archive), ArchiveSHA256: g.ArchiveSHA256,
		PatchRoot: o.root, Work: filepath.Join(work, "patches"), GrammarDir: g.GrammarDir, Patches: g.Patches, Regenerate: true,
		CLI: bin, CLIArchive: filepath.Join(o.dl, path.Base(cli.URL)), CLIArchiveSHA256: cli.SHA256, CLIVersion: p.Tools.TreeSitterCLI}
	root, prov, err := grammarpatch.Prepare(c)
	if err != nil {
		return nil, err
	}
	o.grammarSrc = root
	return prov, nil
}

func bindPatches(id *Identity, p *grammarpatch.Provenance) {
	id.SourceSHA256["patches.inputs"] = p.InputSHA256
	for i, patch := range p.Patches {
		id.SourceSHA256[fmt.Sprintf("patches.%d", i+1)] = patch.SHA256
	}
	for name, sum := range p.Outputs {
		id.SourceSHA256["patched.grammar."+name] = sum
	}
	if p.CLI != "" {
		id.Tools["tree-sitter-cli"], id.Tools["js-runtime"] = p.CLI, p.JSRuntime
	}
	b, _ := marshal(p)
	id.RecipeSHA256 = sumBytes(append([]byte(id.RecipeSHA256+"\ngrammar-patches="), b...))
}
