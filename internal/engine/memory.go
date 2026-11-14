package engine

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
	"runtime"
	"runtime/debug"
	"sync"
	"unsafe"
)

// Linear-memory provider (docs/design/engine-abi.md §4). Each wasm instance keeps its memory in an address-space
// reservation of `limit` bytes; every memory.grow commits only the new tail. The memory is never copied and creates no
// Go-heap garbage. State is keyed by the *[]byte the generated code passes (&Module.memory, stable for the module's life).

const (
	DefaultLimit = 1 << 30 // used when SetLimit was not called
	MinLimit     = 32 << 20
	MaxLimit     = 4 << 30 // wasm32 address space

	pageSize = 1 << 16
)

// ErrLimitRange reports a SetLimit value outside [MinLimit, MaxLimit]; it is never clamped.
var ErrLimitRange = errors.New("engine: memory limit out of range")

type state struct {
	mu        sync.Mutex
	limit     int64 // 0 = DefaultLimit
	reg       osRegion
	reserved  bool
	committed int64
	refused   bool
	cleanup   runtime.Cleanup
}

// live maps uintptr(mem) -> *state. A uintptr key keeps the module collectable so the AddCleanup safety net can fire.
var live sync.Map

type dropArg struct {
	key uintptr
	st  *state
}

// drop is the AddCleanup safety net: frees the region of a module that was dropped without Release.
func drop(a dropArg) {
	a.st.mu.Lock()
	a.st.free()
	a.st.mu.Unlock()
	live.CompareAndDelete(a.key, a.st)
}

func (s *state) free() {
	if s.reserved {
		s.reg.release()
		s.reserved, s.committed = false, 0
	}
}

func stateOf(mem *[]byte, create bool) *state {
	key := uintptr(unsafe.Pointer(mem))
	if v, ok := live.Load(key); ok {
		st := v.(*state)
		st.mu.Lock()
		stale := st.reserved && st.reg.addr() != uintptr(unsafe.Pointer(unsafe.SliceData(*mem)))
		st.mu.Unlock()
		if !stale {
			return st
		}
		// the module that owned this address was collected and the address reused; its own cleanup frees its region
		live.CompareAndDelete(key, v)
	}
	if !create {
		return nil
	}
	st := new(state)
	live.Store(key, st)
	st.cleanup = runtime.AddCleanup(mem, drop, dropArg{key, st})
	return st
}

// SetLimit sets the instance's memory cap in bytes. Call it after New and before the first grow (X_initialize).
func SetLimit(mem *[]byte, bytes int64) error {
	if bytes < MinLimit || bytes > MaxLimit {
		return fmt.Errorf("%w: %d not in [%d, %d]", ErrLimitRange, bytes, int64(MinLimit), int64(MaxLimit))
	}
	st := stateOf(mem, true)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.reserved {
		return errors.New("engine: SetLimit after the instance's memory was reserved")
	}
	st.limit = bytes
	return nil
}

// MemoryGrow replaces the generated memory_grow: same contract (old page count, or -1 on failure) and it sets the
// refused flag on every -1.
func MemoryGrow(mem *[]byte, delta, max int64) int64 {
	buf := *mem
	old := len(buf) >> 16
	if delta == 0 {
		return int64(old)
	}
	st := stateOf(mem, true)
	st.mu.Lock()
	defer st.mu.Unlock()
	refuse := func() int64 { st.refused = true; return -1 }

	max = int64(min(uint64(max), math.MaxInt>>16))
	nw, c := bits.Add64(uint64(old), uint64(delta), 0)
	if c != 0 || nw > uint64(max) {
		return refuse()
	}
	nl := int(nw << 16)
	limit := st.limit
	if limit == 0 {
		limit = DefaultLimit
	}
	if int64(nl) > limit {
		return refuse()
	}
	if st.reserved {
		if st.reg.commit(len(buf), nl-len(buf)) != nil {
			return refuse()
		}
		*mem = buf[:nl]
	} else {
		size := int((limit + pageSize - 1) &^ (pageSize - 1))
		if st.reg.reserve(size) != nil {
			return refuse()
		}
		if st.reg.commit(0, nl) != nil {
			st.reg.release()
			return refuse()
		}
		s := st.reg.slice()
		copy(s, buf)
		*mem = s[:nl:size]
		st.reserved = true
	}
	st.committed = int64(nl)
	return int64(old)
}

// Refused reports whether a grow was refused since the last ClearRefused.
func Refused(mem *[]byte) bool {
	st := stateOf(mem, false)
	if st == nil {
		return false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.refused
}

func ClearRefused(mem *[]byte) {
	if st := stateOf(mem, false); st != nil {
		st.mu.Lock()
		st.refused = false
		st.mu.Unlock()
	}
}

// Committed returns the bytes of linear memory currently committed (0 before the first grow and after Release).
func Committed(mem *[]byte) int64 {
	st := stateOf(mem, false)
	if st == nil {
		return 0
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.committed
}

// Release returns the region to the OS and drops the instance's state; idempotent. *mem is set to nil, so the
// instance must not be used afterwards (a later access panics instead of faulting on unmapped memory).
func Release(mem *[]byte) {
	st := stateOf(mem, false)
	if st == nil {
		return
	}
	st.mu.Lock()
	if st.reserved {
		st.free()
		*mem = nil
	}
	st.mu.Unlock()
	st.cleanup.Stop()
	live.CompareAndDelete(uintptr(unsafe.Pointer(mem)), st)
}

// Call runs fn; any panic from it (generated-code trap) becomes *Trap. A hardware fault on unmapped memory is not a
// panic and cannot be captured; the provider keeps every reachable address either committed or bounds-checked.
func Call(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &Trap{Value: r, Stack: debug.Stack()}
		}
	}()
	fn()
	return nil
}
