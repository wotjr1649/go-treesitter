package gtsadapter

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/wotjr1649/go-treesitter/syntax"
)

var hardeningInputs = []struct{ filename, source string }{
	{"x.go", "package p\nvar café = 1\n"},
	{"x.py", "def f(x):\n    return x + 1\n"},
	{"x.js", "const value = 1;\n"},
	{"x.jsx", "const View = () => <p>a = b &amp; 한글</p>;\n"},
	{"x.ts", "const value: number = 1;\n"},
	{"x.tsx", "const View = () => <p>a = b &amp; 한글</p>;\n"},
	{"x.cs", "namespace N { class C { public int F() => 1; } }\n"},
}

func TestCancellationDuringWork(t *testing.T) {
	before := runtime.NumGoroutine()
	for _, input := range hardeningInputs {
		t.Run(input.filename, func(t *testing.T) {
			p := Adapter{}
			warm, err := p.Parse(context.Background(), syntax.Request{Filename: input.filename, Source: []byte(input.source)})
			if err != nil || !warm.Complete() {
				t.Fatal("warm parse", err)
			}
			warm.Tree.Close()
			source := []byte(strings.Repeat(input.source, 4096))
			ctx, cancel := context.WithCancel(context.Background())
			trigger := time.AfterFunc(20*time.Millisecond, cancel)
			started := time.Now()
			r, err := p.Parse(ctx, syntax.Request{Filename: input.filename, Source: source, Timeout: 2 * time.Second})
			elapsed := time.Since(started)
			trigger.Stop()
			cancel()
			if r.Tree != nil {
				r.Tree.Close()
			}
			t.Logf("elapsed=%s stop=%s tokens=%d iterations=%d arena=%d scratch=%d", elapsed, r.StopReason, r.TokensConsumed, r.Iterations, r.ArenaBytes, r.ScratchBytes)
			if !errors.Is(err, context.Canceled) || r.Outcome != syntax.Cancelled || r.Tree != nil || r.Complete() {
				t.Fatalf("in-flight cancellation lost: %v %+v", err, r.Diagnostics)
			}
			next, err := p.Parse(context.Background(), syntax.Request{Filename: input.filename, Source: []byte(input.source)})
			if next.Tree != nil {
				next.Tree.Close()
			}
			if err != nil || !next.Complete() || next.Outcome != syntax.AcceptedClean {
				t.Fatal("parse after cancellation", err)
			}
		})
	}
	// Give real context callbacks a bounded opportunity to finish; do not require
	// an exact process-wide count while unrelated Go runtime workers may start.
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > before+2 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if after := runtime.NumGoroutine(); after > before+2 {
		t.Fatalf("goroutines grew: %d -> %d", before, after)
	}
}

func TestCancellationDuringSnapshot(t *testing.T) {
	source := []byte("package p\n" + strings.Repeat("var value = 1\n", 10000))
	r, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: "x.go", Source: source, Timeout: 10 * time.Second})
	if err != nil || !r.Complete() {
		t.Fatal("snapshot setup", err)
	}
	defer r.Tree.Close()
	raw := r.Tree.(*tree)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	trigger := time.AfterFunc(2*time.Millisecond, cancel)
	defer trigger.Stop()
	started := time.Now()
	nodes, err := boundedSnapshot(ctx, raw.raw.RootNode(), raw.lang, 0)
	t.Logf("snapshot cancellation elapsed=%s partial_nodes=%d total_nodes=%d", time.Since(started), len(nodes), len(r.Tree.Nodes()))
	if !errors.Is(err, context.Canceled) || len(nodes) >= len(r.Tree.Nodes()) {
		t.Fatal("snapshot ignored cancellation")
	}
}

func FuzzIncrementalAgreement(f *testing.F) {
	for i, input := range hardeningInputs {
		for _, change := range []struct {
			start, removed uint16
			insert         string
		}{
			{0, 0, ""}, {0, 0, "\n"}, {uint16(len(input.source)), 0, "\n"},
			{1, 1, "한"}, {0, 1, "#if A\n"}, {3, 2, "\xff"},
		} {
			f.Add(uint8(i), change.start, change.removed, []byte(change.insert))
		}
	}
	f.Fuzz(func(t *testing.T, language uint8, position, removed uint16, insert []byte) {
		input := hardeningInputs[int(language)%len(hardeningInputs)]
		before := []byte(input.source)
		start := int(position) % (len(before) + 1)
		oldEnd := start + int(removed)%(len(before)-start+1)
		if len(insert) > 128 {
			insert = insert[:128]
		}
		after := append(bytes.Clone(before[:start]), insert...)
		after = append(after, before[oldEnd:]...)
		p := Adapter{}
		old, err := p.Parse(context.Background(), syntax.Request{Filename: input.filename, Source: before})
		if err != nil || !old.Complete() || old.Outcome != syntax.AcceptedClean {
			t.Fatal("clean seed failed", err)
		}
		defer old.Tree.Close()
		limits := syntax.Limits{MaxSnapshotNodes: 4096, MemoryBudgetBytes: 64 << 20, IterationLimit: 20000, NodeLimit: 20000, StackDepthLimit: 256}
		req := syntax.Request{Filename: input.filename, Source: after, Previous: old.Tree,
			Edit:    &syntax.Edit{StartByte: uint32(start), OldEndByte: uint32(oldEnd), NewEndByte: uint32(start + len(insert))},
			Timeout: time.Second, Limits: limits}
		inc, incErr := p.Parse(context.Background(), req)
		if inc.Tree != nil {
			defer inc.Tree.Close()
		}
		valid := utf8.Valid(after) && utf8.Valid(before[:start]) && utf8.Valid(before[:oldEnd]) && utf8.Valid(after[:start+len(insert)])
		if !valid {
			want := syntax.NotRun
			if !utf8.Valid(after) {
				want = syntax.UnsupportedInput
			}
			if incErr == nil || inc.Tree != nil || inc.Outcome != want || old.Tree.(*tree).edited {
				t.Fatal("invalid edit changed or escaped admission")
			}
			return
		}
		req.Previous, req.Edit = nil, nil
		fresh, freshErr := p.Parse(context.Background(), req)
		if fresh.Tree != nil {
			defer fresh.Tree.Close()
		}
		for _, item := range []struct {
			result syntax.Result
			err    error
		}{{inc, incErr}, {fresh, freshErr}} {
			accepted := item.result.Outcome == syntax.AcceptedClean || item.result.Outcome == syntax.AcceptedWithErrors
			if !item.result.Outcome.Valid() || accepted != (item.err == nil) || accepted != (item.result.Tree != nil) || accepted != item.result.Complete() {
				t.Fatal("invalid edit result receipt")
			}
		}
		// Work limits can stop fresh and reused parses at different checkpoints.
		// Exact trees are required whenever both requests complete.
		if inc.Complete() && fresh.Complete() && !reflect.DeepEqual(inc.Tree.Nodes(), fresh.Tree.Nodes()) {
			t.Fatalf("incremental/fresh divergence: %s start=%d old_end=%d insert=%x", input.filename, start, oldEnd, insert)
		}
	})
}
