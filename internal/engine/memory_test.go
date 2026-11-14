package engine

import (
	"bytes"
	"errors"
	"math"
	"math/bits"
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
	Release(m)
	if !regionFreed(p) || !regionFreed(p+MinLimit-1<<16) {
		t.Fatal("region still mapped after Release")
	}
	if *m != nil || Committed(m) != 0 || Refused(m) {
		t.Fatal("state left behind after Release")
	}
	Release(m) // second call: no-op, no panic
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
