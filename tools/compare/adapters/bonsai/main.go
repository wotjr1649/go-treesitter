// Command bonsai is the worker for msuozzo/bonsai v0.4.1 (wasm translated to Go). Parse returns an owned snapshot,
// so the only parse row is "snapshot"; the public API has no live tree, no incremental parse and no S-expression
// call, so the harness serializes the exported node fields (harness.FormatSexp, a documented serialization).
package main

import (
	"errors"

	"github.com/msuozzo/bonsai"
	bonsaic "github.com/msuozzo/bonsai/bonsai-c"
	bonsaigo "github.com/msuozzo/bonsai/bonsai-go"
	bonsaijs "github.com/msuozzo/bonsai/bonsai-javascript"
	bonsaipy "github.com/msuozzo/bonsai/bonsai-python"

	"github.com/wotjr1649/go-treesitter/tools/compare/harness"
)

func main() { harness.Main("bonsai", open) }

type session struct {
	harness.Base
	p *bonsai.Parser
}

func open(lang string) (harness.Session, error) {
	var p *bonsai.Parser
	switch lang {
	case "c":
		p = bonsaic.NewParser()
	case "go":
		p = bonsaigo.NewParser()
	case "javascript":
		p = bonsaijs.NewParser()
	case "python":
		p = bonsaipy.NewParser()
	default: // json: no module
		return nil, harness.ErrUnsupported
	}
	return &session{p: p}, nil
}

func (s *session) Identity() map[string]string {
	return harness.BuildIdentity("github.com/msuozzo/")
}

// Parse has no live-tree form in this library.
func (s *session) Parse([]byte) (harness.Tree, error) { return nil, harness.ErrUnsupported }

func (s *session) ParseSnapshot(src []byte) (harness.Tree, error) {
	n, err := s.p.Parse(src)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, errors.New("Parse returned a nil node")
	}
	return n, nil
}

func (s *session) Sexp(t harness.Tree) (string, bool, error) {
	root := t.(*bonsai.Node)
	return harness.FormatSexp(convert(root)), root.HasError(), nil
}

// convert keeps what ts_node_string prints: named nodes, ERROR nodes and MISSING nodes (an anonymous missing
// node keeps its quoted kind), with their field labels.
func convert(n *bonsai.Node) *harness.SNode {
	k := n.Kind
	if n.IsMissing && !n.Named {
		k = "\"" + k + "\""
	}
	out := &harness.SNode{Field: n.Field, Kind: k, Missing: n.IsMissing, Err: n.IsError}
	for _, c := range n.Children {
		if c.Named || c.IsError || c.IsMissing {
			out.Kids = append(out.Kids, convert(c))
		}
	}
	return out
}
