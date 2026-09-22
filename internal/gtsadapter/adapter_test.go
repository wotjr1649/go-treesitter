package gtsadapter

import (
	"context"
	"testing"

	"github.com/wotjr1649/go-treesitter/syntax"
)

func TestReachableOutcomes(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name    string
		ctx     context.Context
		request syntax.Request
		want    syntax.Outcome
	}{
		{"unsupported_utf8", context.Background(), syntax.Request{Filename: "x.go", Source: []byte{0xff}}, syntax.UnsupportedInput},
		{"unsupported_grammar", context.Background(), syntax.Request{Filename: "x.unknown"}, syntax.UnsupportedInput},
		{"cancelled", cancelled, syntax.Request{Filename: "x.go", Source: []byte("package p\n")}, syntax.Cancelled},
		{"cancelled_before_unsupported", cancelled, syntax.Request{Source: []byte{0xff}}, syntax.Cancelled},
		{"accepted_clean", context.Background(), syntax.Request{Filename: "x.go", Source: []byte("package p\nfunc f() {}\n")}, syntax.AcceptedClean},
		{"accepted_with_errors", context.Background(), syntax.Request{Filename: "x.go", Source: []byte("package p\nfunc f( {\n")}, syntax.AcceptedWithErrors},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := (Adapter{}).Parse(tc.ctx, tc.request)
			if r.Tree != nil {
				defer r.Tree.Close()
			}
			if r.Outcome != tc.want {
				t.Fatalf("outcome=%s error type=%T diagnostics=%+v", r.Outcome, err, r.Diagnostics)
			}
			accepted := tc.want == syntax.AcceptedClean || tc.want == syntax.AcceptedWithErrors
			if accepted != (err == nil) {
				t.Fatalf("error propagation: %T", err)
			}
			if !accepted && r.Tree != nil {
				t.Fatal("failure leaked a tree")
			}
			if tc.want == syntax.AcceptedWithErrors {
				if !r.Complete() || !r.HasError || r.Tree.(*tree).raw != nil {
					t.Fatalf("non-clean receipt/lifetime: %+v", r.Diagnostics)
				}
			}
			t.Logf("%+v", r.Diagnostics)
		})
	}
}

func TestInvalidEditDoesNotMutatePrevious(t *testing.T) {
	r, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: "x.go", Source: []byte("package p\n")})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Tree.Close()
	next, err := (Adapter{}).Parse(context.Background(), syntax.Request{Filename: "x.go", Source: []byte("package q\n"), Previous: r.Tree, Edit: &syntax.Edit{StartByte: 100, OldEndByte: 100, NewEndByte: 100}})
	if err == nil || next.Outcome != syntax.NotRun || next.Tree != nil || r.Tree.(*tree).edited {
		t.Fatalf("invalid edit accepted: %+v", next.Diagnostics)
	}
	r.Tree.Close()
	r.Tree.Close()
}
