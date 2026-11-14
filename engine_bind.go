package treesitter

import "github.com/wotjr1649/go-treesitter/internal/engine"

// Real binding of the seam to internal/engine (docs/design/engine-abi.md section 4). Tests replace ops with fakes.
func defaultOps() engineOps {
	return engineOps{
		setLimit:     engine.SetLimit,
		refused:      engine.Refused,
		clearRefused: engine.ClearRefused,
		committed:    engine.Committed,
		release:      engine.Release,
		call:         engine.Call,
	}
}
