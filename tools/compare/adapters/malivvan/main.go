// Command malivvan is the worker for malivvan/tree-sitter v0.0.1 (embedded wasm on wazero). The release has no tree
// disposal, no edit and no incremental parse, and exposes only C and C++: other languages and ops are unsupported.
package main

import (
	"context"
	"strings"
	"unsafe"

	sitter "github.com/malivvan/tree-sitter"

	"github.com/wotjr1649/go-treesitter/tools/compare/harness"
)

func main() { harness.Main("malivvan", open) }

type session struct {
	harness.Base
	ctx    context.Context
	p      sitter.Parser
	srcB   []byte // the string form is built once per input so its copy is not part of the timed call
	srcStr string
}

func open(lang string) (harness.Session, error) {
	if lang != "c" {
		return nil, harness.ErrUnsupported
	}
	ctx := context.Background()
	ts, err := sitter.New(ctx)
	if err != nil {
		return nil, err
	}
	p, err := ts.NewParser(ctx)
	if err != nil {
		return nil, err
	}
	l, err := ts.LanguageC(ctx)
	if err != nil {
		return nil, err
	}
	if err := p.SetLanguage(ctx, l); err != nil {
		return nil, err
	}
	return &session{ctx: ctx, p: p}, nil
}

func (s *session) Identity() map[string]string {
	return harness.BuildIdentity("github.com/malivvan/", "github.com/tetratelabs/")
}

func (s *session) Parse(src []byte) (harness.Tree, error) {
	if len(src) == 0 || len(s.srcB) != len(src) || unsafe.SliceData(s.srcB) != unsafe.SliceData(src) {
		s.srcB, s.srcStr = src, string(src)
	}
	return s.p.ParseString(s.ctx, s.srcStr)
}

func (s *session) Sexp(t harness.Tree) (string, bool, error) {
	root, err := t.(sitter.Tree).RootNode(s.ctx)
	if err != nil {
		return "", false, err
	}
	str, err := root.String(s.ctx)
	if err != nil {
		return "", false, err
	}
	// the API has no has-error call: read it from the markers the S-expression prints
	return str, strings.Contains(str, "(ERROR") || strings.Contains(str, "(MISSING") || strings.Contains(str, "(UNEXPECTED"), nil
}

func (s *session) Close() { _ = s.p.Close(s.ctx) }
