package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

// goldenTree is the tools/gen treeSHA256 of {src/a.c: "A", src/b/c.h: "B"} under dirs ["src"]. tools/gen asserts the same
// constant (TestTreeSHA256MatchesPinbump), so the two implementations cannot drift apart silently.
const goldenTree = "70a1830b710d98abb6170dd3710480caffbe9ecd908b22ab968471a59a33a4ad"

var (
	oldC = strings.Repeat("a", 40)
	newC = strings.Repeat("b", 40)
	tagC = strings.Repeat("c", 40) // the annotated tag object
)

const fixture = `{
  "schema": 3,
  "tools": {
    "treeSitterCLI": "0.27.2"
  },
  "grammars": {
    "x": {
      "repo": "https://github.com/wotjr1649/tree-sitter-x",
      "commit": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "extractDir": "tree-sitter-x-aaaaaaaaaaaa",
      "archive": "tree-sitter-x-aaaaaaaaaaaa.tar.gz",
      "archiveSHA256": "1111111111111111111111111111111111111111111111111111111111111111",
      "grammarDir": "x/src",
      "dirs": [
        "x/src",
        "common"
      ],
      "treeSHA256": "2222222222222222222222222222222222222222222222222222222222222222",
      "files": {
        "parser.c": "3333333333333333333333333333333333333333333333333333333333333333",
        "scanner.c": "4444444444444444444444444444444444444444444444444444444444444444"
      },
      "licenseSHA256": "5555555555555555555555555555555555555555555555555555555555555555",
      "noticeSHA256": "6666666666666666666666666666666666666666666666666666666666666666",
      "cSymbol": "tree_sitter_x",
      "regenerate": true
    },
    "y": {
      "repo": "https://github.com/wotjr1649/tree-sitter-x",
      "commit": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "extractDir": "tree-sitter-x-aaaaaaaaaaaa",
      "archive": "tree-sitter-x-aaaaaaaaaaaa.tar.gz",
      "archiveSHA256": "1111111111111111111111111111111111111111111111111111111111111111",
      "grammarDir": "y/src",
      "dirs": [
        "y/src",
        "common"
      ],
      "treeSHA256": "7777777777777777777777777777777777777777777777777777777777777777",
      "files": {
        "parser.c": "8888888888888888888888888888888888888888888888888888888888888888"
      },
      "licenseSHA256": "5555555555555555555555555555555555555555555555555555555555555555",
      "noticeSHA256": "6666666666666666666666666666666666666666666666666666666666666666",
      "cSymbol": "tree_sitter_y",
      "regenerate": true
    },
    "z": {
      "repo": "https://github.com/wotjr1649/tree-sitter-z",
      "commit": "dddddddddddddddddddddddddddddddddddddddd",
      "extractDir": "tree-sitter-z-dddddddddddd",
      "archive": "tree-sitter-z-dddddddddddd.tar.gz",
      "archiveSHA256": "9999999999999999999999999999999999999999999999999999999999999999",
      "grammarDir": "src",
      "dirs": [
        "src"
      ],
      "treeSHA256": "2222222222222222222222222222222222222222222222222222222222222222",
      "files": {
        "parser.c": "3333333333333333333333333333333333333333333333333333333333333333"
      },
      "licenseSHA256": "5555555555555555555555555555555555555555555555555555555555555555",
      "cSymbol": "tree_sitter_z",
      "regenerate": true
    },
    "ext": {
      "repo": "https://github.com/tree-sitter-grammars/tree-sitter-ext",
      "commit": "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
      "extractDir": "ext",
      "archive": "ext.tar.gz",
      "archiveSHA256": "9999999999999999999999999999999999999999999999999999999999999999",
      "grammarDir": "src",
      "dirs": [
        "src"
      ],
      "treeSHA256": "2222222222222222222222222222222222222222222222222222222222222222",
      "files": {
        "parser.c": "3333333333333333333333333333333333333333333333333333333333333333"
      },
      "licenseSHA256": "5555555555555555555555555555555555555555555555555555555555555555",
      "cSymbol": "tree_sitter_ext",
      "regenerate": true
    },
    "patched": {
      "repo": "https://github.com/wotjr1649/tree-sitter-p",
      "commit": "ffffffffffffffffffffffffffffffffffffffff",
      "extractDir": "tree-sitter-p-ffffffffffff",
      "archive": "tree-sitter-p-ffffffffffff.tar.gz",
      "archiveSHA256": "9999999999999999999999999999999999999999999999999999999999999999",
      "grammarDir": "src",
      "dirs": [
        "src"
      ],
      "treeSHA256": "2222222222222222222222222222222222222222222222222222222222222222",
      "files": {
        "parser.c": "3333333333333333333333333333333333333333333333333333333333333333"
      },
      "patches": [],
      "licenseSHA256": "5555555555555555555555555555555555555555555555555555555555555555",
      "cSymbol": "tree_sitter_p",
      "regenerate": true
    }
  }
}
`

type ent struct {
	name string
	typ  byte
	body string
}

