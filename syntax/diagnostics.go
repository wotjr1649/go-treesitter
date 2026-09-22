package syntax

// Diagnostics contains no wall-clock measurements or source-bearing error text.
// Empty raw reason strings mean upstream supplied no reason.
type Diagnostics struct {
	Outcome                                     Outcome
	Language                                    string
	InputBytes                                  int
	StopReason                                  string
	StoppedEarly                                bool
	RootPresent                                 bool
	RootStartByte, RootEndByte                  uint32
	HasError, HasMissing                        bool
	ErrorType                                   string
	DeadlineApplied                             bool
	TokensConsumed                              uint64
	LastTokenEndByte                            uint32
	ExpectedEOFByte                             uint64
	Iterations, IterationLimit                  int
	Nodes, NodeLimit                            int
	PeakStackDepth, StackDepthLimit             int
	ArenaBytes, ScratchBytes, MemoryBudgetBytes int64
	Truncated                                   bool
	Route, FallbackReason                       string
	ReuseReason                                 string
	ReusedOldTree                               bool
	// Fresh-route fallback detail is not available per parse upstream.
	FallbackDetailAvailable bool
	// SnapshotComplete distinguishes an inspected tree from a stopped walk.
	// LimitReason is adapter-owned; StopReason remains the raw runtime value.
	SnapshotComplete                 bool
	SnapshotNodes, SnapshotNodeLimit int
	InputLimitBytes                  int
	LimitReason                      string
}
