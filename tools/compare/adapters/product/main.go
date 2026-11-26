// Command product is the worker for this repository's released root module and grammar modules (public API only).
package main

import (
	"context"

	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/grammars/c"
	"github.com/wotjr1649/go-treesitter/grammars/golang"
	"github.com/wotjr1649/go-treesitter/grammars/javascript"
	"github.com/wotjr1649/go-treesitter/grammars/json"
	"github.com/wotjr1649/go-treesitter/grammars/python"
	"github.com/wotjr1649/go-treesitter/tools/compare/harness"
)

func main() { harness.Main("product", open) }

// doc is a live tree or an owned snapshot; Release frees the live one.
type doc struct {
	tree *treesitter.Tree
	snap *treesitter.Snapshot
}

type session struct {
	harness.Base
	p    *treesitter.Parser
	lang string
}

func open(lang string) (harness.Session, error) {
	var l *treesitter.Language
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
	p, err := treesitter.NewParser(l)
	if err != nil {
		return nil, err
	}
	return &session{p: p, lang: lang}, nil
}

func (s *session) Identity() map[string]string {
	return harness.BuildIdentity("github.com/wotjr1649/go-treesitter")
}

// Parse is the public copying path (Parser.Parse), not ParseOwned.
func (s *session) Parse(src []byte) (harness.Tree, error) {
	t, err := s.p.Parse(context.Background(), src, nil)
	if err != nil {
		return nil, err
	}
	return &doc{tree: t}, nil
}

func (s *session) ParseSnapshot(src []byte) (harness.Tree, error) {
	sn, err := s.p.ParseSnapshot(context.Background(), src)
	if err != nil {
		return nil, err
	}
	return &doc{snap: sn}, nil
}

func (s *session) Edit(t harness.Tree, e harness.Edit) error {
	return t.(*doc).tree.Edit(treesitter.InputEdit{
		StartByte: e.StartByte, OldEndByte: e.OldEndByte, NewEndByte: e.NewEndByte,
		StartPoint:  treesitter.Point{Row: e.StartRow, Column: e.StartCol},
		OldEndPoint: treesitter.Point{Row: e.OldEndRow, Column: e.OldEndCol},
		NewEndPoint: treesitter.Point{Row: e.NewEndRow, Column: e.NewEndCol},
	})
}

func (s *session) Reparse(src []byte, old harness.Tree) (harness.Tree, error) {
	t, err := s.p.Parse(context.Background(), src, old.(*doc).tree)
	if err != nil {
		return nil, err
	}
	return &doc{tree: t}, nil
}

func (s *session) Release(t harness.Tree) {
	if d, ok := t.(*doc); ok && d.tree != nil {
		d.tree.Close()
	}
}

func (s *session) Sexp(t harness.Tree) (string, bool, error) {
	d := t.(*doc)
	sn := d.snap
	if sn == nil {
		var err error
		if sn, err = d.tree.Snapshot(context.Background()); err != nil {
			return "", false, err
		}
	}
	root := sn.Root()
	return root.String(), root.HasError(), nil
}

func (s *session) Close() { s.p.Close() }
