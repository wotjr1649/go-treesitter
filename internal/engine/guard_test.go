package engine

import (
	"errors"
	"runtime/debug"
	"sync"
	"testing"
	"unsafe"
)

var sink byte

// peek and poke access guarded memory exactly like the generated code does: a base pointer plus offset, no bounds check.
func peek(m *[]byte, off uint64) byte {
	b := unsafe.Pointer(unsafe.SliceData(*m))
	return *(*byte)(unsafe.Add(b, off))
}

func poke(m *[]byte, off uint64, v byte) {
	b := unsafe.Pointer(unsafe.SliceData(*m))
	*(*byte)(unsafe.Add(b, off)) = v
}

// newGuarded mimics the generated New: a 128 KiB guarded memory, limit MinLimit, 4 more pages grown.
func newGuarded(t *testing.T) *[]byte {
	t.Helper()
	m := new([]byte)
	if err := Call(func() { InitGuardedMemory(m, 0x20000); SetLimit(m, MinLimit); StartMemory(m) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { Release(m) })
	if got := MemoryGrow(m, 4, 65536); got != 2 {
		t.Fatalf("grow = %d", got)
	}
	return m
}

func faultAddr(t *testing.T, err error) uintptr {
	t.Helper()
	var tr *Trap
	if !errors.As(err, &tr) {
		t.Fatalf("want *Trap, got %v", err)
	}
	a, ok := tr.Value.(interface{ Addr() uintptr })
	if !ok {
		t.Fatalf("trap value %T has no Addr: %v", tr.Value, tr.Value)
	}
	return a.Addr()
}

func TestGuardedAccessOutsideCommittedTraps(t *testing.T) {
	m := newGuarded(t)
	base := uint64(base(m))
	committed := uint64(len(*m))
	if cap(*m) != len(*m) || committed != 6<<16 || Committed(m) != 6<<16 {
		t.Fatalf("len=%d cap=%d committed=%d", len(*m), cap(*m), Committed(m))
	}
	if err := Call(func() { poke(m, committed-1, 7); sink = peek(m, committed-1); poke(m, 0, 1) }); err != nil {
		t.Fatalf("in-range access trapped: %v", err)
	}
	offs := []uint64{
		committed,                       // first byte after the committed prefix
		committed + 1<<20,               // inside the reservation, uncommitted
		1<<32 - 1,                       // highest wasm32 address
		1<<32 - 1 + 8,                   // address plus the largest offset+size seen in practice (first guard bytes)
		1<<32 + GuardSize - 1,           // last byte of the guard
		1<<32 - 1 + uint64(GuardSize)/2, // middle of the guard
	}
	for _, off := range offs {
		if err := Call(func() { sink = peek(m, off) }); faultAddr(t, err) != uintptr(base+off) {
			t.Errorf("read at +%#x: fault address %#x, want %#x", off, faultAddr(t, err), base+off)
		}
		if err := Call(func() { poke(m, off, 1) }); faultAddr(t, err) != uintptr(base+off) {
			t.Errorf("write at +%#x did not trap at the right address", off)
		}
	}
	// the instance survives a trap and the faulting page becomes usable once it is grown
	if MemoryGrow(m, 1, 65536) != 6 {
		t.Fatal("grow after trap failed")
	}
	if err := Call(func() { poke(m, committed, 9); sink = peek(m, committed) }); err != nil || sink != 9 {
		t.Fatalf("after grow: %v sink=%d", err, sink)
	}
}

func TestCallRestoresPanicOnFault(t *testing.T) {
	if debug.SetPanicOnFault(false) {
		t.Skip("test goroutine already panics on fault")
	}
	m := newGuarded(t)
	Call(func() {})
	if debug.SetPanicOnFault(false) {
		t.Fatal("Call left panic-on-fault set")
	}
	// nested: the inner fault is the inner Call's trap and the outer Call keeps working
	var inner error
	outer := Call(func() {
		inner = Call(func() { sink = peek(m, 1<<31+7) })
		if !debug.SetPanicOnFault(true) {
			t.Error("inner Call cleared the outer scope")
		}
		sink = peek(m, 1<<31+9) // still protected
	})
	faultAddr(t, inner)
	faultAddr(t, outer)
	if debug.SetPanicOnFault(false) {
		t.Fatal("outer Call left panic-on-fault set")
	}
	// a caller that had it set keeps it
	debug.SetPanicOnFault(true)
	Call(func() {})
	if !debug.SetPanicOnFault(false) {
		t.Fatal("Call cleared the caller's setting")
	}
}

func TestFaultsAreGoroutineLocal(t *testing.T) {
	const n = 8
	var wg sync.WaitGroup
	inCall := make(chan struct{})
	release := make(chan struct{})
	outsideSet := make(chan bool, 1)
	go func() { // another goroutine while one is inside Call: its own setting must stay off
		<-inCall
		outsideSet <- debug.SetPanicOnFault(false)
		close(release)
	}()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m := new([]byte)
			if err := Call(func() { InitGuardedMemory(m, 0x20000); SetLimit(m, MinLimit); StartMemory(m) }); err != nil {
				t.Error(err)
				return
			}
			defer Release(m)
			base := uint64(base(m))
			for j := 0; j < 200; j++ {
				off := uint64(1<<20 + j*4096)
				err := Call(func() {
					if i == 0 && j == 0 {
						close(inCall)
						<-release
					}
					sink = peek(m, off)
				})
				var tr *Trap
				if !errors.As(err, &tr) {
					t.Errorf("no trap at +%#x", off)
					return
				}
				if a := tr.Value.(interface{ Addr() uintptr }).Addr(); a != uintptr(base+off) {
					t.Errorf("fault address %#x, want %#x", a, base+off)
					return
				}
			}
		}()
	}
	wg.Wait()
	if <-outsideSet {
		t.Error("panic-on-fault leaked into a goroutine outside Call")
	}
}

