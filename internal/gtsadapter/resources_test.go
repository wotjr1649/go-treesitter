package gtsadapter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wotjr1649/go-treesitter/syntax"
)

func TestRequestBudgets(t *testing.T) {
	source := []byte("package p\nfunc f() int { x := 1; return x + 1 }\n")
	for _, tc := range []struct {
		name              string
		limits            syntax.Limits
		localReason, stop string
	}{
		{"input", syntax.Limits{MaxInputBytes: len(source) - 1}, "input_bytes", ""},
		{"snapshot", syntax.Limits{MaxSnapshotNodes: 5}, "snapshot_nodes", "accepted"},
		{"iterations", syntax.Limits{IterationLimit: 1}, "", "iteration_limit"},
		{"nodes", syntax.Limits{NodeLimit: 1}, "", "node_limit"},
		{"stack", syntax.Limits{StackDepthLimit: 1}, "", "stack_depth_limit"},
		{"memory", syntax.Limits{MemoryBudgetBytes: 1}, "runtime_memory", "memory_budget"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: "x.go", Source: source, Limits: tc.limits, Timeout: time.Second})
			if r.Tree != nil {
				defer r.Tree.Close()
			}
			t.Logf("diagnostics=%+v error=%T", r.Diagnostics, err)
			stopMatches := r.StopReason == tc.stop
			if tc.name == "memory" {
				// The runtime charges growth and may accept a warm pooled arena.
				// Our receipt cap must reject both that route and a runtime stop.
				stopMatches = r.StopReason == "memory_budget" || r.StopReason == "accepted"
				if r.ArenaBytes+r.ScratchBytes <= 1 || r.SnapshotNodes != 0 {
					t.Fatal("memory receipt or early snapshot stop missing")
				}
			}
			if err == nil || r.ErrorType == "" || r.Outcome != syntax.ResourceLimit || r.Tree != nil || r.Complete() || r.LimitReason != tc.localReason || !stopMatches {
				t.Fatal("resource limit did not propagate")
			}
			if tc.name == "snapshot" && (r.SnapshotComplete || r.SnapshotNodes != 5) {
				t.Fatal("unbounded snapshot")
			}
		})
	}
}

func TestMemoryReceiptAfterWarmup(t *testing.T) {
	source := []byte("package p\nfunc f() int { x := 1; return x + 1 }\n")
	for i := 0; i < 3; i++ {
		warm, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: "x.go", Source: source, Limits: syntax.Limits{IterationLimit: 10000}})
		if err != nil || !warm.Complete() {
			t.Fatal("warm-up parse failed")
		}
		warm.Tree.Close()
		limited, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: "x.go", Source: source, Limits: syntax.Limits{MemoryBudgetBytes: 1}})
		t.Logf("run=%d stop=%s arena=%d scratch=%d local=%s", i, limited.StopReason, limited.ArenaBytes, limited.ScratchBytes, limited.LimitReason)
		if err == nil || limited.Tree != nil || limited.Complete() || limited.Outcome != syntax.ResourceLimit || limited.LimitReason != "runtime_memory" {
			t.Fatal("warm memory budget accepted")
		}
	}
}

func TestNestedInputStopsAtWorkLimit(t *testing.T) {
	source := []byte("package p\nvar x = " + strings.Repeat("(", 4000) + "1" + strings.Repeat(")", 4000) + "\n")
	r, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: "x.go", Source: source, Timeout: time.Second, Limits: syntax.Limits{StackDepthLimit: 128, IterationLimit: 2000, MaxSnapshotNodes: 1000}})
	if r.Tree != nil {
		defer r.Tree.Close()
	}
	if err == nil || r.Tree != nil || r.Outcome != syntax.ResourceLimit || r.Complete() {
		t.Fatalf("nested source escaped its budget: %+v", r.Diagnostics)
	}
}

func TestInvalidOptionsAndCancellation(t *testing.T) {
	for _, limits := range []syntax.Limits{{MaxInputBytes: -1}, {MaxSnapshotNodes: -1}, {MemoryBudgetBytes: -1}, {IterationLimit: -1}, {NodeLimit: -1}, {StackDepthLimit: -1}} {
		r, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: "x.go", Limits: limits})
		if err == nil || r.ErrorType == "" || r.Outcome != syntax.NotRun || r.Tree != nil {
			t.Fatalf("invalid limits accepted: %+v", limits)
		}
	}
	for _, ctx := range []context.Context{nil, context.Background()} {
		r, err := (Adapter{}).Parse(ctx, syntax.Request{Filename: "x.go", Timeout: -1})
		if err == nil || r.ErrorType == "" || r.Outcome != syntax.NotRun || r.Tree != nil {
			t.Fatal("invalid options accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := (Adapter{}).Parse(ctx, syntax.Request{Source: []byte{0xff, 0xfe}, Limits: syntax.Limits{MaxInputBytes: 1}})
	if r.Outcome != syntax.Cancelled || r.Tree != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("pre-cancellation lost priority or identity")
	}
	ctx, cancel = context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer cancel()
	r, err = (Adapter{}).Parse(ctx, syntax.Request{Filename: "x.go"})
	if r.Outcome != syntax.Cancelled || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("expired deadline ignored")
	}
}

