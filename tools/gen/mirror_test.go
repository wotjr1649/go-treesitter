package main

import (
	"strings"
	"testing"
)

func TestMirrorAssets(t *testing.T) {
	p, err := loadPins("pins.json")
	if err != nil {
		t.Fatal(err)
	}
	assets, err := p.mirrorAssets()
	if err != nil {
		t.Fatal(err)
	}
	grammars := map[string]bool{}
	for _, g := range p.Grammars {
		grammars[g.Archive] = true
	}
	wantAssets := 2 + len(grammars) + len(p.Tools.WasiSDKArchives) + len(p.Tools.BinaryenArchives) + len(p.Tools.TreeSitterCLIArchives)
	if len(assets) != wantAssets {
		t.Fatalf("release assets = %d, want %d", len(assets), wantAssets)
	}
	seen := map[string]bool{}
	for _, line := range assets {
		f := strings.Split(line, "\t")
		if len(f) != 3 || len(f[1]) != 64 || seen[f[0]] {
			t.Fatalf("invalid asset %q", line)
		}
		seen[f[0]] = true
	}
	p.Mirrors = []string{"https://mirror.example/v1", "https://mirror.example/v2/"}
	lines, err := p.plan("windows-amd64")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		f := strings.Split(line, "\t")
		if f[0] == "goinstall" {
			continue
		}
		want := "https://mirror.example/v1/" + f[1] + ",https://mirror.example/v2/" + f[1]
		if f[5] != want {
			t.Fatalf("fallback URLs = %q, want %q", f[5], want)
		}
	}
}

func TestMirrorBases(t *testing.T) {
	for _, base := range []string{"http://mirror.example", "https://user:pass@mirror.example/v1", "https://mirror.example/x,y", "https://mirror.example/?token=secret", "https://mirror.example/#frag", "https://mirror.example/\n"} {
		if validateMirrors([]string{base}) == nil {
			t.Errorf("accepted %q", base)
		}
	}
	if err := validateMirrors([]string{"https://github.com/owner/repo/releases/download/v1.2.3"}); err != nil {
		t.Fatal(err)
	}
}
