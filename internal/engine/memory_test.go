package engine

import (
	"bytes"
	"errors"
	"math"
	"math/bits"
	"strings"
	"testing"
	"unsafe"
)

// refMemoryGrow is the original wasm2go v0.4.21 memory_grow body (Go-heap append), the contract MemoryGrow must keep.
func refMemoryGrow(mem *[]byte, delta, max int64) int64 {
	buf := *mem
	l := len(buf)
	old := l >> 16
	if delta == 0 {
		return int64(old)
	}
	max = int64(min(uint64(max), math.MaxInt>>16))
	nw, c := bits.Add64(uint64(old), uint64(delta), 0)
	if c != 0 || nw > uint64(max) {
		return -1
	}
	nl := int(nw << 16)
	*mem = append(buf, make([]byte, nl-l)...)
	return int64(old)
}

// newMem mimics the generated New: a 128 KiB Go-heap slice with a marker byte.
func newMem(t *testing.T) *[]byte {
	t.Helper()
	m := new([]byte)
	*m = make([]byte, 0x20000)
	(*m)[1234] = 0xAB
	t.Cleanup(func() { Release(m) })
	return m
}

func base(m *[]byte) uintptr { return uintptr(unsafe.Pointer(unsafe.SliceData(*m))) }

func checkRelease(t *testing.T, m *[]byte) {
	t.Helper()
	p := base(m)
	if released, err := release(m); !released || err != nil {
		reportReleaseFailure(t, p, err)
	}
	if *m != nil {
		t.Errorf("memory slice after Release is non-nil: len=%d cap=%d", len(*m), cap(*m))
	}
	if n := Committed(m); n != 0 {
		t.Errorf("Committed after Release = %d, want 0", n)
	}
}

func TestGrowWithinLimit(t *testing.T) {
	m := newMem(t)
	if err := SetLimit(m, MinLimit); err != nil {
		t.Fatal(err)
	}
	if got := MemoryGrow(m, 4, 65536); got != 2 {
		t.Fatalf("first grow = %d, want 2", got)
	}
	if len(*m) != 6<<16 || cap(*m) != MinLimit || Committed(m) != 6<<16 {
		t.Fatalf("len=%d cap=%d committed=%d", len(*m), cap(*m), Committed(m))
	}
	if (*m)[1234] != 0xAB {
		t.Fatal("old content lost")
	}
	if !bytes.Equal((*m)[2<<16:], make([]byte, 4<<16)) {
		t.Fatal("new pages not zero")
	}
	p := base(m)
	(*m)[6<<16-1] = 1
	if got := MemoryGrow(m, 1, 65536); got != 6 || base(m) != p {
		t.Fatalf("second grow = %d, moved=%v", got, base(m) != p)
	}
	if (*m)[6<<16-1] != 1 || Refused(m) {
		t.Fatal("content lost or refused set")
	}
}

func TestRefusalAtLimit(t *testing.T) {
	m := newMem(t)
	if err := SetLimit(m, MinLimit); err != nil {
		t.Fatal(err)
	}
	const pages = MinLimit >> 16
	if got := MemoryGrow(m, pages-2, 65536); got != 2 || len(*m) != MinLimit {
		t.Fatalf("grow to exactly the limit = %d len=%d", got, len(*m))
	}
	if Refused(m) {
		t.Fatal("refused before any refusal")
	}
	if got := MemoryGrow(m, 1, 65536); got != -1 || !Refused(m) || len(*m) != MinLimit {
		t.Fatalf("grow past limit = %d refused=%v len=%d", got, Refused(m), len(*m))
	}
	ClearRefused(m)
	if Refused(m) {
		t.Fatal("ClearRefused did not clear")
	}
	if MemoryGrow(m, 0, 65536) != pages || Refused(m) { // delta 0 is a query, never a refusal
		t.Fatal("delta 0 changed state")
	}
	if MemoryGrow(m, 1, pages) != -1 || !Refused(m) { // wasm max also counts as a refusal
		t.Fatal("grow past wasm max not refused")
	}
}

