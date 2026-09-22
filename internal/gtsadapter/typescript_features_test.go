package gtsadapter

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/wotjr1649/go-treesitter/syntax"
)

// The maintained upstream TypeScript patch defines these four features.
// This retained check covers product behavior and edit self-consistency;
// separately identified C artifacts establish oracle agreement.
func TestTypeScriptMaintainedGrammarFeatures(t *testing.T) {
	cases := []struct {
		name, before, source, nodeType string
		count                          int
	}{
		{"import", "type T = string;\n", "type T = import(\"pkg\").Name;\n", "import_type", 1},
		{"variance", "type Co<T> = T;\n", "type Co<out T> = T; type Contra<in T> = (x: T) => void; type Inv<in out T> = T;\n", "variance", 3},
		{"calls", "type Calls = { <T>(value: T): T; }\n", "type Calls = {\n  <T>(value: T): T\n  <U>(value: U): U\n}\n", "call_signature", 2},
		{"contextual-in", "type Easing = { default: string; in: string; out: string };\n", "type Easing = {\n  default: string\n  in: string\n  out: string\n}\n", "property_signature", 3},
	}
	for _, filename := range []string{"x.ts", "x.tsx"} {
		for _, c := range cases {
			t.Run(filename+"/"+c.name, func(t *testing.T) {
				parse := func(source string, old syntax.Tree, edit *syntax.Edit) syntax.Result {
					t.Helper()
					r, err := (Adapter{}).Parse(context.Background(), syntax.Request{
						Filename: filename, Source: []byte(source), Previous: old, Edit: edit, Timeout: 10 * time.Second})
					if err != nil || !r.Complete() || r.Outcome != syntax.AcceptedClean {
						if r.Tree != nil {
							r.Tree.Close()
						}
						t.Fatalf("grammar feature did not parse cleanly: %+v", r.Diagnostics)
					}
					return r
				}
				var expected []syntax.Node
				for i := 0; i < 20; i++ {
					r := parse(c.source, nil, nil)
					nodes := r.Tree.Nodes()
					r.Tree.Close()
					if i == 0 {
						expected = nodes
					} else if !reflect.DeepEqual(expected, nodes) {
						logNodeDifference(t, "repeated fresh", expected, nodes)
						t.Fatalf("fresh tree changed on repetition %d", i+1)
					}
				}
				count := 0
				for _, n := range expected {
					if n.Type == c.nodeType {
						count++
					}
				}
				if count != c.count {
					t.Fatalf("%s count = %d, want %d", c.nodeType, count, c.count)
				}
				start, oldEnd, newEnd := 0, len(c.before), len(c.source)
				for start < min(oldEnd, newEnd) && c.before[start] == c.source[start] {
					start++
				}
				for oldEnd > start && newEnd > start && c.before[oldEnd-1] == c.source[newEnd-1] {
					oldEnd--
					newEnd--
				}
				old := parse(c.before, nil, nil)
				defer old.Tree.Close()
				inc := parse(c.source, old.Tree, &syntax.Edit{StartByte: uint32(start), OldEndByte: uint32(oldEnd), NewEndByte: uint32(newEnd)})
				defer inc.Tree.Close()
				if !reflect.DeepEqual(expected, inc.Tree.Nodes()) {
					logNodeDifference(t, "incremental/fresh", inc.Tree.Nodes(), expected)
					t.Fatal("grammar feature edit changed the canonical tree")
				}
				t.Logf("fresh_repeats=20 digest=%s incremental=fresh", nodeDigest(expected))
			})
		}
	}
}
