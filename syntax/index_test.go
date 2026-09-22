package syntax

import (
	"reflect"
	"slices"
	"testing"
)

func TestIndexBoundariesAndOwnership(t *testing.T) {
	nodes := []Node{
		{Type: "root", Parent: -1, EndByte: 20},
		{Type: "block", Parent: 0, StartByte: 2, EndByte: 18},
		{Type: "identifier", Parent: 1, StartByte: 4, EndByte: 8},
		{Type: "missing", Parent: 1, StartByte: 8, EndByte: 8, Missing: true},
		{Type: "identifier", Parent: 1, StartByte: 8, EndByte: 12},
		{Type: "overlap", Parent: 1, StartByte: 6, EndByte: 14},
	}
	x, err := NewIndex(nodes)
	if err != nil {
		t.Fatal(err)
	}
	for offset, want := range map[uint32]int{0: 0, 1: 0, 2: 1, 4: 2, 7: 2, 8: 4, 11: 4, 12: 5, 14: 1, 18: 0, 19: 0, 20: -1, 100: -1} {
		got, ok := x.NodeAt(offset)
		if got != want || ok != (want >= 0) {
			t.Fatalf("offset=%d got=%d/%t want=%d", offset, got, ok, want)
		}
	}
	if got := slices.Collect(x.OfType("identifier")); !reflect.DeepEqual(got, []int{2, 4}) {
		t.Fatal(got)
	}
	if got := slices.Collect(x.OfType("absent")); len(got) != 0 {
		t.Fatal(got)
	}
	count := 0
	for range x.OfType("identifier") {
		count++
		break
	}
	if count != 1 {
		t.Fatal("iterator did not stop")
	}
	nodes[2].StartByte, nodes[2].Type = 19, "changed"
	if got, _ := x.NodeAt(4); got != 2 {
		t.Fatal("index aliases mutable input")
	}
	if got := slices.Collect(x.OfType("identifier")); len(got) != 2 {
		t.Fatal("type index changed")
	}
}

func TestIndexRejectsMalformedSnapshots(t *testing.T) {
	for _, nodes := range [][]Node{
		{{Type: "root", Parent: 0}},
		{{Type: "root", Parent: -1, StartByte: 2, EndByte: 1}},
		{{Type: "root", Parent: -1}, {Type: "child", Parent: 1}},
		{{Type: "root", Parent: -1}, {Type: "child", Parent: -1}},
		{{Type: "root", Parent: -1}, {Type: "child", Parent: 0, EndByte: 1}},
		{{Parent: -1}},
	} {
		if _, err := NewIndex(nodes); err == nil {
			t.Fatal("malformed snapshot accepted")
		}
	}
	x, err := NewIndex(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := x.NodeAt(0); ok {
		t.Fatal("empty lookup")
	}
}