func TestLimitRangeAndDefault(t *testing.T) {
	m := newMem(t)
	for _, b := range []int64{-1, 0, MinLimit - 1, MaxLimit + 1} {
		if err := SetLimit(m, b); !errors.Is(err, ErrLimitRange) {
			t.Errorf("SetLimit(%d) = %v, want ErrLimitRange", b, err)
		}
	}
	// default: 1 GiB reservation; a grow over it is refused without committing anything
	if MemoryGrow(m, 1, 65536) != 2 || cap(*m) != DefaultLimit {
		t.Fatalf("default cap = %d", cap(*m))
	}
	if MemoryGrow(m, DefaultLimit>>16, 65536) != -1 || !Refused(m) {
		t.Fatal("grow over the default limit not refused")
	}
	if err := SetLimit(m, MinLimit); err == nil {
		t.Fatal("SetLimit after reserve must fail")
	}
	m2 := newMem(t)
	if err := SetLimit(m2, MaxLimit); err != nil {
		t.Fatal(err)
	}
	if MemoryGrow(m2, 1, 65536) != 2 || cap(*m2) != MaxLimit {
		t.Fatal("MaxLimit not honoured")
	}
}

func TestInstancesAreIndependent(t *testing.T) {
	a, b := newMem(t), newMem(t)
	SetLimit(a, MinLimit)
	SetLimit(b, 2*MinLimit)
	MemoryGrow(a, MinLimit>>16, 65536) // over a's limit
	if !Refused(a) || Refused(b) {
		t.Fatal("refused flag leaked between instances")
	}
	if MemoryGrow(b, MinLimit>>16, 65536) < 0 {
		t.Fatal("b should accept what a refused")
	}
}

func TestReleaseReturnsRegionAndIsIdempotent(t *testing.T) {
	m := newMem(t)
	SetLimit(m, MinLimit)
	MemoryGrow(m, 8, 65536)
	p := base(m)
	if !regionCommitted(p) || !regionCommitted(p+9<<16) {
		t.Fatal("committed pages not accessible before Release")
	}
	checkRelease(t, m)
	if Refused(m) {
		t.Error("refused flag left behind after Release")
	}
	if released, err := release(m); released || err != nil {
		t.Errorf("second Release: released=%t, err=%v; want false, nil", released, err)
	}
	if _, ok := live.Load(uintptr(unsafe.Pointer(m))); ok {
		t.Fatal("state map entry not dropped")
	}
}

func TestContractEqualsOriginal(t *testing.T) {
	m := newMem(t)
	r := new([]byte)
	*r = make([]byte, 0x20000)
	(*r)[1234] = 0xAB
	const max = 20
	for i, d := range []int64{0, 1, 3, 0, 10, 5, -1, math.MaxInt64, math.MinInt64, 4, 1, 0} {
		want := refMemoryGrow(r, d, max)
		got := MemoryGrow(m, d, max)
		if got != want || len(*m) != len(*r) || !bytes.Equal(*m, *r) {
			t.Fatalf("step %d delta %d: got %d len %d, want %d len %d", i, d, got, len(*m), want, len(*r))
		}
	}
}

func TestCallCapturesPanic(t *testing.T) {
	if err := Call(func() {}); err != nil {
		t.Fatalf("no panic returned %v", err)
	}
	err := Call(func() { panic("unreachable") })
	var tr *Trap
	if !errors.As(err, &tr) || tr.Value != "unreachable" || len(tr.Stack) == 0 {
		t.Fatalf("got %v", err)
	}
	s := make([]byte, 1)
	i := 5
	err = Call(func() { _ = s[i] }) // out-of-bounds, like a wasm load past len(mem)
	if !errors.As(err, &tr) {
		t.Fatalf("runtime panic not captured: %v", err)
	}
}

