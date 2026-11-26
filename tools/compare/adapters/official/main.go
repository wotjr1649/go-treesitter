// Command official is the worker for tree-sitter/go-tree-sitter v0.24.0 (cgo; official C runtime and grammar modules).
package main

import (
	"errors"
	"unsafe"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_json "github.com/tree-sitter/tree-sitter-json/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"

	"github.com/wotjr1649/go-treesitter/tools/compare/harness"
)

func main() { harness.Main("official", open) }

type session struct {
	harness.Base
	p *tree_sitter.Parser
}

func open(lang string) (harness.Session, error) {
	var ptr unsafe.Pointer
	switch lang {
	case "c":
		ptr = tree_sitter_c.Language()
	case "go":
		ptr = tree_sitter_go.Language()
	case "javascript":
		ptr = tree_sitter_javascript.Language()
	case "json":
		ptr = tree_sitter_json.Language()
	case "python":
		ptr = tree_sitter_python.Language()
	default:
		return nil, harness.ErrUnsupported
	}
	p := tree_sitter.NewParser()
	if err := p.SetLanguage(tree_sitter.NewLanguage(ptr)); err != nil {
		p.Close()
		return nil, err
	}
	return &session{p: p}, nil
}

func (s *session) Identity() map[string]string {
	return harness.BuildIdentity("github.com/tree-sitter/")
}

func (s *session) Parse(src []byte) (harness.Tree, error) {
	t := s.p.Parse(src, nil)
	if t == nil {
		return nil, errors.New("Parse returned nil")
	}
	return t, nil
}

func (s *session) Edit(t harness.Tree, e harness.Edit) error {
	t.(*tree_sitter.Tree).Edit(&tree_sitter.InputEdit{
		StartByte: uint(e.StartByte), OldEndByte: uint(e.OldEndByte), NewEndByte: uint(e.NewEndByte),
		StartPosition:  tree_sitter.Point{Row: uint(e.StartRow), Column: uint(e.StartCol)},
		OldEndPosition: tree_sitter.Point{Row: uint(e.OldEndRow), Column: uint(e.OldEndCol)},
		NewEndPosition: tree_sitter.Point{Row: uint(e.NewEndRow), Column: uint(e.NewEndCol)},
	})
	return nil
}

func (s *session) Reparse(src []byte, old harness.Tree) (harness.Tree, error) {
	t := s.p.Parse(src, old.(*tree_sitter.Tree))
	if t == nil {
		return nil, errors.New("Parse returned nil")
	}
	return t, nil
}

func (s *session) Release(t harness.Tree) {
	if tr, ok := t.(*tree_sitter.Tree); ok {
		tr.Close()
	}
}

func (s *session) Sexp(t harness.Tree) (string, bool, error) {
	root := t.(*tree_sitter.Tree).RootNode()
	return root.ToSexp(), root.HasError(), nil
}

func (s *session) Close() { s.p.Close() }
