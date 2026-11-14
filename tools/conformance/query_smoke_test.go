package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	treesitter "github.com/wotjr1649/go-treesitter"
)

// TestQuerySmoke compiles each pinned grammar repository's own query files and runs three corpus inputs.
// This is a smoke check of the Go API and capture mapping, not a native query differential.
func TestQuerySmoke(t *testing.T) {
	root := dataRoot(t)
	var pins struct {
		Grammars map[string]struct {
			ExtractDir string `json:"extractDir"`
		} `json:"grammars"`
	}
	if err := json.Unmarshal(readFile(t, filepath.Join("..", "gen", "pins.json")), &pins); err != nil {
		t.Fatal(err)
	}
	rows := readCSV(t, filepath.Join(root, "corpus", "manifest.csv"))
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			p := newParser(t, lang)
			language := languages[lang]()
			pinName := lang
			if pinName == "go" {
				pinName = "golang"
			}
			files, err := filepath.Glob(filepath.Join(queryDirFor(root), pins.Grammars[pinName].ExtractDir, "queries", "*.scm"))
			if err != nil {
				t.Fatal(err)
			}
			if len(files) == 0 {
				t.Fatal("no pinned upstream queries found")
			}
			queries := make([]*treesitter.Query, 0, len(files)+1)
			for _, file := range files {
				pattern := readFile(t, file)
				q, err := treesitter.NewQuery(language, string(pattern))
				if err != nil {
					t.Fatalf("%s: %v", file, err)
				}
				queries = append(queries, q)
				t.Logf("query %s sha256=%s patterns=%d", filepath.Base(file), sha(pattern), q.PatternCount())
			}
			all, err := treesitter.NewQuery(language, "(_) @node")
			if err != nil {
				t.Fatal(err)
			}
			queries = append(queries, all)
			inputs, matches, captures := 0, 0, 0
			for _, r := range rows {
				if r[1] != lang {
					continue
				}
				if inputs == 3 {
					break
				}
				inputs++
				src := readFile(t, corpusFile(t, root, lang, filepath.Base(r[0])))
				if sha(src) != r[4] {
					t.Fatal("corpus identity drift", r[0])
				}
				tree, err := p.Parse(bg, src, nil)
				if err != nil {
					t.Fatal(err)
				}
				snap, err := tree.Snapshot(bg)
				if err != nil {
					tree.Close()
					t.Fatal(err)
				}
				for qi, q := range queries {
					for m, err := range tree.Matches(bg, q, nil) {
						if err != nil {
							tree.Close()
							t.Fatalf("query %d input %s: %v", qi, r[0], err)
						}
						matches++
						for _, capture := range m.Captures {
							captures++
							node := capture.Node
							if node.IsZero() || node != snap.Node(node.Index()) || int(node.EndByte()) > len(src) {
								tree.Close()
								t.Fatal("invalid snapshot capture")
							}
						}
					}
				}
				tree.Close()
			}
			if inputs != 3 || captures == 0 {
				t.Fatalf("insufficient smoke coverage: inputs=%d captures=%d", inputs, captures)
			}
			summary(t, map[string]any{"lang": lang, "queryFiles": len(files), "inputs": inputs, "matches": matches, "captures": captures})
		})
	}
}

// FuzzQueryCompile exercises the real C query compiler when the conformance inputs are available.
func FuzzQueryCompile(f *testing.F) {
	if os.Getenv("TSG_S15_DIR") == "" && os.Getenv("TSG_DATA_DIR") == "" {
		f.Skip("no conformance inputs selected")
	}
	language := languages["json"]
	if language == nil {
		f.Skip("json is not linked")
	}
	for _, s := range []string{"", "(_) @node", "((string) @s (#match? @s \"^x\"))", "(", "\xff"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, source string) {
		_, err := treesitter.NewQuery(language(), source)
		if qe, ok := err.(*treesitter.QueryError); ok && uint64(qe.Offset) > uint64(len(source)) {
			t.Fatalf("invalid query offset: %v", qe)
		}
	})
}

func TestQueryEOFRange(t *testing.T) {
	language := languages["json"]
	if language == nil {
		t.Skip("json not linked")
	}
	p := newParser(t, "json")
	tree, err := p.Parse(bg, []byte("[1"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()
	q, err := treesitter.NewQuery(language(), "(MISSING) @m")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		opts *treesitter.QueryOptions
		want int
	}{
		{"default", nil, 1}, {"zero", &treesitter.QueryOptions{}, 1},
		{"start at EOF", &treesitter.QueryOptions{StartByte: 2}, 0}, // C also prunes the nonempty root ending here.
		{"explicit end at EOF", &treesitter.QueryOptions{EndByte: 2}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count := 0
			for m, err := range tree.Matches(bg, q, tc.opts) {
				if err != nil {
					t.Fatal(err)
				}
				count++
				if n := m.Captures[0].Node; !n.IsMissing() || n.StartByte() != 2 || n.EndByte() != 2 {
					t.Fatal(n)
				}
			}
			if count != tc.want {
				t.Fatalf("got %d matches, want %d", count, tc.want)
			}
		})
	}
}

func TestQueryWideArray(t *testing.T) {
	language := languages["json"]
	if language == nil {
		t.Skip("json not linked")
	}
	const children = 16000
	p := newParser(t, "json")
	src := []byte("[" + strings.Repeat("1,", children-1) + "1]")
	tree, err := p.Parse(bg, src, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()
	q, err := treesitter.NewQuery(language(), "(array (_) @a)")
	if err != nil {
		t.Fatal(err)
	}
	// 500ms leaves substantial headroom for linear mapping while rejecting the reviewed 28.8s quadratic path.
	const bound = 500 * time.Millisecond
	start := time.Now()
	count, previous := 0, -1
	for m, err := range tree.Matches(bg, q, nil) {
		if err != nil {
			t.Fatal(err)
		}
		if len(m.Captures) != 1 || m.Captures[0].Node.Kind() != "number" || m.Captures[0].Node.Index() <= previous {
			t.Fatal("bad capture", m)
		}
		previous = m.Captures[0].Node.Index()
		count++
	}
	elapsed := time.Since(start)
	t.Logf("%d children, %d matches in %s; bound %s (snapshot export + compile + iteration)", children, count, elapsed, bound)
	if count != children {
		t.Fatalf("got %d matches, want %d", count, children)
	}
	if elapsed >= bound {
		t.Fatalf("wide-array query took %s, bound %s", elapsed, bound)
	}
}
