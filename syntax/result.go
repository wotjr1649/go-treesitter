package syntax

type Point struct{ Row, Column uint32 }

// Node is one entry in a preorder snapshot. Parent is -1 at the root.
// Field names belong to the edge from Parent; child order is never sorted.
// Type may be empty for an anonymous literal NUL terminal, as in the C API.
type Node struct {
	Type, Field                  string
	Parent                       int
	Named, Extra, Missing, Error bool
	StartByte, EndByte           uint32
	StartPoint, EndPoint         Point
}

// Tree owns a backend handle. Close it once when done. Nodes are read-only;
// their snapshot remains independent of backend lifetime. Use one worker only.
type Tree interface {
	Nodes() []Node
	Close()
}

type Result struct {
	Diagnostics
	Tree Tree
}

// Complete checks the whole completion receipt, not just the root span.
func (r Result) Complete() bool {
	return (r.Outcome == AcceptedClean || r.Outcome == AcceptedWithErrors) &&
		r.RootPresent && r.RootStartByte <= r.RootEndByte && uint64(r.RootEndByte) == uint64(r.InputBytes) &&
		uint64(r.LastTokenEndByte) == uint64(r.InputBytes) && r.ExpectedEOFByte == uint64(r.InputBytes) &&
		!r.StoppedEarly && !r.Truncated && r.StopReason == "accepted" && r.SnapshotComplete
}
