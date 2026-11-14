package conformance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	treesitter "github.com/wotjr1649/go-treesitter"
)

func TestQueryNarrowRangeReuse(t *testing.T) {
	if languages["json"] == nil {
		t.Skip("json not linked")
	}
	p := newParser(t, "json")
	var b strings.Builder
	b.WriteByte('[')
	for i := range 100000 {
		if i != 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"k%d":[%d,"s"]}`, i, i)
	}
	b.WriteByte(']')
	tree, err := p.Parse(bg, []byte(b.String()), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()
	q, err := treesitter.NewQuery(languages["json"](), "(number) @n")
	if err != nil {
		t.Fatal(err)
	}
	opts := &treesitter.QueryOptions{StartByte: 10, EndByte: 40}
	run := func() {
		t.Helper()
		count := 0
		for m, err := range tree.Matches(bg, q, opts) {
			if err != nil {
				t.Fatal(err)
			}
			n := m.Captures[0].Node
			if n.Kind() != "number" || n.StartByte() >= 40 || n.EndByte() <= 10 {
				t.Fatal("bad narrow-range capture", n)
			}
			count++
		}
		if count != 2 {
			t.Fatalf("got %d matches, want 2", count)
		}
	}
	start := time.Now()
	run() // Snapshot, compilation and the first index build are outside the reuse window.
	first := time.Since(start)
	start = time.Now()
	for range 20 {
		run()
	}
	elapsed := time.Since(start)
	// P18 measured 9.65s for these 20 warm calls; 1s rejects per-exec whole-tree rebuilds with headroom.
	const bound = time.Second
	t.Logf("bytes=%d first=%s repeated=20 elapsed=%s per-call=%s bound=%s", b.Len(), first, elapsed, elapsed/20, bound)
	if elapsed >= bound {
		t.Fatalf("repeated narrow-range queries took %s, bound %s", elapsed, bound)
	}
	other, err := p.Parse(bg, []byte("[1]"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, err := range other.Matches(bg, q, nil) {
		if err != nil {
			t.Fatal(err)
		}
	}
	other.Close() // Drop the large tree's cached index before interrupting its next build.
	ctx, cancel := context.WithCancel(bg)
	defer cancel()
	timer := time.AfterFunc(10*time.Millisecond, cancel)
	defer timer.Stop()
	var stopped error
	for _, err := range tree.Matches(ctx, q, opts) {
		stopped = err
	}
	if !errors.Is(stopped, context.Canceled) {
		t.Fatalf("index build did not report cancellation: %v", stopped)
	}
	run() // A cancelled partial index must never be used by the next exec on this tree.
}

func TestQueryIndexTreeLifetime(t *testing.T) {
	if languages["json"] == nil {
		t.Skip("json not linked")
	}
	p := newParser(t, "json")
	q, err := treesitter.NewQuery(languages["json"](), "(number) @n")
	if err != nil {
		t.Fatal(err)
	}
	check := func(tree *treesitter.Tree, want int) {
		t.Helper()
		count := 0
		for m, err := range tree.Matches(bg, q, nil) {
			if err != nil {
				t.Fatal(err)
			}
			if m.Captures[0].Node.Kind() != "number" {
				t.Fatal(m)
			}
			count++
		}
		if count != want {
			t.Fatalf("got %d matches, want %d", count, want)
		}
	}
	old, err := p.Parse(bg, []byte("[0]"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	check(old, 1)
	for i := range 64 {
		count := 2 + i%7
		next, err := p.Parse(bg, []byte("["+strings.Repeat("1,", count-1)+"1]"), nil)
		if err != nil {
			t.Fatal(err)
		}
		check(next, count)
		check(next, count)
		next.Close()
		if i%8 == 7 {
			check(old, 1)
		}
	}
}
