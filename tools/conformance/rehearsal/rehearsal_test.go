package rehearsal

import (
	"bytes"
	"strings"
	"testing"
)

func corpus(id, tree, fields string) Row {
	i, e, r, n := NoEdit()
	return Row{"python", id, "corpus", "s-" + id, tree, "10", "false", fields, i, e, r, n}
}

func edit(id, tree, fields, inc, incEq, ranges string) Row {
	return Row{"typescript", id, "edit", "s-" + id, tree, "10", "false", fields, inc, incEq, ranges, "1"}
}

func TestCompareClassifies(t *testing.T) {
	base := []Row{
		corpus("same", "t1", "f1"),
		corpus("fieldsonly", "t2", "f2"),
		corpus("tree", "t3", "f3"),
		edit("e-same", "t4", "f4", "i4", "true", "r4"),
		edit("e-ranges", "t5", "f5", "i5", "true", "r5"),
		edit("e-flip", "t6", "f6", "i6", "false", "r6"),
		edit("e-inc", "t7", "f7", "i7", "true", "r7"),
	}
	cand := []Row{
		corpus("same", "t1", "f1"),
		corpus("fieldsonly", "t2", "FX"),
		corpus("tree", "T3", "F3"), // fields change with the tree: counted as tree only
		edit("e-same", "t4", "f4", "i4", "true", "r4"),
		edit("e-ranges", "t5", "f5", "i5", "true", "RX"),
		edit("e-flip", "t6", "f6", "I6", "true", "r6"),
		edit("e-inc", "t7", "f7", "IX", "true", "r7"),
	}
	res := Compare(base, cand)
	if len(res.Problems) != 0 {
		t.Fatalf("problems: %v", res.Problems)
	}
	got := map[string]string{}
	for _, d := range res.Diffs {
		got[d.ID] = strings.Join(d.Cats, ",")
	}
	want := map[string]string{
		"fieldsonly": "fields", "tree": "tree",
		"e-ranges": "ranges", "e-flip": "incremental,inc-vs-fresh", "e-inc": "incremental",
	}
	if len(got) != len(want) {
		t.Fatalf("diffs %v, want %v", got, want)
	}
	for id, cats := range want {
		if got[id] != cats {
			t.Fatalf("%s: %q, want %q", id, got[id], cats)
		}
	}
	for _, d := range res.Diffs {
		if d.ID == "e-flip" && d.IncFlip != "differs->equal" {
			t.Fatalf("flip %q", d.IncFlip)
		}
	}
	for _, g := range res.Groups {
		switch g.Kind {
		case "corpus":
			if g.Inputs != 3 || g.Tree != 1 || g.Fields != 1 {
				t.Fatalf("corpus group %+v", g)
			}
		case "edit":
			if g.Inputs != 4 || g.Inc != 2 || g.Ranges != 1 || g.IncFlip != 1 || g.BaseIncDiffers != 1 || g.CandIncDiffers != 0 {
				t.Fatalf("edit group %+v", g)
			}
		}
	}
	if s := res.Summary("title line", 1); !strings.Contains(s, "title line") || !strings.Contains(s, "... 1 more in diff.tsv") || strings.Contains(s, "INVALID") {
		t.Fatalf("summary:\n%s", s)
	}
	var d bytes.Buffer
	if err := res.WriteDiffs(&d); err != nil || strings.Count(d.String(), "\n") != 5 {
		t.Fatalf("diffs table %q %v", d.String(), err)
	}
}

func TestCompareProblems(t *testing.T) {
	a := corpus("a", "t", "f")
	drift := a
	drift.Src = "other"
	res := Compare([]Row{a, corpus("only-base", "t", "f")}, []Row{drift, corpus("only-cand", "t", "f")})
	if len(res.Problems) != 3 || len(res.Diffs) != 0 {
		t.Fatalf("problems %v diffs %v", res.Problems, res.Diffs)
	}
	if !strings.Contains(res.Summary("x", 5), "INVALID: 3 problems") {
		t.Fatal("summary hides the problems")
	}
	if p := Compare([]Row{a, a}, []Row{a}).Problems; len(p) != 1 || !strings.Contains(p[0], "duplicate") {
		t.Fatalf("duplicate not reported: %v", p)
	}
}

func TestTableRoundTrip(t *testing.T) {
	rows := []Row{corpus("a", "t", "f"), edit("e", "t", "f", "i", "false", "r")}
	var b bytes.Buffer
	if err := Write(&b, rows); err != nil {
		t.Fatal(err)
	}
	got, err := Parse(&b)
	if err != nil || len(got) != 2 || got[1] != rows[1] {
		t.Fatalf("%v %v", got, err)
	}
	for _, bad := range []string{"", "x\n", Header + "\na\tb\n"} {
		if _, err := Parse(strings.NewReader(bad)); err == nil && bad != "" {
			t.Fatalf("%q accepted", bad)
		}
	}
	if c := Clean("a\tb\nc"); c != "a b c" {
		t.Fatal(c)
	}
}