// proc_exit is a trap: it unwinds through nested frames to Call (the process survives), and the instance's region is
// still returned by Release afterwards.
func TestProcExitTrapsAndRegionIsReleased(t *testing.T) {
	m := newMem(t)
	SetLimit(m, MinLimit)
	if got := MemoryGrow(m, 4, 65536); got != 2 {
		t.Fatalf("first grow = %d, want 2", got)
	}
	var h Host
	err := Call(func() { func() { h.Xproc_exit(1) }() })
	var tr *Trap
	if !errors.As(err, &tr) || !strings.Contains(tr.Error(), "proc_exit(1)") || len(tr.Stack) == 0 {
		t.Fatalf("got %v", err)
	}
	checkRelease(t, m)
	if _, ok := live.Load(uintptr(unsafe.Pointer(m))); ok {
		t.Fatal("state map entry not dropped")
	}
}

// newInitMem mimics the generated New after the single-copy patch: nothing is allocated or copied yet.
func newInitMem(t *testing.T, size int, segs ...Segment) *[]byte {
	t.Helper()
	m := new([]byte)
	InitMemory(m, size, segs...)
	t.Cleanup(func() { Release(m) })
	if *m != nil || Committed(m) != 0 {
		t.Fatalf("InitMemory must only record: len=%d committed=%d", len(*m), Committed(m))
	}
	return m
}

func TestStartMemoryWritesSegmentsIntoRegion(t *testing.T) {
	m := newInitMem(t, 0x20000, Segment{Dest: 0x10000, Data: "hello"}, Segment{Dest: 0x100, Data: "ab"})
	if err := SetLimit(m, MinLimit); err != nil {
		t.Fatal(err)
	}
	StartMemory(m)
	if len(*m) != 0x20000 || cap(*m) != MinLimit || Committed(m) != 0x20000 {
		t.Fatalf("len=%d cap=%d committed=%d", len(*m), cap(*m), Committed(m))
	}
	if string((*m)[0x10000:0x10005]) != "hello" || string((*m)[0x100:0x102]) != "ab" || (*m)[0] != 0 || (*m)[0x10005] != 0 {
		t.Fatal("segments not written exactly")
	}
	if err := SetLimit(m, 2*MinLimit); err == nil {
		t.Fatal("SetLimit after StartMemory must fail")
	}
	p := base(m)
	if got := MemoryGrow(m, 2, 65536); got != 2 || base(m) != p {
		t.Fatalf("grow after StartMemory = %d, base moved=%v (the region must be reused, never copied)", got, base(m) != p)
	}
	if string((*m)[0x10000:0x10005]) != "hello" {
		t.Fatal("content lost by grow")
	}
	StartMemory(m) // second call: nothing pending
	if len(*m) != 4<<16 {
		t.Fatalf("second StartMemory changed the memory: len=%d", len(*m))
	}
	checkRelease(t, m)
}

func TestStartMemoryDefaultLimit(t *testing.T) {
	m := newInitMem(t, 0x20000)
	StartMemory(m)
	if cap(*m) != DefaultLimit {
		t.Fatalf("cap=%d, want DefaultLimit", cap(*m))
	}
	checkRelease(t, m)
}

func TestStartMemoryWithoutInitIsNoop(t *testing.T) {
	m := new([]byte)
	StartMemory(m)
	if *m != nil || Refused(m) {
		t.Fatal("StartMemory without InitMemory must do nothing")
	}
}

func TestStartMemoryTraps(t *testing.T) {
	for _, c := range []struct {
		name    string
		size    int
		segs    []Segment
		refused bool
	}{
		{"above limit", 2 * MinLimit, nil, true},
		{"segment past end", 0x20000, []Segment{{Dest: 0x1ffff, Data: "ab"}}, false},
		{"negative dest", 0x20000, []Segment{{Dest: -1, Data: "a"}}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newInitMem(t, c.size, c.segs...)
			if err := SetLimit(m, MinLimit); err != nil {
				t.Fatal(err)
			}
			var trap *Trap
			if err := Call(func() { StartMemory(m) }); !errors.As(err, &trap) {
				t.Fatalf("want *Trap, got %v", err)
			}
			if Refused(m) != c.refused || *m != nil || Committed(m) != 0 {
				t.Fatalf("refused=%v mem=%v committed=%d", Refused(m), *m != nil, Committed(m))
			}
		})
	}
}
