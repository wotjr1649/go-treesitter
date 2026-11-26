// Command smacker is the worker for smacker/go-tree-sitter (cgo; vendored runtime and bundled grammars).
// Its bundled C and JavaScript parser.c carry compile-time optimize-off pragmas; the stock build keeps them.
package main

import (
	"context"
	"errors"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"

	"github.com/wotjr1649/go-treesitter/tools/compare/harness"
)

func main() { harness.Main("smacker", open) }

type session struct {
	harness.Base
	p *sitter.Parser
}

func open(lang string) (harness.Session, error) {
	var l *sitter.Language
	switch lang {
	case "c":
		l = c.GetLanguage()
	case "go":
		l = golang.GetLanguage()
	case "javascript":
		l = javascript.GetLanguage()
	case "python":
		l = python.GetLanguage()
	default: // json: the module ships no JSON package
		return nil, harness.ErrUnsupported
	}
	p := sitter.NewParser()
	p.SetLanguage(l)
	return &session{p: p}, nil
}

func (s *session) Identity() map[string]string {
	return harness.BuildIdentity("github.com/smacker/")
}

func (s *session) Parse(src []byte) (harness.Tree, error) { return s.parse(src, nil) }

func (s *session) parse(src []byte, old *sitter.Tree) (harness.Tree, error) {
	t, err := s.p.ParseCtx(context.Background(), old, src)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, errors.New("ParseCtx returned nil")
	}
	return t, nil
}

// Edit passes the harness's edit unchanged. Smacker's EditInput.c() maps OldEndPoint into the new-end point;
// the timing workload therefore uses equal-point edits, and the newline cases expose any consequence.
func (s *session) Edit(t harness.Tree, e harness.Edit) error {
	t.(*sitter.Tree).Edit(sitter.EditInput{
		StartIndex: e.StartByte, OldEndIndex: e.OldEndByte, NewEndIndex: e.NewEndByte,
		StartPoint:  sitter.Point{Row: e.StartRow, Column: e.StartCol},
		OldEndPoint: sitter.Point{Row: e.OldEndRow, Column: e.OldEndCol},
		NewEndPoint: sitter.Point{Row: e.NewEndRow, Column: e.NewEndCol},
	})
	return nil
}

func (s *session) Reparse(src []byte, old harness.Tree) (harness.Tree, error) {
	return s.parse(src, old.(*sitter.Tree))
}

func (s *session) Release(t harness.Tree) {
	if tr, ok := t.(*sitter.Tree); ok {
		tr.Close()
	}
}

func (s *session) Sexp(t harness.Tree) (string, bool, error) {
	root := t.(*sitter.Tree).RootNode()
	return root.String(), root.HasError(), nil
}

func (s *session) Close() { s.p.Close() }
