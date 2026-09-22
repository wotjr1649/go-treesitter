package gtsadapter

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"testing"

	"github.com/wotjr1649/go-treesitter/syntax"
)

var releaseSeed = flag.Uint64("release-seed", 1, "fixed benchmark ordering seed")
var releaseBenchmarkSink int

// BenchmarkRelease separates adapter end-to-end work from snapshot/index costs.
// An edit consumes its old tree; setup is excluded, while Parse and Close are timed.
func BenchmarkRelease(b *testing.B) {
	type input struct {
		name, filename string
		source         []byte
	}
	var inputs []input
	for _, c := range []struct{ ext, format string }{
		{"go", "func f%d() int { return %d }\n"}, {"py", "def f%d():\n    return %d\n"},
		{"js", "const value%d = %d;\n"}, {"jsx", "const View%d = () => <p>{%d}</p>;\n"},
		{"ts", "const value%d: number = %d;\n"}, {"tsx", "const View%d = () => <p>{%d}</p>;\n"},
		{"cs", "class C%d { public int F() => %d; }\n"},
	} {
		var source bytes.Buffer
		if c.ext == "go" {
			source.WriteString("package p\n")
		}
		for i := range 128 {
			fmt.Fprintf(&source, c.format, i, i)
		}
		inputs = append(inputs, input{c.ext, "bench." + c.ext, source.Bytes()})
	}
	recovery, err := os.ReadFile("../../testdata/newtonsoft/JsonTextReader-excerpt.cs")
	if err != nil {
		b.Fatal(err)
	}
	inputs = append(inputs, input{"cs-recovery", "bench.cs", recovery})
	rng := rand.New(rand.NewPCG(*releaseSeed, 0))
	rng.Shuffle(len(inputs), func(i, j int) { inputs[i], inputs[j] = inputs[j], inputs[i] })
	for _, c := range inputs {
		b.Run(c.name, func(b *testing.B) {
			p := Adapter{}
			parse := func(source []byte, previous syntax.Tree, edit *syntax.Edit) syntax.Result {
				r, err := p.Parse(context.Background(), syntax.Request{Filename: c.filename, Source: source, Previous: previous, Edit: edit})
				if err != nil || !r.Complete() {
					if r.Tree != nil {
						r.Tree.Close()
					}
					b.Fatal("incomplete benchmark parse", err, r.Diagnostics)
				}
				return r
			}
			warm := parse(c.source, nil, nil)
			defer warm.Tree.Close()
			// Recovery materialization is included in full: the adapter releases
			// the non-clean backend before returning the owned snapshot.
			modes := []string{"full", "index", "query"}
			if warm.Outcome == syntax.AcceptedClean {
				modes = append(modes, "snapshot", "replace", "insert", "delete", "no-edit")
			}
			rng.Shuffle(len(modes), func(i, j int) { modes[i], modes[j] = modes[j], modes[i] })
			for _, mode := range modes {
				b.Run(mode, func(b *testing.B) {
					b.ReportAllocs()
					index, err := syntax.NewIndex(warm.Tree.Nodes())
					if err != nil {
						b.Fatal(err)
					}
					after := bytes.Clone(c.source)
					at := uint32(bytes.IndexByte(after, '1'))
					edit := syntax.Edit{StartByte: at, OldEndByte: at + 1, NewEndByte: at + 1}
					switch mode {
					case "replace":
						after[at] = '2'
					case "insert":
						after = bytes.Join([][]byte{after[:at], []byte("2"), after[at:]}, nil)
						edit.OldEndByte = at
					case "delete":
						// Delete the trailing newline: valid in all seven grammars.
						at = uint32(len(after) - 1)
						after = after[:at]
						edit = syntax.Edit{StartByte: at, OldEndByte: at + 1, NewEndByte: at}
					case "no-edit":
						edit = syntax.Edit{}
					}
					var reused, fallback, arena, scratch, depth int64
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						switch mode {
						case "snapshot":
							raw := warm.Tree.(*tree)
							nodes, err := boundedSnapshot(context.Background(), raw.raw.RootNode(), raw.lang, 0)
							if err != nil {
								b.Fatal(err)
							}
							releaseBenchmarkSink = len(nodes)
						case "index":
							x, err := syntax.NewIndex(warm.Tree.Nodes())
							if err != nil {
								b.Fatal(err)
							}
							releaseBenchmarkSink, _ = x.NodeAt(0)
						case "query":
							releaseBenchmarkSink, _ = index.NodeAt(uint32(i*7919) % uint32(len(c.source)))
						default:
							var old syntax.Tree
							var change *syntax.Edit
							if mode != "full" {
								b.StopTimer()
								old = parse(c.source, nil, nil).Tree
								change = &edit
								b.StartTimer()
							}
							r := parse(after, old, change)
							if r.ReusedOldTree {
								reused++
							}
							if r.FallbackReason != "" || r.ReuseReason != "" {
								fallback++
							}
							arena, scratch, depth = max(arena, r.ArenaBytes), max(scratch, r.ScratchBytes), max(depth, int64(r.PeakStackDepth))
							r.Tree.Close()
							if old != nil {
								old.Close()
							}
						}
					}
					b.StopTimer()
					if mode != "snapshot" && mode != "index" && mode != "query" {
						b.ReportMetric(float64(reused)/float64(b.N), "reuse/op")
						b.ReportMetric(float64(fallback)/float64(b.N), "fallback/op")
						b.ReportMetric(float64(arena), "peak-arena-B")
						b.ReportMetric(float64(scratch), "peak-scratch-B")
						b.ReportMetric(float64(depth), "peak-depth")
					}
				})
			}
		})
	}
}
