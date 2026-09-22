package syntax

import (
	"context"
	"time"
)

// Edit uses UTF-8 byte offsets; the adapter derives and validates point offsets.
type Edit struct{ StartByte, OldEndByte, NewEndByte uint32 }

// Limits bounds one request. Zero fields preserve the upstream defaults, or
// impose no additional adapter cap. Negative fields are invalid.
// MemoryBudgetBytes caps the reported runtime arena/scratch footprint and also
// configures the upstream growth budget. It does not bound the Go process,
// grammar caches, source copies or result snapshots. Work limits are checked at
// runtime checkpoints and can be exceeded by a single step.
type Limits struct {
	MaxInputBytes, MaxSnapshotNodes            int
	MemoryBudgetBytes                          int64
	IterationLimit, NodeLimit, StackDepthLimit int
}

type Request struct {
	Filename string
	Language string
	Source   []byte
	// Previous is consumed for editing; close it separately after Parse returns.
	Previous Tree
	Edit     *Edit
	// Timeout bounds the parse API; grammar loading is outside this budget.
	Timeout time.Duration
	Limits  Limits
}

type Parser interface {
	Parse(context.Context, Request) (Result, error)
}
