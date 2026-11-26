// Package harness is the part shared by the comparison orchestrator and every candidate adapter: the worker command
// line and result schema, the edit construction, and the public-S-expression comparison profiles. It imports no
// candidate library (those live in the adapter modules), so it builds and tests without a C compiler or network.
package harness

import (
	"bytes"
	"errors"
	"fmt"
)

// Schema versions the Result and workload JSON; the collector rejects other values.
const Schema = 1

// ErrUnsupported marks a capability the candidate's public API lacks (a named cell in the report, not a failure).
var ErrUnsupported = errors.New("unsupported by the public API")

// Edit is one byte-based edit with the points derived from the same bytes (row = count of '\n', column = bytes since
// the last '\n'), the convention of treesitter.EditFor and of the native reference driver.
type Edit struct {
	StartByte, OldEndByte, NewEndByte uint32
	StartRow, StartCol                uint32
	OldEndRow, OldEndCol              uint32
	NewEndRow, NewEndCol              uint32
}

func pointAt(b []byte, off uint32) (row, col uint32) {
	pre := b[:off]
	row = uint32(bytes.Count(pre, []byte{'\n'}))
	return row, off - uint32(bytes.LastIndexByte(pre, '\n')+1)
}

// MakeEdit builds the Edit that turns old into new and rejects an inconsistent triple: offsets out of range, an end
// before the start, or bytes outside [start, oldEnd) / [start, newEnd) that differ between the two texts.
func MakeEdit(old, new []byte, start, oldEnd, newEnd uint32) (Edit, error) {
	switch {
	case start > oldEnd || start > newEnd:
		return Edit{}, fmt.Errorf("edit start %d is after an end (%d, %d)", start, oldEnd, newEnd)
	case int64(oldEnd) > int64(len(old)) || int64(newEnd) > int64(len(new)):
		return Edit{}, fmt.Errorf("edit ends (%d, %d) exceed the texts (%d, %d)", oldEnd, newEnd, len(old), len(new))
	case !bytes.Equal(old[:start], new[:start]):
		return Edit{}, errors.New("texts differ before the edit start")
	case !bytes.Equal(old[oldEnd:], new[newEnd:]):
		return Edit{}, errors.New("texts differ after the edit end")
	}
	e := Edit{StartByte: start, OldEndByte: oldEnd, NewEndByte: newEnd}
	e.StartRow, e.StartCol = pointAt(old, start)
	e.OldEndRow, e.OldEndCol = pointAt(old, oldEnd)
	e.NewEndRow, e.NewEndCol = pointAt(new, newEnd)
	return e, nil
}

// Result is the one JSON line a worker prints. Samples and counters are raw; nothing is summarized here.
type Result struct {
	Schema   int    `json:"schema"`
	Adapter  string `json:"adapter"`
	Lang     string `json:"lang"`
	Op       string `json:"op"`   // parse | snapshot | edit | sexp | esexp | ctl
	Mode     string `json:"mode"` // time | alloc | rss | out
	Src      string `json:"src"`
	Bytes    int    `json:"bytes"`
	Reps     int    `json:"reps"`
	Status   string `json:"status"` // ok | unsupported | error
	Error    string `json:"error,omitempty"`
	SetupNs  int64  `json:"setup_ns,omitempty"`
	WarmupNs int64  `json:"warmup_ns,omitempty"`

	SamplesNs  []int64 `json:"samples_ns,omitempty"`
	Mallocs    uint64  `json:"mallocs,omitempty"` // Go-heap counters summed over Reps operations (ReadMemStats deltas)
	TotalAlloc uint64  `json:"total_alloc,omitempty"`
	PeakRSSKiB int64   `json:"peak_rss_kib,omitempty"` // VmHWM of the worker at the end (Linux); 0 elsewhere

	HasError  bool   `json:"has_error,omitempty"`
	OutSHA256 string `json:"out_sha256,omitempty"`
	OutBytes  int    `json:"out_bytes,omitempty"`

	Identity map[string]string `json:"identity,omitempty"` // library/grammar versions the adapter reports about itself
}

// Tree is whatever the candidate's public API returns for a parsed document.
type Tree any

// Session is one parser with one loaded language. Every method uses only the candidate's public API; a missing
// capability returns ErrUnsupported (the zero Base does so for the optional ones).
type Session interface {
	Identity() map[string]string
	Parse(src []byte) (Tree, error)         // live-tree parse
	ParseSnapshot(src []byte) (Tree, error) // owned snapshot parse
	Edit(t Tree, e Edit) error              // apply the edit to the old tree in place
	Reparse(src []byte, old Tree) (Tree, error)
	Release(t Tree)
	// Sexp returns the public S-expression of the tree (the adapter builds it from node fields only when the library
	// has no such call) and whether the root reports an error.
	Sexp(t Tree) (string, bool, error)
	Close()
}

// Base supplies ErrUnsupported for the optional operations.
type Base struct{}

func (Base) ParseSnapshot([]byte) (Tree, error) { return nil, ErrUnsupported }
func (Base) Edit(Tree, Edit) error              { return ErrUnsupported }
func (Base) Reparse([]byte, Tree) (Tree, error) { return nil, ErrUnsupported }
func (Base) Release(Tree)                       {}
func (Base) Close()                             {}

// Opener creates the session for a language name (c, go, python, javascript, json) or returns ErrUnsupported.
type Opener func(lang string) (Session, error)
