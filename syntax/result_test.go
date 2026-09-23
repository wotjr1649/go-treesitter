package syntax

import "testing"

func TestOutcomeAndCompleteness(t *testing.T) {
	for _, o := range []Outcome{AcceptedClean, AcceptedWithErrors, EarlyStop, Timeout,
		Cancelled, ResourceLimit, InvariantViolation, UnsupportedInput, NotRun} {
		if !o.Valid() {
			t.Fatalf("invalid declared outcome %q", o)
		}
	}
	if Outcome("").Valid() || Outcome("success").Valid() {
		t.Fatal("open outcome set")
	}
	good := Diagnostics{Outcome: AcceptedClean, InputBytes: 8, RootPresent: true,
		RootEndByte: 8, LastTokenEndByte: 8, ExpectedEOFByte: 8, StopReason: "accepted", SnapshotComplete: true}
	if !(Result{Diagnostics: good}).Complete() {
		t.Fatal("complete receipt rejected")
	}
	for _, mutate := range []func(*Diagnostics){
		func(d *Diagnostics) { d.Outcome = Timeout },
		func(d *Diagnostics) { d.StoppedEarly = true },
		func(d *Diagnostics) { d.Truncated = true },
		func(d *Diagnostics) { d.StopReason = "timeout" },
		func(d *Diagnostics) { d.RootPresent = false },
		func(d *Diagnostics) { d.RootEndByte = 7 },
		func(d *Diagnostics) { d.RootStartByte = 9 },
		func(d *Diagnostics) { d.LastTokenEndByte = 7 },
		func(d *Diagnostics) { d.ExpectedEOFByte = 7 },
		func(d *Diagnostics) { d.SnapshotComplete = false },
	} {
		d := good
		mutate(&d)
		if (Result{Diagnostics: d}).Complete() {
			t.Fatalf("incomplete receipt accepted: %+v", d)
		}
	}
	good.RootStartByte = 2
	if !(Result{Diagnostics: good}).Complete() {
		t.Fatal("grammar-owned leading trivia rejected despite complete EOF receipt")
	}
}
