package main

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"time"

	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/syntax"
)

func main() {
	p := treesitter.New()
	before := []byte("package p\nfunc f() int { return 1 }\n")
	first, err := p.Parse(context.Background(), syntax.Request{Filename: "x.go", Source: before, Timeout: time.Second})
	if err != nil || !first.Complete() || first.Outcome != syntax.AcceptedClean {
		panic("fresh parse failed")
	}
	invalid, err := p.Parse(context.Background(), syntax.Request{Filename: "x.go", Source: before,
		Previous: first.Tree, Edit: &syntax.Edit{StartByte: uint32(len(before) + 1)}})
	if err == nil || invalid.Outcome != syntax.NotRun || invalid.Tree != nil {
		panic("invalid edit was ignored")
	}
	replacement := []byte("x := 22\nreturn x")
	after := bytes.Replace(before, []byte("return 1"), replacement, 1)
	offset := uint32(bytes.Index(before, []byte("return 1")))
	inc, err := p.Parse(context.Background(), syntax.Request{Filename: "x.go", Source: after,
		Previous: first.Tree, Edit: &syntax.Edit{StartByte: offset, OldEndByte: offset + uint32(len("return 1")), NewEndByte: offset + uint32(len(replacement))}, Timeout: time.Second})
	if err != nil || !inc.Complete() || inc.Outcome != syntax.AcceptedClean {
		panic("incremental parse failed")
	}
	defer inc.Tree.Close()
	first.Tree.Close()
	fresh, err := p.Parse(context.Background(), syntax.Request{Filename: "x.go", Source: after, Timeout: time.Second})
	if err != nil || !fresh.Complete() || fresh.Outcome != syntax.AcceptedClean {
		panic("edited fresh parse failed")
	}
	defer fresh.Tree.Close()
	if !reflect.DeepEqual(inc.Tree.Nodes(), fresh.Tree.Nodes()) {
		panic("incremental snapshot differs or old-tree release invalidated it")
	}
	index, err := syntax.NewIndex(fresh.Tree.Nodes())
	if err != nil {
		panic("snapshot index failed")
	}
	node, found := index.NodeAt(uint32(bytes.Index(after, []byte("22"))))
	if !found || fresh.Tree.Nodes()[node].Type != "int_literal" {
		panic("position lookup failed")
	}
	identifiers := 0
	for i := range index.OfType("identifier") {
		if fresh.Tree.Nodes()[i].Type != "identifier" {
			panic("type lookup failed")
		}
		identifiers++
	}
	if identifiers == 0 {
		panic("type lookup empty")
	}
	broken, err := p.Parse(context.Background(), syntax.Request{Filename: "x.go", Source: []byte("package p\nfunc f( {\n"), Timeout: time.Second})
	if err != nil || !broken.Complete() || broken.Outcome != syntax.AcceptedWithErrors || broken.Tree == nil {
		panic("usable error tree not exposed")
	}
	broken.Tree.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stopped, err := p.Parse(ctx, syntax.Request{Filename: "x.go", Source: before})
	if err == nil || stopped.Outcome != syntax.Cancelled || stopped.Tree != nil {
		panic("cancellation not propagated")
	}
	fmt.Println("external consumer: fresh, edit, errors, cancellation, release OK")
}