func TestInitGuardedMemoryContract(t *testing.T) {
	m := new([]byte)
	InitGuardedMemory(m, 0x20000)
	if err := SetLimit(m, MinLimit); err != nil {
		t.Fatal(err)
	}
	StartMemory(m)
	defer Release(m)
	if len(*m) != 0x20000 || cap(*m) != 0x20000 || Committed(m) != 0x20000 {
		t.Fatalf("len=%d cap=%d committed=%d", len(*m), cap(*m), Committed(m))
	}
	p := base(m)
	const pages = MinLimit >> 16
	if got := MemoryGrow(m, pages-2, 65536); got != 2 || len(*m) != MinLimit || base(m) != p {
		t.Fatalf("grow to the limit = %d len=%d moved=%v", got, len(*m), base(m) != p)
	}
	if got := MemoryGrow(m, 1, 65536); got != -1 || !Refused(m) || len(*m) != MinLimit {
		t.Fatalf("grow past the limit = %d refused=%v len=%d", got, Refused(m), len(*m))
	}
	if err := SetLimit(m, 2*MinLimit); err == nil {
		t.Fatal("SetLimit after a grow must fail")
	}
	// a second reservation of the same instance and a size that is not a page multiple are refused, as traps
	for name, fn := range map[string]func(){
		"twice":  func() { InitGuardedMemory(m, 0x20000); StartMemory(m) },
		"unpage": func() { n := new([]byte); InitGuardedMemory(n, 0x20001); StartMemory(n) },
	} {
		err := Call(fn)
		var tr *Trap
		if !errors.As(err, &tr) || !errors.Is(tr.Value.(error), ErrReserve) {
			t.Errorf("%s: got %v, want ErrReserve trap", name, err)
		}
	}
	checkRelease(t, m)
}

func TestGuardedReleaseReturnsRegion(t *testing.T) {
	m := newGuarded(t)
	p := base(m)
	if !regionCommitted(p) {
		t.Fatal("not committed before Release")
	}
	checkRelease(t, m)
}
