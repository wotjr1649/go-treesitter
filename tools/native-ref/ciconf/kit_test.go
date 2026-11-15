package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKitProjection(t *testing.T) {
	for _, tt := range []struct {
		old, find, replace, want string
		start, oe, ne            int
	}{
		{"a a", "a", "b", "b a", 0, 1, 1},
		{"é\r\nα", "α", "β\n", "é\r\nβ\n", 4, 6, 7},
		{"é", "", "!", "é!", 2, 2, 3},
		{"abc", "b", "", "ac", 1, 2, 1},
	} {
		nw, start, oe, ne, err := projectKitEdit([]byte(tt.old), tt.find, tt.replace)
		if err != nil || string(nw) != tt.want || start != tt.start || oe != tt.oe || ne != tt.ne {
			t.Fatalf("%+v: got %q %d %d %d: %v", tt, nw, start, oe, ne, err)
		}
	}
	if _, _, _, _, err := projectKitEdit([]byte("x"), "absent", ""); err == nil {
		t.Fatal("accepted absent target")
	}
}

func TestKitManifest(t *testing.T) {
	dir := filepath.Join("..", "..", "conformance", "testdata", "kit")
	inputs, edits, identity, err := loadKit(dir, []string{"csharp", "go", "json", "python", "tsql", "tsx", "typescript"})
	if err != nil || len(inputs) == 0 || len(edits) == 0 || len(identity) != 64 {
		t.Fatalf("load: %d %d %s %v", len(inputs), len(edits), identity, err)
	}
	seen := map[string]bool{}
	for _, j := range inputs {
		if seen[j.id] {
			t.Fatalf("duplicate input: %s", j.id)
		}
		seen[j.id] = true
	}
	for _, e := range edits {
		got := append(append(append([]byte{}, e.old[:e.start]...), e.new[e.start:e.ne]...), e.old[e.oe:]...)
		if !bytes.Equal(got, e.new) {
			t.Fatal("invalid edit offsets")
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"ok", "unknown-field", "duplicate", "route", "source-hash", "preceding", "projection", "missing-target", "notice", "budget"} {
		t.Run(mode, func(t *testing.T) {
			var m kitManifest
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatal(err)
			}
			for _, c := range m.Cases {
				if len(c.Edits) >= 2 {
					m.Cases = m.Cases[:1]
					m.Cases[0] = c
					break
				}
			}
			c := &m.Cases[0]
			dst := t.TempDir()
			write := func(path string, b []byte) {
				t.Helper()
				if err := writeFile(filepath.Join(dst, path), b); err != nil {
					t.Fatal(err)
				}
			}
			write("LICENSE", []byte(m.License))
			hashes := []string{c.SourceSHA256}
			for _, e := range c.Edits {
				hashes = append(hashes, e.NewSHA256)
			}
			for _, hash := range hashes {
				b, err := os.ReadFile(filepath.Join(dir, "sources", hash+".in"))
				if err != nil {
					t.Fatal(err)
				}
				write("sources/"+hash+".in", b)
			}
			switch mode {
			case "duplicate":
				m.Cases = append(m.Cases, m.Cases[0])
			case "route":
				c.Lang = "n461"
			case "source-hash":
				write("sources/"+c.SourceSHA256+".in", []byte("corrupt"))
			case "preceding":
				c.Edits[1].OldSHA256 = c.Edits[0].OldSHA256
			case "projection":
				c.Edits[0].Replace += "unexpected"
			case "missing-target":
				c.Edits[0].Find = "missing target 9c6837"
			case "notice":
				write("LICENSE", []byte("missing notice"))
			case "budget":
				b := bytes.Repeat([]byte("x"), 262145)
				c.SourceSHA256 = sha(b)
				c.Edits = nil
				write("sources/"+sha(b)+".in", b)
			}
			b, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "unknown-field" {
				b = append([]byte(`{"expect":[],`), b[1:]...)
			}
			write("manifest.json", b)
			_, es, _, err := loadKit(dst, []string{c.Lang})
			if mode == "ok" {
				if err != nil || len(es) < 2 || !bytes.Equal(es[0].new, es[1].old) {
					t.Fatalf("pair chain not preserved: %v", err)
				}
			} else if err == nil {
				t.Fatalf("accepted %s", mode)
			}
		})
	}
	js, es, _, err := loadKit(dir, []string{"bash"})
	if err != nil || len(js) != 0 || len(es) != 0 {
		t.Fatalf("unaffected shard: %v", err)
	}
}

func TestKitInputsOnly(t *testing.T) {
	dir := t.TempDir()
	pins, corpus, out := filepath.Join(dir, "pins.json"), filepath.Join(dir, "corpus"), filepath.Join(dir, "data")
	if err := writeFile(pins, []byte(`{"grammars":{"json":{"extractDir":"json"}}}`)); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(corpus, "json", "test", "corpus", "sample.txt"), []byte("===\nfixture\n===\n{}\n---\n(document)\n")); err != nil {
		t.Fatal(err)
	}
	kit := filepath.Join("..", "..", "conformance", "testdata", "kit")
	if err := run(pins, corpus, "", out, 1, 1, "json", true, kit); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "inputs.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		Inputs            int
		KitManifestSHA256 string
	}
	if err := json.Unmarshal(raw, &meta); err != nil || meta.Inputs != 34 || len(meta.KitManifestSHA256) != 64 {
		t.Fatalf("nightly input identity: %s, %v", raw, err)
	}
	for _, name := range []string{"native", "edits", "ci.json"} {
		if _, err := os.Stat(filepath.Join(out, name)); !os.IsNotExist(err) {
			t.Fatalf("inputs-only wrote %s: %v", name, err)
		}
	}
}

func TestKitRejectsEmptyFlagAndStrayFile(t *testing.T) {
	dir := t.TempDir()
	pins, corpus := filepath.Join(dir, "pins.json"), filepath.Join(dir, "corpus")
	if err := writeFile(pins, []byte(`{"grammars":{"json":{"extractDir":"json"}}}`)); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(corpus, "json", "test", "corpus", "sample.txt"), []byte("===\nfixture\n===\n{}\n---\n(document)\n")); err != nil {
		t.Fatal(err)
	}
	if err := run(pins, corpus, "", filepath.Join(dir, "out1"), 1, 1, "json", true, ""); err == nil || !strings.Contains(err.Error(), "empty directory") {
		t.Fatalf("empty -kit value: %v", err)
	}
	kit := filepath.Join(dir, "kit")
	if err := os.CopyFS(kit, os.DirFS(filepath.Join("..", "..", "conformance", "testdata", "kit"))); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := loadKit(kit, []string{"json"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kit, "sources", strings.Repeat("0", 64)+".in"), []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := loadKit(kit, []string{"json"}); err == nil || !strings.Contains(err.Error(), "does not reference") {
		t.Fatalf("stray source file accepted: %v", err)
	}
}
