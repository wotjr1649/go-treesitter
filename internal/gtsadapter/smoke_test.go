package gtsadapter

import (
	"context"
	"crypto/sha256"
	"reflect"
	"testing"

	"github.com/wotjr1649/go-treesitter/syntax"
)

func TestSevenLanguageIncrementalSelfConsistency(t *testing.T) {
	for _, tc := range []struct{ id, file, source string }{
		{"SM-GO", "x.go", "package p\nfunc f() int { return 1 }\n"},
		{"SM-PY", "x.py", "def f(xs):\n    if xs:\n        return [x + 1 for x in xs]\n"},
		{"SM-JS", "x.js", "const f = () => <p>hello</p>;\n"},
		{"SM-JSX", "x.jsx", "const f = () => <p>hello</p>;\n"},
		{"SM-TS", "x.ts", "interface Box<T> { value: T }\nconst f = <T>(x: T): T => x;\n"},
		{"SM-TSX", "x.tsx", "const App = () => <p>hello</p>;\n"},
		{"SM-CS", "x.cs", "class C { public int X { get; set; } public int F() => X + 1; }\n"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			ctx := context.Background()
			first, err := (Adapter{}).Parse(ctx, syntax.Request{Filename: tc.file, Source: []byte(tc.source)})
			if first.Tree != nil {
				defer first.Tree.Close()
			}
			t.Logf("fresh source_sha256=%x diagnostics=%+v", sha256.Sum256([]byte(tc.source)), first.Diagnostics)
			if err != nil || first.Outcome != syntax.AcceptedClean || !first.Complete() {
				t.Fatalf("baseline smoke: error type=%T", err)
			}
			edited := []byte(tc.source + "\n")
			e := &syntax.Edit{StartByte: uint32(len(tc.source)), OldEndByte: uint32(len(tc.source)), NewEndByte: uint32(len(edited))}
			incremental, incErr := (Adapter{}).Parse(ctx, syntax.Request{Filename: tc.file, Source: edited, Previous: first.Tree, Edit: e})
			if incremental.Tree != nil {
				defer incremental.Tree.Close()
			}
			fresh, freshErr := (Adapter{}).Parse(ctx, syntax.Request{Filename: tc.file, Source: edited})
			if fresh.Tree != nil {
				defer fresh.Tree.Close()
			}
			t.Logf("edited source_sha256=%x incremental=%+v fresh=%+v", sha256.Sum256(edited), incremental.Diagnostics, fresh.Diagnostics)
			if incErr != nil || freshErr != nil || !incremental.Complete() || !fresh.Complete() || incremental.Outcome != syntax.AcceptedClean || fresh.Outcome != syntax.AcceptedClean {
				t.Fatalf("incomplete comparison: incremental error=%T fresh error=%T", incErr, freshErr)
			}
			left, right := incremental.Tree.Nodes(), fresh.Tree.Nodes()
			if (tc.id == "SM-PY" || tc.id == "SM-CS") && incremental.ReuseReason == "" {
				t.Fatal("missing upstream reuse/fallback attribution")
			}
			firstDiff := -1
			for i := 0; i < min(len(left), len(right)); i++ {
				if !reflect.DeepEqual(left[i], right[i]) {
					firstDiff = i
					break
				}
			}
			if firstDiff < 0 && len(left) != len(right) {
				firstDiff = min(len(left), len(right))
			}
			t.Logf("self-consistency: left_nil=%t right_nil=%t left_len=%d right_len=%d first_difference=%d reuse=%t raw_fallback=%q", left == nil, right == nil, len(left), len(right), firstDiff, incremental.ReusedOldTree, incremental.FallbackReason)
			if firstDiff >= 0 {
				if firstDiff < len(left) {
					t.Logf("left[%d]=%+v", firstDiff, left[firstDiff])
				}
				if firstDiff < len(right) {
					t.Logf("right[%d]=%+v", firstDiff, right[firstDiff])
				}
				t.Fatal("incremental/fresh ordered snapshot mismatch")
			}
		})
	}
}
