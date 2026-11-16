package main

import "math"

type smallPass struct{ product, native float64 }

func verdict(pass bool) string {
	if pass {
		return "PASS"
	}
	return "FAIL"
}

func (p smallPass) passed() bool { return p.product <= 1.0 || p.product <= 2.5*p.native }
func (p smallPass) verdict() string {
	if math.IsNaN(p.product) || math.IsNaN(p.native) {
		return "ERROR"
	}
	return verdict(p.passed())
}

// confirmSmall measures at most once more, only after a first-pass threshold failure.
func confirmSmall(first smallPass, measure func() (smallPass, error)) ([]smallPass, bool, error) {
	passes := []smallPass{first}
	if first.passed() {
		return passes, true, nil
	}
	second, err := measure()
	if err != nil {
		return append(passes, smallPass{math.NaN(), math.NaN()}), false, err
	}
	return append(passes, second), second.passed(), nil
}
