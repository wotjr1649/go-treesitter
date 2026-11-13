package treesitter

import (
	"context"
	"errors"
	"fmt"
)

// Outcome classifies the result of a parse (docs/design/api-v2.md section 4).
type Outcome uint8

const (
	// AcceptedClean: a complete tree without syntax errors.
	AcceptedClean Outcome = iota + 1
	// AcceptedWithErrors: a complete tree that contains ERROR or MISSING nodes. This is not a Go error.
	AcceptedWithErrors
	Timeout
	Cancelled
	ResourceLimit
	InvalidInput
	EngineFault
	IncompatibleLanguage
)

var outcomeNames = [...]string{"", "AcceptedClean", "AcceptedWithErrors", "Timeout", "Cancelled", "ResourceLimit",
	"InvalidInput", "EngineFault", "IncompatibleLanguage"}

func (o Outcome) String() string {
	if int(o) < len(outcomeNames) && o != 0 {
		return outcomeNames[o]
	}
	return fmt.Sprintf("Outcome(%d)", uint8(o))
}

type sentinel struct {
	msg  string
	also error // matched by errors.Is as well (context errors)
}

func (s *sentinel) Error() string        { return s.msg }
func (s *sentinel) Is(target error) bool { return s.also != nil && target == s.also }

// Sentinels for errors.Is. ErrTimeout also matches context.DeadlineExceeded, ErrCancelled context.Canceled.
var (
	ErrTimeout              error = &sentinel{"treesitter: timeout", context.DeadlineExceeded}
	ErrCancelled            error = &sentinel{"treesitter: cancelled", context.Canceled}
	ErrResourceLimit              = &sentinel{msg: "treesitter: resource limit"}
	ErrInvalidInput               = &sentinel{msg: "treesitter: invalid input"}
	ErrEngineFault                = &sentinel{msg: "treesitter: engine fault"}
	ErrIncompatibleLanguage       = &sentinel{msg: "treesitter: incompatible language module"}
	ErrTreeInvalidated            = &sentinel{msg: "treesitter: tree invalidated by Close, Reset or an instance reset"}
	ErrTreeEdited                 = &sentinel{msg: "treesitter: tree was edited and is only usable as the old tree of a parse"}
	ErrParserClosed               = &sentinel{msg: "treesitter: parser is closed"}
)

func sentinelFor(o Outcome) error {
	switch o {
	case Timeout:
		return ErrTimeout
	case Cancelled:
		return ErrCancelled
	case ResourceLimit:
		return ErrResourceLimit
	case InvalidInput:
		return ErrInvalidInput
	case EngineFault:
		return ErrEngineFault
	case IncompatibleLanguage:
		return ErrIncompatibleLanguage
	}
	return nil
}

// ParseError reports a parse (or snapshot export) that produced no result. errors.Is matches the sentinel of Outcome
// and, through Unwrap, the context error for Timeout/Cancelled.
type ParseError struct {
	Outcome Outcome
	Reason  string
	// InstanceReset is true when the engine instance was discarded and recreated: every tree of the parser is
	// invalidated (ErrTreeInvalidated).
	InstanceReset bool

	cause error
}

func (e *ParseError) Error() string {
	s := "treesitter: " + e.Outcome.String()
	if e.Reason != "" {
		s += ": " + e.Reason
	}
	if e.InstanceReset {
		s += " (instance reset)"
	}
	return s
}

func (e *ParseError) Is(target error) bool {
	s := sentinelFor(e.Outcome)
	return s != nil && (target == s || errors.Is(s, target))
}

func (e *ParseError) Unwrap() error { return e.cause }

func perr(o Outcome, reason string, cause error) *ParseError {
	return &ParseError{Outcome: o, Reason: reason, cause: cause}
}

// ctxError maps a done context to Timeout or Cancelled.
func ctxError(ctx context.Context, where string) *ParseError {
	err := ctx.Err()
	if errors.Is(err, context.DeadlineExceeded) {
		return perr(Timeout, where, err)
	}
	return perr(Cancelled, where, err)
}
