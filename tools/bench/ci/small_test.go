package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestConfirmSmall(t *testing.T) {
	for _, tc := range []struct {
		name          string
		first, second smallPass
		wantPass      bool
		wantCalls     int
	}{
		{"fail/fail", smallPass{2, 0.5}, smallPass{3, 0.5}, false, 1},
		{"fail/pass absolute", smallPass{2, 0.5}, smallPass{1, 0.2}, true, 1},
		{"fail/pass ratio", smallPass{2, 0.5}, smallPass{2, 0.8}, true, 1},
		{"pass absolute", smallPass{1, 0.2}, smallPass{}, true, 0},
		{"pass ratio", smallPass{2, 0.8}, smallPass{}, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			passes, passed, err := confirmSmall(tc.first, func() (smallPass, error) {
				calls++
				return tc.second, nil
			})
			want := []smallPass{tc.first}
			if tc.wantCalls == 1 {
				want = append(want, tc.second)
			}
			if err != nil || passed != tc.wantPass || calls != tc.wantCalls || !reflect.DeepEqual(passes, want) {
				t.Fatalf("got passes=%v pass=%v calls=%d err=%v; want %v pass=%v calls=%d", passes, passed, calls, err, want, tc.wantPass, tc.wantCalls)
			}
			if passes[0].verdict() != verdict(tc.first.passed()) || passes[len(passes)-1].verdict() != verdict(passed) {
				t.Fatalf("incorrect recorded verdicts: %v", passes)
			}
		})
	}
}

func TestConfirmSmallMeasurementError(t *testing.T) {
	first := smallPass{2, 0.5}
	wantErr := errors.New("native exited")
	passes, passed, err := confirmSmall(first, func() (smallPass, error) {
		return smallPass{}, wantErr
	})
	if passed || !errors.Is(err, wantErr) || len(passes) != 2 || passes[0] != first || passes[1].verdict() != "ERROR" {
		t.Fatalf("lost the first failure or ignored measurement error: %v, %v, %v", passes, passed, err)
	}
}
