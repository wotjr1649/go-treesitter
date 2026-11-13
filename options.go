package treesitter

import "math"

// Hard bounds: node offsets are uint32 and a wasm32 instance cannot address more than 4 GiB.
const (
	hardMaxInput  = math.MaxInt32
	hardMaxMemory = int64(1) << 32
)

type config struct {
	maxInput, maxMemory, maxSnapshotNodes, recycleAbove int64
	strictUTF8                                          bool
}

// Option configures NewParser. Negative or out-of-range values make NewParser fail with InvalidInput; they are never
// clamped. Zero means "no explicit limit" (the engine's own bounds still apply).
type Option func(*config) error

func nonNegative(name string, n int64) error {
	if n < 0 {
		return perr(InvalidInput, name+": negative value", nil)
	}
	return nil
}

// WithMaxInputBytes rejects sources longer than n bytes with ResourceLimit before any engine work.
func WithMaxInputBytes(n int64) Option {
	return func(c *config) error {
		if err := nonNegative("WithMaxInputBytes", n); err != nil {
			return err
		}
		if n > hardMaxInput {
			return perr(InvalidInput, "WithMaxInputBytes: above the 2 GiB offset range", nil)
		}
		c.maxInput = n
		return nil
	}
}

// WithMaxMemoryBytes limits the linear memory of this parser's instance (a refused grow becomes ResourceLimit).
// Values the engine cannot honor make NewParser fail.
func WithMaxMemoryBytes(n int64) Option {
	return func(c *config) error {
		if err := nonNegative("WithMaxMemoryBytes", n); err != nil {
			return err
		}
		if n > hardMaxMemory {
			return perr(InvalidInput, "WithMaxMemoryBytes: above the 4 GiB wasm32 range", nil)
		}
		c.maxMemory = n
		return nil
	}
}

// WithMaxSnapshotNodes rejects exporting a tree with more than n nodes (ResourceLimit, checked before the export).
func WithMaxSnapshotNodes(n int64) Option {
	return func(c *config) error {
		if err := nonNegative("WithMaxSnapshotNodes", n); err != nil {
			return err
		}
		c.maxSnapshotNodes = n
		return nil
	}
}

// WithRecycleAboveBytes recreates the instance, returning its memory to the OS, once a parse has committed more than n
// bytes and the parser holds no live tree (a reset would invalidate them). ParseSnapshot recycles right away.
func WithRecycleAboveBytes(n int64) Option {
	return func(c *config) error {
		if err := nonNegative("WithRecycleAboveBytes", n); err != nil {
			return err
		}
		c.recycleAbove = n
		return nil
	}
}

// WithStrictUTF8 turns invalid UTF-8 input into InvalidInput instead of parsing it like C tree-sitter does.
func WithStrictUTF8() Option {
	return func(c *config) error { c.strictUTF8 = true; return nil }
}
