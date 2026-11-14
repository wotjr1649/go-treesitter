// Package engine is the frozen, versioned ABI between the treesitter package and the generated grammar modules
// (docs/design/engine-abi.md). Generated code implements Module; the treesitter package drives it.
package engine

import (
	"fmt"
	"sync/atomic"
)

// ABIVersion is the engine ABI this root module speaks. A grammar module generated for another version is rejected.
const ABIVersion = 1

// Export chunk buffer header (snapshot block format) and language meta block identifiers.
const (
	ExportMagic  = "TSGX"
	ExportFormat = 1
	MetaMagic    = "TSGM"
	MetaFormat   = 1
)

// Memory is an alias so that it is identical to the interface type wasm2go emits.
type Memory = interface {
	Slice() *[]byte
	Grow(delta, max int64) int64
}

// Module is implemented by the generated *Module of every grammar module. Pointers are wasm32 offsets.
type Module interface {
	X_initialize()
	Xmemory() Memory
	Xmalloc(n int32) int32
	Xfree(p int32)

	Xtsg_abi_version() int32
	Xtsg_init() int32
	Xtsg_parse(src, n, old int32) int32
	Xtsg_parser_reset()
	Xtsg_root_end(t int32) int32
	Xtsg_root_has_error(t int32) int32
	Xtsg_descendant_count(t int32) int32
	Xtsg_tree_edit(t, startByte, oldEndByte, newEndByte, startRow, startCol, oldEndRow, oldEndCol, newEndRow, newEndCol int32)
	Xtsg_tree_delete(t int32)
	Xtsg_changed_ranges(old, neu int32) int32
	Xtsg_export_begin(t int32) int32
	Xtsg_export_next() int32
	Xtsg_export_err() int32
	Xtsg_export_buf() int32
	Xtsg_export_end()
}

// Host implements the instance's imports: tsg.should_stop and the WASI preview1 stubs.
// Set Stop to non-zero to halt a parse at its next progress callback.
type Host struct{ Stop atomic.Int32 }

const enosys = 52

func (h *Host) Xshould_stop() int32                      { return h.Stop.Load() }
func (*Host) Xclock_time_get(int32, int64, int32) int32  { return enosys }
func (*Host) Xfd_close(int32) int32                      { return enosys }
func (*Host) Xfd_seek(int32, int64, int32, int32) int32  { return enosys }
func (*Host) Xfd_write(int32, int32, int32, int32) int32 { return enosys }

// Grammar is the static description a generated grammar module hands to treesitter.NewLanguage.
type Grammar struct {
	ABI      uint32 // literal ABI version the module was generated for
	Name     string
	New      func(h *Host) Module
	Meta     LangMeta
	Identity Identity
}

// LangMeta is the language's static name data (from the meta block at generation time).
type LangMeta struct {
	LanguageABI uint32
	Symbols     []string
	SymbolFlags []uint8  // MetaNamed | MetaVisible | MetaSupertype
	Fields      []string // index 0 is ""
}

const (
	MetaNamed     = 1
	MetaVisible   = 2
	MetaSupertype = 4
)

// Identity binds a grammar module to the exact inputs it was generated from.
type Identity struct {
	Runtime      string // tree-sitter release, e.g. "v0.27.2"
	GrammarRepo  string
	GrammarRev   string // full commit
	SourceSHA256 map[string]string
	RecipeSHA256 string
	Tools        map[string]string // wasi-sdk, wasm2go, go
}

// Trap is a panic raised by generated code (wasm unreachable, out-of-bounds access), captured by Call.
type Trap struct {
	Value any
	Stack []byte
}

func (t *Trap) Error() string { return fmt.Sprintf("engine trap: %v", t.Value) }
