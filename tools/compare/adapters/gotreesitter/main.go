// Command gotreesitter is the worker for odvcencio/gotreesitter v0.58.0 (pure Go reimplementation of the runtime).
package main

import (
	"errors"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars/c"
	golang "github.com/odvcencio/gotreesitter/grammars/go"
	"github.com/odvcencio/gotreesitter/grammars/javascript"
	"github.com/odvcencio/gotreesitter/grammars/json"
	"github.com/odvcencio/gotreesitter/grammars/python"

	"github.com/wotjr1649/go-treesitter/tools/compare/harness"
)

func main() { harness.Main("gotreesitter", open) }

type session struct {
	harness.Base
	p    *gts.Parser
	lang *gts.Language
}

func open(lang string) (harness.Session, error) {
	var l *gts.Language
	switch lang {
	case "c":
		l = c.Language()
	case "go":
		l = golang.Language()
	case "javascript":
		l = javascript.Language()
	case "json":
		l = json.Language()
	case "python":
		l = python.Language()
	default:
		return nil, harness.ErrUnsupported
	}
	return &session{p: gts.NewParser(l), lang: l}, nil
}

func (s *session) Identity() map[string]string {
	return harness.BuildIdentity("github.com/odvcencio/")
}

func (s *session) Parse(src []byte) (harness.Tree, error) { return s.check(s.p.Parse(src)) }

func (s *session) check(t *gts.Tree, err error) (harness.Tree, error) {
	if err != nil {
		if t != nil {
			t.Release() // "release every non-nil tree, even when an error accompanies it"
		}
		return nil, err
	}
	if t == nil {
		return nil, errors.New("parse returned a nil tree")
	}
	return t, nil
}

func (s *session) Edit(t harness.Tree, e harness.Edit) error {
	t.(*gts.Tree).Edit(gts.InputEdit{
		StartByte: e.StartByte, OldEndByte: e.OldEndByte, NewEndByte: e.NewEndByte,
		StartPoint:  gts.Point{Row: e.StartRow, Column: e.StartCol},
		OldEndPoint: gts.Point{Row: e.OldEndRow, Column: e.OldEndCol},
		NewEndPoint: gts.Point{Row: e.NewEndRow, Column: e.NewEndCol},
	})
	return nil
}

func (s *session) Reparse(src []byte, old harness.Tree) (harness.Tree, error) {
	return s.check(s.p.ParseIncremental(src, old.(*gts.Tree)))
}

func (s *session) Release(t harness.Tree) {
	if tr, ok := t.(*gts.Tree); ok {
		tr.Release()
	}
}

func (s *session) Sexp(t harness.Tree) (string, bool, error) {
	root := t.(*gts.Tree).RootNode()
	return root.SExpr(s.lang), root.HasError(), nil
}