func tarball(top string, ents ...ent) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range ents {
		h := &tar.Header{Name: top + "/" + e.name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(e.body))}
		if e.typ != 0 {
			h.Typeflag = e.typ
		}
		if h.Typeflag == tar.TypeSymlink {
			h.Linkname, h.Size = e.body, 0
		}
		_ = tw.WriteHeader(h)
		if h.Size > 0 {
			_, _ = tw.Write([]byte(e.body))
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func goodTree() []ent {
	return []ent{
		{"x/src/parser.c", 0, "P"}, {"x/src/scanner.c", 0, "S"}, {"y/src/parser.c", 0, "YP"}, {"common/c.js", 0, "C"},
		{"LICENSE", 0, "L"}, {"NOTICE", 0, "N"}, {"CHANGELOG.md", 0, "## 2.0.0\n"}, {"README.md", 0, "ignored"},
	}
}

type fake struct {
	release, compare string // release: "", "prerelease" or "none"; compare: "" means ahead
	tarball          []byte
}

func serve(t *testing.T, f fake) config {
	t.Helper()
	if f.tarball == nil {
		f.tarball = tarball("t", goodTree()...)
	}
	mux := http.NewServeMux()
	js := func(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }
	p := "/repos/wotjr1649/tree-sitter-x/"
	mux.HandleFunc(p+"git/ref/tags/v2.0.0", func(w http.ResponseWriter, r *http.Request) {
		js(w, map[string]any{"object": map[string]string{"type": "tag", "sha": tagC}})
	})
	mux.HandleFunc(p+"git/tags/"+tagC, func(w http.ResponseWriter, r *http.Request) {
		js(w, map[string]any{"object": map[string]string{"type": "commit", "sha": newC}})
	})
	mux.HandleFunc(p+"releases/tags/v2.0.0", func(w http.ResponseWriter, r *http.Request) {
		switch f.release {
		case "none":
			http.NotFound(w, r)
		case "prerelease":
			js(w, map[string]any{"prerelease": true})
		default:
			js(w, map[string]any{"prerelease": false})
		}
	})
	mux.HandleFunc(p+"compare/"+oldC+"..."+newC, func(w http.ResponseWriter, r *http.Request) {
		st := f.compare
		if st == "" {
			st = "ahead"
		}
		js(w, map[string]string{"status": st})
	})
	mux.HandleFunc(p+"tarball/"+newC, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(f.tarball) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return config{Pins: []byte(fixture), Grammar: "x", Tag: "v2.0.0", API: srv.URL, Client: srv.Client()}
}

func TestTreeHashGolden(t *testing.T) {
	got, err := treeSHA256(map[string][]byte{"src/a.c": []byte("A"), "src/b/c.h": []byte("B"), "other/x": []byte("X")}, []string{"src"})
	if err != nil || got != goldenTree {
		t.Fatalf("tree hash = %s, %v; want %s", got, err, goldenTree)
	}
}

func TestBumpMovesTheWholeGroup(t *testing.T) {
	tb := tarball("t", goodTree()...)
	res, err := serve(t, fake{tarball: tb}).bump()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct{ Grammars map[string]pin }
	if err := json.Unmarshal(res.Pins, &doc); err != nil {
		t.Fatal(err)
	}
	wantTree, _ := treeSHA256(map[string][]byte{"x/src/parser.c": []byte("P"), "x/src/scanner.c": []byte("S"), "common/c.js": []byte("C")}, []string{"x/src", "common"})
	x, y := doc.Grammars["x"], doc.Grammars["y"]
	for n, g := range map[string]pin{"x": x, "y": y} {
		if g.Commit != newC || g.ExtractDir != "tree-sitter-x-bbbbbbbbbbbb" || g.Archive != "tree-sitter-x-bbbbbbbbbbbb.tar.gz" ||
			g.ArchiveSHA256 != sum(tb) || g.LicenseSHA256 != sum([]byte("L")) || g.NoticeSHA256 != sum([]byte("N")) {
			t.Errorf("%s: %+v", n, g)
		}
	}
	if x.TreeSHA256 != wantTree || x.Files["parser.c"] != sum([]byte("P")) || x.Files["scanner.c"] != sum([]byte("S")) || y.Files["parser.c"] != sum([]byte("YP")) {
		t.Errorf("hashes: %+v %+v", x, y)
	}
	if doc.Grammars["z"].Commit != strings.Repeat("d", 40) || doc.Grammars["ext"].Commit != strings.Repeat("e", 40) {
		t.Error("an unrelated entry moved")
	}
	// Layout untouched: same line count, and only x's 9 and y's 8 pin lines differ.
	ol, nl := strings.Split(fixture, "\n"), strings.Split(string(res.Pins), "\n")
	if len(ol) != len(nl) {
		t.Fatal("line count changed")
	}
	changed := 0
	for i := range ol {
		if ol[i] != nl[i] {
			changed++
		}
	}
	if changed != 9+8 {
		t.Errorf("changed lines = %d, want 17", changed)
	}
	var cand map[string]string
	if err := json.Unmarshal(res.Candidate, &cand); err != nil || !reflect.DeepEqual(cand, map[string]string{"Kind": "grammar", "Name": "x", "Tag": "v2.0.0", "Commit": newC}) {
		t.Errorf("candidate.json = %s, %v", res.Candidate, err)
	}
	for _, s := range []string{"`x`, `y`", "releases/tag/v2.0.0", "blob/v2.0.0/CHANGELOG.md", "compare/aaaaaaaaaaaa...bbbbbbbbbbbb", "ADR-0024 rule 7"} {
		if !strings.Contains(res.Summary, s) {
			t.Errorf("summary lacks %q", s)
		}
	}
	if strings.Contains(res.Summary, "WARNING") || strings.Contains(res.Summary, "PIN-MOVE-ALLOW") {
		t.Error("unexpected warning or marker in the summary")
	}
}

func TestBumpRefusals(t *testing.T) {
	without := func(name string) []ent {
		var o []ent
		for _, e := range goodTree() {
			if e.name != name {
				o = append(o, e)
			}
		}
		return o
	}
	with := func(e ent) []ent { return append(goodTree(), e) }
	var twoTops bytes.Buffer
	gz := gzip.NewWriter(&twoTops)
	tw := tar.NewWriter(gz)
	for _, n := range []string{"a/x", "b/y"} {
		_ = tw.WriteHeader(&tar.Header{Name: n, Typeflag: tar.TypeReg, Mode: 0o644, Size: 1})
		_, _ = tw.Write([]byte("x"))
	}
	_ = tw.Close()
	_ = gz.Close()
	for name, tc := range map[string]struct {
		f    fake
		mut  func(*config)
		want string
	}{
		"prerelease":        {fake{release: "prerelease"}, nil, "prerelease"},
		"behind":            {fake{compare: "behind"}, nil, "only moves forward"},
		"identical":         {fake{compare: "identical"}, nil, "only moves forward"},
		"non-owner":         {fake{}, func(c *config) { c.Grammar = "ext" }, "not an owner repository"},
		"entry with extras": {fake{}, func(c *config) { c.Grammar = "patched" }, "pins with extras"},
		"unknown grammar":   {fake{}, func(c *config) { c.Grammar = "nope" }, "not in pins.json"},
		"unstable tag":      {fake{}, func(c *config) { c.Tag = "v2.0.0-rc1" }, "unexpected form"},
		"odd name":          {fake{}, func(c *config) { c.Grammar = "x;y" }, "unexpected form"},
		"no changelog":      {fake{tarball: tarball("t", without("CHANGELOG.md")...)}, nil, "CHANGELOG"},
		"pinned file lost":  {fake{tarball: tarball("t", without("x/src/scanner.c")...)}, nil, "missing from the new tree"},
		"new scanner":       {fake{tarball: tarball("t", with(ent{"y/src/scanner.c", 0, "S"})...)}, nil, "scanner.c"},
		"notice vanished":   {fake{tarball: tarball("t", without("NOTICE")...)}, nil, "NOTICE"},
		"no license":        {fake{tarball: tarball("t", without("LICENSE")...)}, nil, "LICENSE"},
		"symlink in src":    {fake{tarball: tarball("t", with(ent{"x/src/l", tar.TypeSymlink, "/etc/passwd"})...)}, nil, "link or special"},
		"path escape":       {fake{tarball: tarball("t", with(ent{"../evil", 0, "x"})...)}, nil, "refused path"},
		"two top dirs":      {fake{tarball: twoTops.Bytes()}, nil, "top-level"},
		"not gzip":          {fake{tarball: []byte("not a gzip stream at all")}, nil, "gzip"},
	} {
		c := serve(t, tc.f)
		if tc.mut != nil {
			tc.mut(&c)
		}
		if _, err := c.bump(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
}

func TestTagWithoutReleaseIsAllowed(t *testing.T) {
	res, err := serve(t, fake{release: "none"}).bump()
	if err != nil || !strings.Contains(res.Summary, "tag without a GitHub Release") {
		t.Fatalf("err = %v", err)
	}
}

func TestChangelogWithoutTheVersionWarns(t *testing.T) {
	tr := goodTree()
	tr[6].body = "## 1.0.0\n"
	res, err := serve(t, fake{tarball: tarball("t", tr...)}).bump()
	if err != nil || !strings.Contains(res.Summary, "WARNING") {
		t.Fatalf("err = %v, summary %q", err, res.Summary)
	}
}

// The committed pins.json must be readable by this tool: the owner grammars follow the naming rule and carry no extras,
// typescript and tsx form one group, and a third-party grammar is refused.
func TestRealPinsOwnerEntries(t *testing.T) {
	b, err := os.ReadFile("../pins.json")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string][]string{"java": {"java"}, "javascript": {"javascript"}, "typescript": {"tsx", "typescript"}, "tsx": {"tsx", "typescript"}, "tsql": {"tsql"}} {
		g, err := groupOf(b, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var got []string
		for _, c := range g {
			got = append(got, c.Name)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: group %v, want %v", name, got, want)
		}
	}
	if _, err := groupOf(b, "c"); err == nil {
		t.Error("c (a third-party repository) must be refused")
	}
}
