//go:build !grammar_subset || (grammar_subset_c && grammar_subset_cpp)

package gtsadapter

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/wotjr1649/go-treesitter/syntax"
)

func TestCFamilyLexerStateAcrossReuse(t *testing.T) {
	sources := []string{
		"#include <stdio.h>\nint main() { return 0; }\n",
		"#include \"x.h\"\nint x = 1;\n",
		"#define VALUE 1\nint x = VALUE;\n",
		"#define F(x) ((x)+1)\nint x = F(2);\n",
		"#if FLAG\nint x;\n#else\nint y;\n#endif\n",
		"const char *x = \"one\\ntwo\";\n",
		"int x = 1; /* ordinary */\n",
	}
	for _, language := range []string{"c", "cpp"} {
		for _, source := range sources {
			t.Run(language+"/"+source, func(t *testing.T) {
				request := syntax.Request{Language: language, Source: []byte(source), Timeout: time.Second}
				old, err := (Adapter{}).Parse(context.Background(), request)
				if old.Tree != nil {
					defer old.Tree.Close()
				}
				if err != nil || !old.Complete() || old.Outcome != syntax.AcceptedClean {
					t.Fatalf("initial source: %+v", old.Diagnostics)
				}
				request.Source = []byte(source + "\n")
				request.Previous = old.Tree
				request.Edit = &syntax.Edit{StartByte: uint32(len(source)), OldEndByte: uint32(len(source)), NewEndByte: uint32(len(source) + 1)}
				incremental, editErr := (Adapter{}).Parse(context.Background(), request)
				if incremental.Tree != nil {
					defer incremental.Tree.Close()
				}
				request.Previous, request.Edit = nil, nil
				fresh, freshErr := (Adapter{}).Parse(context.Background(), request)
				if fresh.Tree != nil {
					defer fresh.Tree.Close()
				}
				if editErr != nil || freshErr != nil || !incremental.Complete() || !fresh.Complete() ||
					incremental.Outcome != syntax.AcceptedClean || fresh.Outcome != syntax.AcceptedClean {
					t.Fatalf("incremental=%+v fresh=%+v", incremental.Diagnostics, fresh.Diagnostics)
				}
				if !reflect.DeepEqual(incremental.Tree.Nodes(), fresh.Tree.Nodes()) {
					t.Fatalf("ordered nodes differ: incremental=%+v fresh=%+v", incremental.Tree.Nodes(), fresh.Tree.Nodes())
				}
			})
		}
	}
}
