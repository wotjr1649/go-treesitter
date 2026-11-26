package main

import (
	"math"
	"slices"
)

func median(xs []float64) float64 { return quantile(xs, 0.5) }

// quantile uses linear interpolation between order statistics; NaN for an empty sample.
func quantile(xs []float64, q float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	s := slices.Clone(xs)
	slices.Sort(s)
	pos := q * float64(len(s)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	return s[lo] + (s[hi]-s[lo])*(pos-float64(lo))
}

func geomean(xs []float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	var sum float64
	for _, x := range xs {
		sum += math.Log(x)
	}
	return math.Exp(sum / float64(len(xs)))
}

// Ratio summarizes the per-round paired ratios of one cell (candidate over product).
type Ratio struct {
	N                  int     `json:"n"`
	Median             float64 `json:"median"`
	Q1                 float64 `json:"q1"`
	Q3                 float64 `json:"q3"`
	Min                float64 `json:"min"`
	Max                float64 `json:"max"`
	Inconclusive       bool    `json:"inconclusive"`
	InconclusiveReason string  `json:"inconclusive_reason,omitempty"`
}

// summarize reports the median, interquartile range and full range of the paired ratios and applies the plan's
// rule: a spread above 20% of the median, or a range that crosses 1x, makes the cell inconclusive for a direction.
func summarize(r []float64) Ratio {
	s := Ratio{N: len(r)}
	if len(r) == 0 {
		s.Inconclusive, s.InconclusiveReason = true, "no complete round"
		return s
	}
	s.Median, s.Q1, s.Q3 = median(r), quantile(r, 0.25), quantile(r, 0.75)
	s.Min, s.Max = slices.Min(r), slices.Max(r)
	switch {
	case len(r) < 3:
		s.Inconclusive, s.InconclusiveReason = true, "fewer than three rounds"
	case (s.Max - s.Min) > 0.2*s.Median:
		s.Inconclusive, s.InconclusiveReason = true, "range above 20% of the median"
	case s.Min < 1 && s.Max > 1:
		s.Inconclusive, s.InconclusiveReason = true, "range crosses 1x"
	}
	return s
}
