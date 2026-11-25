package conformance

import (
	"bytes"
	"errors"
	"testing"

	treesitter "github.com/wotjr1649/go-treesitter"
)

// TestMemoryLimitRealEngine drives WithMaxMemoryBytes through the real engine (the root package tests only fake the
// refused memory.grow): the smallest limit fails a large parse with ResourceLimit and an instance reset, the parser
// stays usable, and a roomier limit parses the same input. It needs no corpus data; json is the smallest grammar.
func TestMemoryLimitRealEngine(t *testing.T) {
	mk := languages["json"]
	if mk == nil {
		t.Skip("json grammar is not linked")
	}
	const minLimit = 32 << 20 // the engine's floor, per the S39 R2 measurement
	big := bytes.Repeat([]byte("[1,2,3,{\"a\":[4,5]}],"), 200000)
	big = append(append([]byte("["), big...), '0', ']') // ~4 MB, far more than 32 MiB of tree and stack
	small := []byte("[1,2,3]")

	newP := func(t *testing.T, opts ...treesitter.Option) *treesitter.Parser {
		t.Helper()
		p, err := treesitter.NewParser(mk(), opts...)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { p.Close() })
		return p
	}

	t.Run("below the floor", func(t *testing.T) {
		if _, err := treesitter.NewParser(mk(), treesitter.WithMaxMemoryBytes(minLimit-1)); !errors.Is(err, treesitter.ErrInvalidInput) {
			t.Fatalf("NewParser below %d bytes: err = %v, want ErrInvalidInput", minLimit, err)
		}
	})

	t.Run("refused grow", func(t *testing.T) {
		p := newP(t, treesitter.WithMaxMemoryBytes(minLimit))
		assertRefusedGrow := func(label string) {
			t.Helper()
			tr, err := p.Parse(bg, big, nil)
			var pe *treesitter.ParseError
			if tr != nil || !errors.As(err, &pe) || !errors.Is(err, treesitter.ErrResourceLimit) || pe.Outcome != treesitter.ResourceLimit || !pe.InstanceReset {
				t.Fatalf("%s: tree = %p, err = %#v, want nil tree and ResourceLimit with InstanceReset", label, tr, err)
			}
		}
		before, err := p.Parse(bg, small, nil)
		if err != nil {
			t.Fatal(err)
		}
		assertRefusedGrow("first large parse")
		if _, err := before.Snapshot(bg); !errors.Is(err, treesitter.ErrTreeInvalidated) {
			t.Fatalf("tree from before the reset: err = %v, want ErrTreeInvalidated", err)
		}
		tr, err := p.Parse(bg, small, nil) // the recreated instance is reusable
		if err != nil {
			t.Fatalf("parse after the reset: %v", err)
		}
		if tr.Outcome() != treesitter.AcceptedClean {
			t.Fatalf("parse after the reset: outcome %v", tr.Outcome())
		}
		assertRefusedGrow("second large parse")
	})

	t.Run("enough memory", func(t *testing.T) {
		p := newP(t, treesitter.WithMaxMemoryBytes(1<<30))
		tr, err := p.Parse(bg, big, nil)
		if err != nil {
			t.Fatalf("large parse at 1 GiB: %v", err)
		}
		if tr.Outcome() != treesitter.AcceptedClean {
			t.Fatalf("large parse: outcome %v", tr.Outcome())
		}
		tr.Close()
	})
}