func TestRuntimeTimeoutReleasesPartialTree(t *testing.T) {
	source := []byte("package p\n" + strings.Repeat("func f() int { return 123 }\n", 8000))
	r, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: "x.go", Source: source, Timeout: time.Microsecond})
	if r.Tree != nil {
		defer r.Tree.Close()
	}
	t.Logf("diagnostics=%+v", r.Diagnostics)
	if err == nil || r.Outcome != syntax.Timeout || r.Tree != nil || r.Complete() || !r.DeadlineApplied {
		t.Fatal("timeout treated as a complete tree")
	}
}

func TestSourceOwnershipAndUTF8Edit(t *testing.T) {
	input := []byte("package p\nvar café = 1\n")
	after := bytes.Replace(input, []byte("1"), []byte("22"), 1)
	first, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: "x.go", Source: input})
	if err != nil || !first.Complete() {
		t.Fatal("initial parse")
	}
	defer first.Tree.Close()
	inside := uint32(bytes.Index(input, []byte("é")) + 1)
	invalid, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: "x.go", Source: input, Previous: first.Tree, Edit: &syntax.Edit{StartByte: inside, OldEndByte: inside, NewEndByte: inside}})
	if err == nil || invalid.Tree != nil || invalid.Outcome != syntax.NotRun {
		t.Fatal("split UTF-8 edit accepted")
	}
	offset := uint32(bytes.IndexByte(input, '1'))
	input[0] = 'x' // A caller may reuse its source buffer after Parse returns.
	inc, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: "x.go", Source: after, Previous: first.Tree, Edit: &syntax.Edit{StartByte: offset, OldEndByte: offset + 1, NewEndByte: offset + 2}})
	if err != nil || !inc.Complete() {
		t.Fatalf("caller mutation affected old tree: %+v", inc.Diagnostics)
	}
	defer inc.Tree.Close()
	fresh, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: "x.go", Source: after})
	if err != nil || !fresh.Complete() {
		t.Fatal("fresh parse")
	}
	defer fresh.Tree.Close()
	if !reflect.DeepEqual(inc.Tree.Nodes(), fresh.Tree.Nodes()) {
		t.Fatal("UTF-8 edit differs")
	}
	var absent *tree
	r, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: "x.go", Source: after, Previous: absent, Edit: &syntax.Edit{}})
	if err == nil || r.Outcome != syntax.NotRun || r.Tree != nil {
		t.Fatal("typed nil old tree accepted")
	}
}

func TestIndependentWorkers(t *testing.T) {
	inputs := []struct{ file, source string }{
		{"x.go", "package p\nfunc f() int { return 1 }\n"},
		{"x.py", "def f(x):\n    return x + 1\n"},
		{"x.js", "const f = x => x + 1;\n"},
		{"x.jsx", "const X = () => <p>hello</p>;\n"},
		{"x.ts", "interface X { value: number }\n"},
		{"x.tsx", "const X = () => <p>hello</p>;\n"},
		{"x.cs", "class C { public int F() => 1; }\n"},
	}
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 14; i++ {
				c := inputs[(worker+i)%len(inputs)]
				// This is a correctness watchdog, not a latency gate. Leave room
				// for cold grammar initialization under race instrumentation.
				r, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: c.file, Source: []byte(c.source), Timeout: 10 * time.Second})
				if err != nil || !r.Complete() || r.Outcome != syntax.AcceptedClean {
					if r.Tree != nil {
						r.Tree.Close()
					}
					t.Errorf("worker %d %s: %+v", worker, c.file, r.Diagnostics)
					return
				}
				after := []byte(c.source + "\n")
				end := uint32(len(c.source))
				inc, incErr := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: c.file, Source: after,
					Previous: r.Tree, Edit: &syntax.Edit{StartByte: end, OldEndByte: end, NewEndByte: end + 1}, Timeout: 10 * time.Second})
				r.Tree.Close()
				fresh, freshErr := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: c.file, Source: after, Timeout: 10 * time.Second})
				equal := incErr == nil && freshErr == nil && inc.Complete() && fresh.Complete() &&
					reflect.DeepEqual(inc.Tree.Nodes(), fresh.Tree.Nodes())
				if inc.Tree != nil {
					inc.Tree.Close()
				}
				if fresh.Tree != nil {
					fresh.Tree.Close()
				}
				if !equal {
					t.Errorf("worker %d %s: concurrent incremental/fresh difference", worker, c.file)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func FuzzBoundedParse(f *testing.F) {
	for _, seed := range []string{"package p\nvar x = 1\n", "", "\xff", "package p\nvar x = (((1)))", "/*\x00*/"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, source []byte) {
		if len(source) > 512 {
			source = source[:512]
		}
		r, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: "x.go", Source: source, Timeout: 100 * time.Millisecond,
			Limits: syntax.Limits{MaxInputBytes: 512, MaxSnapshotNodes: 4096, MemoryBudgetBytes: 32 << 20, IterationLimit: 10000, NodeLimit: 10000, StackDepthLimit: 256}})
		if r.Tree != nil {
			defer r.Tree.Close()
		}
		accepted := r.Outcome == syntax.AcceptedClean || r.Outcome == syntax.AcceptedWithErrors
		if !r.Outcome.Valid() || accepted != (err == nil) || accepted != (r.Tree != nil) || accepted != r.Complete() {
			t.Fatal("invalid result receipt")
		}
		if r.Tree != nil {
			for i, n := range r.Tree.Nodes() {
				if n.StartByte > n.EndByte || int(n.EndByte) > len(source) || (i == 0 && n.Parent != -1) || (i > 0 && (n.Parent < 0 || n.Parent >= i)) {
					t.Fatal(fmt.Sprintf("invalid node %d", i))
				}
			}
		}
	})
}
