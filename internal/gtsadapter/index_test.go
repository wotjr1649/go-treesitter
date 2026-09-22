package gtsadapter

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/wotjr1649/go-treesitter/syntax"
)

func linearNodeAt(nodes []syntax.Node, offset uint32) (int, bool) {
	best, depth := -1, -1
	for i, n := range nodes {
		if n.StartByte <= offset && offset < n.EndByte {
			d := 0
			for parent := n.Parent; parent >= 0; parent = nodes[parent].Parent {
				d++
			}
			if d > depth {
				best, depth = i, d
			}
		}
	}
	return best, best >= 0
}

func TestIndexRealSnapshots(t *testing.T) {
	data, err := os.ReadFile("../../testdata/oracle/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []oracleCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			var source []byte
			if c.Source != nil {
				source = []byte(*c.Source)
			} else {
				var err error
				source, err = os.ReadFile("../../" + *c.Path)
				if err != nil {
					t.Fatal(err)
				}
			}
			if fmt.Sprintf("%x", sha256.Sum256(source)) != c.SHA256 {
				t.Fatal("fixture identity changed")
			}
			r, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: c.Filename, Source: source})
			if r.Tree != nil {
				defer r.Tree.Close()
			}
			if err != nil || !r.Complete() {
				t.Fatal("incomplete snapshot")
			}
			nodes := r.Tree.Nodes()
			x, err := syntax.NewIndex(nodes)
			if err != nil {
				t.Fatal(err)
			}
			for offset := uint32(0); int(offset) <= len(source); offset++ {
				got, ok := x.NodeAt(offset)
				want, found := linearNodeAt(nodes, offset)
				if got != want || ok != found {
					t.Fatalf("offset=%d got=%d want=%d", offset, got, want)
				}
			}
			byType := map[string][]int{}
			for i, n := range nodes {
				byType[n.Type] = append(byType[n.Type], i)
			}
			for typ, want := range byType {
				if got := slices.Collect(x.OfType(typ)); !reflect.DeepEqual(got, want) {
					t.Fatalf("type=%s", typ)
				}
			}
			// Readers share only the immutable index, never a parser or tree.
			var wg sync.WaitGroup
			for range 4 {
				wg.Go(func() {
					for offset := uint32(0); int(offset) <= len(source); offset += 19 {
						got, _ := x.NodeAt(offset)
						want, _ := linearNodeAt(nodes, offset)
						if got != want {
							t.Error("concurrent lookup mismatch")
							return
						}
					}
				})
			}
			wg.Wait()
		})
	}
}

var indexBenchmarkSink int

func BenchmarkSnapshotLookup(b *testing.B) {
	var source strings.Builder
	source.WriteString("package p\n")
	for i := range 2048 {
		fmt.Fprintf(&source, "var item%d = %d\n", i, i)
	}
	input := []byte(source.String())
	r, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: "bench.go", Source: input})
	if r.Tree != nil {
		defer r.Tree.Close()
	}
	if err != nil || !r.Complete() {
		b.Fatal("benchmark parse")
	}
	nodes := r.Tree.Nodes()
	x, err := syntax.NewIndex(nodes)
	if err != nil {
		b.Fatal(err)
	}
	b.Logf("source=%x bytes=%d nodes=%d digest=%s", sha256.Sum256(input), len(input), len(nodes), nodeDigest(nodes))
	b.Run("BuildIndex", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			x, err := syntax.NewIndex(nodes)
			if err != nil {
				b.Fatal(err)
			}
			indexBenchmarkSink, _ = x.NodeAt(0)
		}
	})
	for _, indexed := range []bool{false, true} {
		name := "Linear"
		if indexed {
			name = "Indexed"
		}
		b.Run(name+"Position", func(b *testing.B) {
			b.ReportAllocs()
			query := uint32(0)
			for b.Loop() {
				query = (query + 7919) % uint32(len(input))
				if indexed {
					indexBenchmarkSink, _ = x.NodeAt(query)
				} else {
					indexBenchmarkSink, _ = linearNodeAt(nodes, query)
				}
			}
		})
		b.Run(name+"Type", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				count := 0
				if indexed {
					for range x.OfType("identifier") {
						count++
					}
				} else {
					for _, n := range nodes {
						if n.Type == "identifier" {
							count++
						}
					}
				}
				indexBenchmarkSink = count
			}
		})
	}
}
