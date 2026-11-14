package treesitter

import (
	"context"
	"sync"
)

// ParserPool lends Parsers of one Language to many goroutines. Each lent Parser belongs to its borrower alone (one
// confinement unit); Put frees and invalidates every Tree the borrower left alive before the Parser can be lent
// again, keeping its warm instance. It is safe for concurrent use.
type ParserPool struct {
	lang *Language
	opts []Option

	mu     sync.Mutex
	idle   []*Parser
	lent   map[*Parser]struct{}
	closed bool
	slots  chan struct{} // nil = unbounded
}

// NewParserPool creates a pool; maxParsers > 0 bounds the number of Parsers alive at once (Get then waits for a Put or
// for its context), 0 means unbounded. Options are validated now and applied to every Parser the pool creates.
func NewParserPool(lang *Language, maxParsers int, opts ...Option) (*ParserPool, error) {
	if lang == nil || lang.g == nil || lang.g.New == nil {
		return nil, perr(InvalidInput, "nil or empty language", nil)
	}
	if maxParsers < 0 {
		return nil, perr(InvalidInput, "NewParserPool: negative maxParsers", nil)
	}
	var cfg config
	for _, o := range opts {
		if err := o(&cfg); err != nil {
			return nil, err
		}
	}
	pl := &ParserPool{lang: lang, opts: opts, lent: map[*Parser]struct{}{}}
	if maxParsers > 0 {
		pl.slots = make(chan struct{}, maxParsers)
	}
	return pl, nil
}

// Get lends an idle Parser or creates one. With a bound it waits for a free slot until ctx is done (Timeout/Cancelled).
// Return the Parser with Put; do not Close it.
func (pl *ParserPool) Get(ctx context.Context) (*Parser, error) {
	if ctx == nil {
		return nil, perr(InvalidInput, "nil context", nil)
	}
	if ctx.Err() != nil {
		return nil, ctxError(ctx, "Get: context done")
	}
	if pl.slots != nil {
		select {
		case pl.slots <- struct{}{}:
		case <-ctx.Done():
			return nil, ctxError(ctx, "Get: waiting for a free parser")
		}
	}
	pl.mu.Lock()
	if pl.closed {
		pl.mu.Unlock()
		pl.release()
		return nil, perr(InvalidInput, "parser pool is closed", ErrParserClosed)
	}
	var p *Parser
	if n := len(pl.idle); n > 0 {
		p, pl.idle = pl.idle[n-1], pl.idle[:n-1]
	}
	if p != nil {
		pl.lent[p] = struct{}{}
		pl.mu.Unlock()
		return p, nil
	}
	pl.mu.Unlock()
	p, err := NewParser(pl.lang, pl.opts...)
	if err != nil {
		pl.release()
		return nil, err
	}
	pl.mu.Lock()
	pl.lent[p] = struct{}{}
	pl.mu.Unlock()
	return p, nil
}

func (pl *ParserPool) release() {
	if pl.slots != nil {
		<-pl.slots
	}
}

// Put returns a Parser obtained from Get. Every Tree still alive is freed and invalidated (ErrTreeInvalidated)
// before the Parser can be lent again; Snapshots stay valid. A Parser that was closed, is busy, or was not lent by this
// pool (or was already returned) is not reused. Using a Parser or its Trees after Put is a data race.
func (pl *ParserPool) Put(p *Parser) {
	if p == nil {
		return
	}
	pl.mu.Lock()
	if _, ok := pl.lent[p]; !ok {
		pl.mu.Unlock()
		return
	}
	delete(pl.lent, p)
	pl.mu.Unlock()
	defer pl.release()

	c := p.c
	if !c.busy.CompareAndSwap(false, true) {
		return // still in use elsewhere: not safe to touch, not returned to the pool
	}
	reusable := !c.closed
	if reusable {
		c.dropTrees() // a trap inside has already recreated the instance
	}
	c.end()
	pl.mu.Lock()
	if reusable && !pl.closed {
		pl.idle = append(pl.idle, p)
		pl.mu.Unlock()
		return
	}
	pl.mu.Unlock()
	p.Close()
}

// Close closes the idle Parsers and refuses further Get calls; Parsers still lent are closed when they are Put back.
func (pl *ParserPool) Close() {
	pl.mu.Lock()
	pl.closed = true
	idle := pl.idle
	pl.idle = nil
	pl.mu.Unlock()
	for _, p := range idle {
		p.Close()
	}
}
