package syntax

import (
	"context"
	"time"
)

// Edit uses UTF-8 byte offsets; the adapter derives and validates point offsets.
type Edit struct{ StartByte, OldEndByte, NewEndByte uint32 }

type Request struct {
	Filename string
	Language string
	Source   []byte
	// Previous is consumed for editing; close it separately after Parse returns.
	Previous Tree
	Edit     *Edit
	// Timeout bounds the parse API; grammar loading is outside this budget.
	Timeout time.Duration
}

type Parser interface {
	Parse(context.Context, Request) (Result, error)
}
