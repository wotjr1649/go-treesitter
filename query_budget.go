package treesitter

import (
	"context"
	"errors"
	"math"

	"github.com/wotjr1649/go-treesitter/internal/engine"
)

type queryCompileConfig struct {
	source, memory, metadata, regex int64
}

// QueryCompileOption configures NewQueryWithOptions. Zero leaves the corresponding limit unset.
type QueryCompileOption func(*queryCompileConfig) error

// WithMaxQuerySourceBytes caps the query source, including comments and whitespace, before engine creation.
func WithMaxQuerySourceBytes(n int64) QueryCompileOption {
	return func(c *queryCompileConfig) error {
		return setQueryLimit("WithMaxQuerySourceBytes", &c.source, n, hardMaxInput)
	}
}

// WithMaxQueryMemoryBytes caps the scratch instance's linear memory. Matches uses its parser's own limit.
func WithMaxQueryMemoryBytes(n int64) QueryCompileOption {
	return func(c *queryCompileConfig) error {
		if n > 0 && n < engine.MinLimit {
			return perr(InvalidInput, "WithMaxQueryMemoryBytes: below the engine minimum", nil)
		}
		return setQueryLimit("WithMaxQueryMemoryBytes", &c.memory, n, hardMaxMemory)
	}
}

// WithMaxQueryMetadataBytes independently caps serialized query metadata and derived property string copies.
// The engine has already allocated the serialized metadata when the Go decoder checks the cap.
func WithMaxQueryMetadataBytes(n int64) QueryCompileOption {
	return func(c *queryCompileConfig) error {
		return setQueryLimit("WithMaxQueryMetadataBytes", &c.metadata, n, math.MaxUint32)
	}
}

// WithMaxQueryRegexUnits caps cumulative Go regex translation and compilation units.
// Unit counts are implementation-dependent and include conservative reservations for parser and compiler growth.
func WithMaxQueryRegexUnits(n int64) QueryCompileOption {
	return func(c *queryCompileConfig) error {
		return setQueryLimit("WithMaxQueryRegexUnits", &c.regex, n, math.MaxInt64)
	}
}

// NewQueryWithOptions validates pattern with opt-in limits. Context is checked around engine
// calls and during Go validation. Engine query compilation can only stop at the Go call boundary.
// Syntax and predicate errors match ErrInvalidInput and wrap a *QueryError.
func NewQueryWithOptions(ctx context.Context, lang *Language, pattern string, opts ...QueryCompileOption) (*Query, error) {
	var cfg queryCompileConfig
	for _, opt := range opts {
		if opt == nil {
			return nil, perr(InvalidInput, "nil query compile option", nil)
		}
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}
	q, err := newQuery(ctx, lang, pattern, &cfg)
	var syntaxErr *QueryError
	if errors.As(err, &syntaxErr) {
		return nil, perr(InvalidInput, syntaxErr.Error(), syntaxErr)
	}
	return q, err
}

type queryMatchConfig struct {
	captures, results, work int64
}

// QueryMatchOption configures MatchesWithOptions. Zero leaves the corresponding limit unset.
type QueryMatchOption func(*queryMatchConfig) error

// WithMaxQueryCaptures caps the total raw candidate captures, including candidates rejected by predicates.
// The Go decoder checks the cap after the engine returns the candidate and before allocating its Go captures.
func WithMaxQueryCaptures(n int64) QueryMatchOption {
	return func(c *queryMatchConfig) error {
		return setQueryLimit("WithMaxQueryCaptures", &c.captures, n, math.MaxInt64)
	}
}

// WithMaxQueryResults caps accepted matches per iteration. The first excess match ends the stream
// with ErrResourceLimit before it is yielded.
func WithMaxQueryResults(n int64) QueryMatchOption {
	return func(c *queryMatchConfig) error {
		return setQueryLimit("WithMaxQueryResults", &c.results, n, math.MaxInt64)
	}
}

// WithMaxQueryPredicateWork caps Go work across raw candidates, including rejected candidates.
// Units count candidate admission, predicate evaluation, and regex VM work and may vary by implementation.
func WithMaxQueryPredicateWork(n int64) QueryMatchOption {
	return func(c *queryMatchConfig) error {
		return setQueryLimit("WithMaxQueryPredicateWork", &c.work, n, math.MaxInt64)
	}
}

func setQueryLimit(name string, dst *int64, n, max int64) error {
	if err := nonNegative(name, n); err != nil {
		return err
	}
	if n > max {
		return perr(InvalidInput, name+": value outside supported range", nil)
	}
	*dst = n
	return nil
}

// queryMeter latches an interruption so boolean predicate and VM results are usable only on success.
type queryMeter struct {
	ctx       context.Context
	remaining int64
	limited   bool
	what      string
	err       error
}

func newQueryMeter(ctx context.Context, n int64, what string) queryMeter {
	return queryMeter{ctx: ctx, remaining: n, limited: n > 0, what: what}
}

func (m *queryMeter) charge(n int64) bool {
	return m == nil || m.consume(n)
}

func (m *queryMeter) consume(n int64) bool {
	if m.err == nil {
		if m.ctx.Err() != nil {
			m.err = ctxError(m.ctx, m.what+" stopped by the context")
		} else if m.limited {
			if n < 0 || n > m.remaining {
				m.err = perr(ResourceLimit, m.what+" limit exceeded", nil)
			} else {
				m.remaining -= n
			}
		}
	}
	return m.err == nil
}
