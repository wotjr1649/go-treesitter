//go:build !grammar_subset || grammar_subset_cpp

package gtsadapter

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/go-treesitter/syntax"
)

// C++ is outside the retained oracle epoch, so these are regression controls, not oracle records. Each digest is
// nodeDigest of the ordered nodes printed by separate native C builds of the same sources
// (tools/native-oracle/driver.c, runtime f5afe475, tree-sitter-cpp c0092228 checked-in parser.c and scanner.c).
// Carrier patch 30 alone parses each source the other way when fresh (KR-0007). The edit half only checks that
// incremental reuse reaches the same tree; that path does not use packed version order.
func TestCppPackedVersionOrderControls(t *testing.T) {
	cases := []struct{ name, source, digest string }{
		{"template-method-vs-binary-call", "void f() {\n  i = x.B::f<0>(5);\n}\n",
			"79168f26f501d428e49b46662c61a44fccea0c97f26879b900522bd23a2ece46"},
		{"qualified-template-vs-binary-scope", "void f() {\n  x = N::T<1>::v;\n}\n",
			"cbc6b4999aa73a0bba9a94e597cf3b770e3a95b0b108984d8d17a94e50ea37c8"},
		{"qualified-template-vs-binary-equality", "void f() {\n  x = N::T<1>::v == a::v;\n}\n",
			"4c3b567db32c2665742b80389f524e41885f5fda4640381083a76a903de19a0f"},
		{"splice-qualified-template-vs-binary", "void f() {\n  x = [:N:]::T<1>::v;\n}\n",
			"2a88c34630159f2017d254c07b7d120f7f6e39ebd0d2eeb12c7043d8a9686a16"},
	}
	parse := func(t *testing.T, request syntax.Request) string {
		t.Helper()
		result, err := (Adapter{}).Parse(context.Background(), request)
		if result.Tree != nil {
			t.Cleanup(result.Tree.Close)
		}
		if err != nil || !result.Complete() || result.Outcome != syntax.AcceptedClean {
			t.Fatalf("parse: err=%v outcome=%v diagnostics=%+v", err, result.Outcome, result.Diagnostics)
		}
		return nodeDigest(result.Tree.Nodes())
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			request := syntax.Request{Language: "cpp", Source: []byte(c.source), Timeout: time.Second}
			if got := parse(t, request); got != c.digest {
				t.Fatalf("fresh ordered nodes digest %s, native C %s", got, c.digest)
			}
			// Reach the same source by editing the template argument digit.
			at := strings.IndexAny(c.source, "01")
			old := c.source[:at] + "7" + c.source[at+1:]
			previous, err := (Adapter{}).Parse(context.Background(),
				syntax.Request{Language: "cpp", Source: []byte(old), Timeout: time.Second})
			if previous.Tree != nil {
				t.Cleanup(previous.Tree.Close)
			}
			if err != nil || !previous.Complete() {
				t.Fatalf("old source: err=%v diagnostics=%+v", err, previous.Diagnostics)
			}
			request.Previous = previous.Tree
			request.Edit = &syntax.Edit{StartByte: uint32(at), OldEndByte: uint32(at + 1), NewEndByte: uint32(at + 1)}
			if got := parse(t, request); got != c.digest {
				t.Fatalf("incremental ordered nodes digest %s, native C %s", got, c.digest)
			}
		})
	}
}
