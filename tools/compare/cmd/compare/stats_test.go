package main

import (
	"math"
	"testing"
)

func TestSummarizeRules(t *testing.T) {
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	s := summarize([]float64{2.0, 2.1, 2.05, 2.0})
	if s.Inconclusive || !near(s.Median, 2.025) || s.Min != 2.0 || s.Max != 2.1 {
		t.Fatalf("%+v", s)
	}
	if s = summarize([]float64{1.0, 2.0, 3.0}); !s.Inconclusive || s.InconclusiveReason != "range above 20% of the median" {
		t.Fatalf("%+v", s)
	}
	if s = summarize([]float64{0.95, 1.0, 1.05, 1.0}); !s.Inconclusive { // tight (10%) but crossing 1x
		t.Fatalf("%+v", s)
	}
	if s = summarize([]float64{2, 2}); !s.Inconclusive || s.InconclusiveReason != "fewer than three rounds" {
		t.Fatalf("%+v", s)
	}
	if s = summarize(nil); !s.Inconclusive || s.N != 0 {
		t.Fatalf("%+v", s)
	}
	if !near(geomean([]float64{1, 4}), 2) || !near(quantile([]float64{1, 2, 3, 4}, .25), 1.75) {
		t.Fatal("geomean or quantile")
	}
}

func TestTokenAndNewlineEdits(t *testing.T) {
	src := []byte("aa bb ccc ddddd eeeee fff ggggg\nhh ii jj\n")
	st, end, nw, ok := tokenEdit(src)
	if !ok || end-st < 5 || len(nw) != len(src) || nw[end-1] == src[end-1] || string(nw[:st]) != string(src[:st]) {
		t.Fatalf("%d %d %q %v", st, end, nw, ok)
	}
	pos, nl, ok := newlineEdit(src)
	if !ok || nl[pos] != '\n' || src[pos] != ' ' || len(nl) != len(src) {
		t.Fatalf("%d %q %v", pos, nl, ok)
	}
	if _, _, _, ok := tokenEdit([]byte("a b c")); ok {
		t.Fatal("token edit invented a token")
	}
}
