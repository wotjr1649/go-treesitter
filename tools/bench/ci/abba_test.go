package main

import (
	"math"
	"slices"
	"testing"
)

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

// A linear drift of the host (each position 1% slower than the previous one) must cancel exactly in A,B,B,A.
func TestRoundLogCancelsLinearDrift(t *testing.T) {
	d := func(pos int) float64 { return 100 * (1 + 0.01*float64(pos)) }
	if got := roundLog(d(0), d(1), d(2), d(3)); !near(got, 0, 0.001) { // exact for linear drift in time; ln adds <0.0005
		t.Fatalf("drift leaked into the ratio: %v", got)
	}
	if got := roundLog(100, 120, 120, 100); !near(got, math.Log(1.2), 1e-12) {
		t.Fatalf("pure 20%% slowdown: %v", got)
	}
}

func TestAbbaVerdict(t *testing.T) {
	// mean 0, sd sqrt(0.0002/4), t(4)=2.132: half-width 2.132*sd/sqrt(5) = 0.006742 in log space.
	s := abbaVerdict([]float64{0, 0.01, -0.01, 0, 0}, 0.10)
	if s.Verdict != "PASS" || !near(s.Ratio, 1, 1e-12) || !near(math.Log(s.Hi), 0.0067420, 1e-6) {
		t.Fatalf("%v", s)
	}
	for _, c := range []struct {
		name string
		logs []float64
		want string
	}{
		{"pass", []float64{0.01, 0.02, 0.0, 0.015, 0.005, 0.01}, "PASS"},
		{"regress: clearly 30% slower", []float64{0.26, 0.27, 0.25, 0.265, 0.26}, "REGRESS"},
		{"inconclusive: noisy around the budget", []float64{0.0, 0.2, 0.05, 0.15, 0.1, 0.12}, "INCONCLUSIVE"},
		{"inconclusive: too few rounds", []float64{0.0, 0.0}, "INCONCLUSIVE"},
		{"slower but within budget", []float64{0.05, 0.052, 0.049, 0.051}, "PASS"},
		{"faster is not a regression", []float64{-0.3, -0.31, -0.29}, "PASS"},
	} {
		if got := abbaVerdict(c.logs, 0.10).Verdict; got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}

func TestSpeedMedian(t *testing.T) {
	out := "# lang=json op=parse bytes=10 go=go1.27\n0,99.0,98.0,x\n1,10.0,9.0,x\n2,30.0,9.0,x\n3,20.0,9.0,x\n"
	if v, err := speedMedian(out); err != nil || v != 20 { // the warm-up rep 0 is dropped
		t.Fatalf("%v %v", v, err)
	}
	if _, err := speedMedian("# only a header\n0,1.0,1.0,x\n"); err == nil {
		t.Fatal("no timed rep must be an error")
	}
}

func TestAbbaSequential(t *testing.T) {
	quiet := []float64{0.01, 0.02, 0.0, 0.015, 0.005, 0.01, 0.012, 0.008}
	// A decisive first look is final: a later regression-looking tail is never consulted.
	tail := []float64{0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5}
	if s := abbaSequential(append(slices.Clone(quiet), tail...), 8, 16, 0.10); s.Verdict != "PASS" || s.N != 8 {
		t.Fatalf("first look must stand: %v", s)
	}
	// INCONCLUSIVE at 8 rounds continues: the verdict is the 16-round one, from all 16 values.
	noisy := []float64{0.0, 0.2, 0.05, 0.15, 0.1, 0.12, -0.05, 0.08}
	if abbaVerdict(noisy, 0.10).Verdict != "INCONCLUSIVE" {
		t.Fatal("fixture must be inconclusive at 8")
	}
	calm := []float64{0.05, 0.055, 0.045, 0.05, 0.052, 0.048, 0.05, 0.05}
	all := append(slices.Clone(noisy), calm...)
	if s := abbaSequential(all, 8, 16, 0.10); s != abbaVerdict(all, 0.10) || s.N != 16 {
		t.Fatalf("second look: %v vs %v", s, abbaVerdict(all, 0.10))
	}
	// Only 8 rounds measured so far, or no extension configured: the first look.
	if s := abbaSequential(noisy, 8, 16, 0.10); s.N != 8 || s.Verdict != "INCONCLUSIVE" {
		t.Fatalf("%v", s)
	}
	if s := abbaSequential(all, 8, 8, 0.10); s.N != 8 {
		t.Fatalf("max=min must not extend: %v", s)
	}
}

func TestParseCorpusAndPick(t *testing.T) {
	m, err := parseCorpus("a.in,10,AcceptedClean,9.0;1.0;3.0;2.0\nb.in,20,AcceptedClean,5.0;0.0;0.0\n")
	if err != nil || m["a.in"] != 2 || m["b.in"] != 1e-4 { // warm-up dropped, median, zero clamped for the log
		t.Fatalf("%v %v", m, err)
	}
	if _, err := parseCorpus("a.in,10,ERR:x,-1\n"); err == nil {
		t.Fatal("a failed file must be an error")
	}
	names := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
	if got := pickEvenly(names, 4); !slices.Equal(got, []string{"0", "2", "5", "7"}) {
		t.Fatalf("%v", got)
	}
	if got := pickEvenly(names, 20); len(got) != 10 {
		t.Fatalf("%v", got)
	}
}
