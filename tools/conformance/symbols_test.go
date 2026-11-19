package conformance

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/tools/bench/proc"
)

type symbolLookup struct {
	name  string
	named bool
}

type symbolMismatch struct {
	symbolLookup
	resolved, node treesitter.Symbol
}

func TestCorpusSymbols(t *testing.T) {
	root := dataRoot(t)
	rows := readCSV(t, filepath.Join(root, "corpus", "manifest.csv"))
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			p := newParser(t, lang)
			l := languages[lang]()
			lookup := map[symbolLookup]treesitter.Symbol{}
			mismatches := map[symbolMismatch]string{}
			var files, nodes int
			for _, row := range rows {
				if row[1] != lang {
					continue
				}
				files++
				id := row[0]
				src := readFile(t, corpusFile(t, root, lang, filepath.Base(id)))
				if sha(src) != row[4] {
					t.Fatalf("%s: input sha256 differs from the manifest", id)
				}
				snap, err := p.ParseSnapshot(bg, src)
				if err != nil {
					t.Fatalf("%s: ParseSnapshot: %v", id, err)
				}
				nodes += snap.Len()
				x := snap.Index()
				indexed := make([]bool, snap.Len())
				seen := map[treesitter.Symbol]bool{}
				for n := range snap.All() {
					key := symbolLookup{n.Kind(), n.IsNamed()}
					s, cached := lookup[key]
					if !cached {
						var ok bool
						s, ok = l.SymbolForName(key.name, key.named)
						if !ok {
							t.Fatalf("%s: no public symbol for %q (named=%t), node ID %d", id, key.name, key.named, n.Symbol())
						}
						lookup[key] = s
					}
					if s != n.Symbol() {
						mismatch := symbolMismatch{key, s, n.Symbol()}
						if _, seen := mismatches[mismatch]; !seen {
							mismatches[mismatch] = id
						}
						continue
					}
					if !seen[s] {
						seen[s] = true
						for indexedNode := range x.OfSymbol(s) {
							if indexedNode.Symbol() != s {
								t.Fatalf("%s: OfSymbol(%d) returned node %d with symbol %d", id, s, indexedNode.Index(), indexedNode.Symbol())
							}
							indexed[indexedNode.Index()] = true
						}
					}
					if !indexed[n.Index()] {
						t.Fatalf("%s: OfSymbol(%d) omitted node %d (%q)", id, s, n.Index(), key.name)
					}
				}
			}
			if files == 0 {
				t.Fatal("no pinned corpus inputs")
			}
			for c, id := range mismatches {
				t.Errorf("%s: %q (named=%t) resolves to %d, node ID %d", id, c.name, c.named, c.resolved, c.node)
			}
			summary(t, map[string]any{"lang": lang, "files": files, "nodes": nodes, "names": len(lookup)})
		})
	}
}

func TestSymbolLookupNative(t *testing.T) {
	if os.Getenv("TSG_NATIVE_EXE_DIR") == "" {
		t.Skip("TSG_NATIVE_EXE_DIR is required")
	}
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			exe, ok := nativeExe(lang)
			if !ok {
				t.Fatalf("no native executable for %s", lang)
			}
			out, err := proc.Command(exe, "symbols").Output()
			if err != nil {
				t.Fatal(err)
			}
			rows := bufio.NewScanner(bytes.NewReader(out))
			if !rows.Scan() || rows.Text() != "#tsg-symbols/1" {
				t.Fatal("invalid native symbol header")
			}
			l := languages[lang]()
			seen := map[symbolLookup]bool{}
			aliases := 0
			for rows.Scan() {
				row := strings.Split(rows.Text(), "\t")
				if len(row) != 5 || row[3] != "0" && row[3] != "1" {
					t.Fatalf("invalid native symbol row: %q", rows.Text())
				}
				raw, rawErr := strconv.ParseUint(row[0], 10, 16)
				_, publicErr := strconv.ParseUint(row[1], 10, 16)
				want, wantErr := strconv.ParseUint(row[2], 10, 16)
				name, nameErr := hex.DecodeString(row[4])
				if rawErr != nil || publicErr != nil || wantErr != nil || nameErr != nil || int(raw) >= l.SymbolCount() {
					t.Fatalf("invalid native symbol row: %q", rows.Text())
				}
				k := symbolLookup{string(name), row[3] == "1"}
				if got, ok := l.SymbolForName(k.name, k.named); !ok || got != treesitter.Symbol(want) {
					t.Errorf("SymbolForName(%q, %t) = %d,%t, native = %d (raw ID %d)", k.name, k.named, got, ok, want, raw)
				}
				if got := l.SymbolName(treesitter.Symbol(raw)); got != k.name {
					t.Errorf("SymbolName(%d) = %q, native name = %q", raw, got, k.name)
				}
				if !seen[k] {
					seen[k] = true
					if raw != want {
						aliases++
					}
				}
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if len(seen) == 0 {
				t.Fatal("native symbol output has no rows")
			}
			summary(t, map[string]any{"lang": lang, "names": len(seen), "aliases": aliases})
		})
	}
}
