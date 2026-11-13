package treesitter

// engineOps is the seam over the engine package's memory-provider and trap-capture functions
// (docs/design/engine-abi.md section 4). defaultOps (engine_bind.go) binds the real functions; tests install a fake.
type engineOps struct {
	setLimit     func(mem *[]byte, bytes int64) error
	refused      func(mem *[]byte) bool
	clearRefused func(mem *[]byte)
	committed    func(mem *[]byte) int64
	release      func(mem *[]byte)
	call         func(fn func()) error
}

var ops = defaultOps()
