// Package syntax defines backend-independent parse requests and results.
package syntax

type Outcome string

const (
	AcceptedClean      Outcome = "accepted_clean"
	AcceptedWithErrors Outcome = "accepted_with_errors"
	EarlyStop          Outcome = "early_stop"
	Timeout            Outcome = "timeout"
	Cancelled          Outcome = "cancelled"
	ResourceLimit      Outcome = "resource_limit"
	InvariantViolation Outcome = "invariant_violation"
	UnsupportedInput   Outcome = "unsupported_input"
	NotRun             Outcome = "not_run"
)

func (o Outcome) Valid() bool {
	switch o {
	case AcceptedClean, AcceptedWithErrors, EarlyStop, Timeout, Cancelled,
		ResourceLimit, InvariantViolation, UnsupportedInput, NotRun:
		return true
	}
	return false
}
