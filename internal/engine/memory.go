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
	"weak"
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
	pending   *initMem // set by InitMemory, consumed by StartMemory
	cleanup   runtime.Cleanup
	obj       weak.Pointer[[]byte] // the module that owns this state; nil once it was collected
}

// Segment is one active data segment of the initial linear memory.
type Segment struct {
	Dest int
	Data string
}

type initMem struct {
	size int
	segs []Segment
}

// live maps uintptr(mem) -> *state. A uintptr key keeps the module collectable so the AddCleanup safety net can fire.
// A collected module's state stays in the map until its cleanup runs, and a new module can get the same address in that
// time: state.obj tells the two apart, so the new module never inherits a dead one's flags, limit or region (and its
// cleanup, which would free the new module's region).
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

func (s *state) free() error {
	if s.reserved {
		err := s.reg.release()
		s.reserved, s.committed = false, 0
		return err
	}
	return nil
}

func stateOf(mem *[]byte, create bool) *state {
	key := uintptr(unsafe.Pointer(mem))
	if v, ok := live.Load(key); ok {
		st := v.(*state)
		if st.obj.Value() == mem {
			return st
		}
		// the module that owned this address was collected and the address reused; its own cleanup frees its region
		live.CompareAndDelete(key, v)
	}
	if !create {
		return nil
	}
	st := &state{obj: weak.Make(mem)}
	live.Store(key, st)
	st.cleanup = runtime.AddCleanup(mem, drop, dropArg{key, st})
	return st
}

// SetLimit sets the instance's memory cap in bytes. Call it after New and before the first grow or StartMemory
// (both happen in X_initialize).
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

// open reserves the region for limit, commits its first n bytes and records it; the caller sets *mem.
func (st *state) open(limit int64, n int) ([]byte, int, error) {
	size := int((limit + pageSize - 1) &^ (pageSize - 1))
	if err := st.reg.reserve(size); err != nil {
		return nil, 0, err
	}
	if err := st.reg.commit(0, n); err != nil {
		st.reg.release()
		return nil, 0, err
	}
	st.reserved, st.committed = true, int64(n)
	return st.reg.slice(), size, nil
}

func (st *state) limitOrDefault() int64 {
	if st.limit == 0 {
		return DefaultLimit
	}
	return st.limit
}

// InitMemory is called by the generated New instead of allocating the initial memory and copying the data segments
// into it: it only records them (*mem stays nil, the segments are not copied). StartMemory, called at the start of
// X_initialize after SetLimit, creates the reserved region and writes the segments straight into it, so the data is
// copied once and no initial-size Go-heap slice exists.
func InitMemory(mem *[]byte, size int, segs ...Segment) {
	st := stateOf(mem, true)
	st.mu.Lock()
	st.pending = &initMem{size, segs}
	st.mu.Unlock()
}

// StartMemory materializes the memory recorded by InitMemory; it does nothing when none is pending (a second call, or
// a module generated before InitMemory existed). An initial size above the limit, a refused reservation or a segment
// outside the initial memory panics (a trap in Call); the first two set the refused flag, like a refused grow.
func StartMemory(mem *[]byte) {
	st := stateOf(mem, false)
	if st == nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	p := st.pending
	if p == nil {
		return
	}
	st.pending = nil
	limit := st.limitOrDefault()
	if int64(p.size) > limit {
		st.refused = true
		panic(fmt.Sprintf("engine: initial memory of %d bytes exceeds the limit of %d", p.size, limit))
	}
	if p.size == 0 {
		return
	}
	s, size, err := st.open(limit, p.size)
	if err != nil {
		st.refused = true
		panic(fmt.Sprintf("engine: initial memory: %v", err))
	}
	for _, sg := range p.segs {
		if sg.Dest < 0 || sg.Dest > p.size || len(sg.Data) > p.size-sg.Dest {
			st.free()
			panic(fmt.Sprintf("engine: data segment [%d,+%d) outside the initial memory of %d bytes", sg.Dest, len(sg.Data), p.size))
		}
		copy(s[sg.Dest:], sg.Data)
	}
	*mem = s[:p.size:size]
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
	limit := st.limitOrDefault()
	if int64(nl) > limit {
		return refuse()
	}
	if st.reserved {
		if st.reg.commit(len(buf), nl-len(buf)) != nil {
			return refuse()
		}
		*mem = buf[:nl]
	} else {
		s, size, err := st.open(limit, nl)
		if err != nil {
			return refuse()
		}
		copy(s, buf)
		*mem = s[:nl:size]
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
	release(mem)
}

// release returns the OS result so tests can check the release before any address reuse can affect the result.
func release(mem *[]byte) (released bool, err error) {
	st := stateOf(mem, false)
	if st == nil {
		return false, nil
	}
	st.mu.Lock()
	if st.reserved {
		err = st.free()
		released = err == nil
		*mem = nil
	}
	st.mu.Unlock()
	st.cleanup.Stop()
	live.CompareAndDelete(uintptr(unsafe.Pointer(mem)), st)
	return released, err
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
