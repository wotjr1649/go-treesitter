package conformance

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	treesitter "github.com/wotjr1649/go-treesitter"
)

func FuzzQueryMatches(f *testing.F) {
	language := languages["json"]
	if language == nil {
		f.Skip("json is not linked")
	}
	var queries []*treesitter.Query
	for _, pattern := range []string{"(_) @node", "(pair key: (string) @key value: (_) @value)", "((string) @s (#match? @s \"^\\\"x\"))"} {
		q, err := treesitter.NewQuery(language(), pattern)
		if err != nil {
			f.Fatal(err)
		}
		queries = append(queries, q)
	}
	f.Add("{\"x\":[1,2]}", uint8(0))
	f.Add("{\"key\":true}", uint8(1))
	f.Add("[\"x\", \"no\", \"xyz\"]", uint8(2))
	f.Add("[\"\xff\",", uint8(0))
	f.Fuzz(func(t *testing.T, src string, query uint8) {
		if len(src) > 512 {
			t.Skip()
		}
		// No wall-clock budget: inputs are size-bounded and the test process limits catch a hang.
		ctx := bg
		p := newParser(t, "json", treesitter.WithMaxMemoryBytes(64<<20))
		tree, err := p.Parse(ctx, []byte(src), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Close()
		q := queries[int(query)%len(queries)]
		names := q.CaptureNames()
		stopped, stop := context.WithCancel(ctx)
		stop()
		cancelled := false
		for _, err := range tree.Matches(stopped, q, nil) {
			if cancelled || !errors.Is(err, treesitter.ErrCancelled) {
				t.Fatalf("cancelled Matches: %v", err)
			}
			cancelled = true
		}
		if !cancelled {
			t.Fatal("Matches did not report cancellation")
		}
		matched, limited := false, false
		var got []string
		for m, err := range tree.Matches(ctx, q, &treesitter.QueryOptions{MatchLimit: 128}) {
			if errors.Is(err, treesitter.ErrQueryMatchLimit) { // allowed by the contract; completed matches stay valid
				limited = true
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if m.Pattern < 0 || m.Pattern >= q.PatternCount() || len(m.Captures) == 0 {
				t.Fatalf("invalid match for pattern %d", m.Pattern)
			}
			for _, c := range m.Captures {
				n := c.Node
				if int(c.Index) >= len(names) || n.IsZero() || n.StartByte() > n.EndByte() ||
					n.EndByte() > uint32(len(src)) || n.Text() != src[n.StartByte():n.EndByte()] {
					t.Fatalf("invalid capture %d at [%d,%d)", c.Index, n.StartByte(), n.EndByte())
				}
				got = append(got, names[c.Index]+"="+n.Text())
			}
			matched = true
		}
		// The two seeds must yield these captures over the whole match stream, not only the first match.
		switch {
		case src == "{\"key\":true}" && query == 1 && !limited:
			if want := []string{"key=\"key\"", "value=true"}; !slices.Equal(got, want) {
				t.Fatalf("pair captures %q, want %q", got, want)
			}
		case src == "[\"x\", \"no\", \"xyz\"]" && query == 2 && !limited:
			if want := []string{"s=\"x\"", "s=\"xyz\""}; !slices.Equal(got, want) {
				t.Fatalf("regex captures %q, want %q", got, want)
			}
		}
		if int(query)%len(queries) == 0 && !matched {
			snap, err := tree.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if snap.Root().Kind() == "document" {
				t.Fatal("named wildcard did not match the document")
			}
		}
		snap, err := p.ParseSnapshot(ctx, []byte("[]"))
		if err != nil || snap.Root().HasError() || snap.Source() != "[]" {
			t.Fatalf("parse after Matches: %v", err)
		}
	})
}

func FuzzTreeEditRanges(f *testing.F) {
	if languages["json"] == nil {
		f.Skip("json is not linked")
	}
	f.Add("[]", "1", uint16(1), uint16(0), uint8(0))
	f.Add("[1,2]\r\n", "\n\"한😀\"", uint16(3), uint16(1), uint8(1))
	f.Add("[\"\xff\",", "]", uint16(4), uint16(2), uint8(2))
	f.Add("", "\n", uint16(0), uint16(0), uint8(0))
	f.Add("[1,2]", "3", uint16(3), uint16(6), uint8(0)) // valid JSON after both edits: [1,2] -> [1,32]
	f.Fuzz(func(t *testing.T, original, replacement string, at, remove uint16, invalid uint8) {
		if len(original) > 256 || len(replacement) > 256 {
			t.Skip()
		}
		ctx := bg
		p := newParser(t, "json", treesitter.WithMaxMemoryBytes(64<<20))
		old, err := p.ParseOwned(ctx, []byte(original), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer old.Close()
		before, err := old.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}

		replacement0, at0, remove0 := replacement, at, remove // the loop below rewrites its parameters
		src := original
		for i := range 2 {
			start := uint32(at) % uint32(len(src)+1)
			end := start + uint32(remove)%uint32(len(src)-int(start)+1)
			next := src[:start] + replacement + src[end:]
			newEnd := start + uint32(len(replacement))
			edit := treesitter.InputEdit{
				StartByte: start, OldEndByte: end, NewEndByte: newEnd,
				StartPoint: fuzzBytePoint(src, start), OldEndPoint: fuzzBytePoint(src, end),
				NewEndPoint: fuzzBytePoint(next, newEnd),
			}
			if i == 0 {
				bad := edit
				switch invalid % 3 {
				case 0:
					bad.StartByte = end + 1
				case 1:
					bad.OldEndByte = uint32(len(src)) + 1
				case 2:
					bad.StartPoint.Row = math.MaxUint32
				}
				if err := old.Edit(bad); !errors.Is(err, treesitter.ErrInvalidInput) || old.InputBytes() != len(src) {
					t.Fatalf("invalid Edit changed the tree or returned the wrong error: %v", err)
				}
				if s, err := old.Snapshot(ctx); err != nil || s.Source() != original {
					t.Fatalf("Snapshot after an invalid Edit: %v, want the unedited tree", err)
				}
			}
			if err := old.Edit(edit); err != nil || old.InputBytes() != len(next) {
				t.Fatalf("Edit: input length %d, want %d, error %v", old.InputBytes(), len(next), err)
			}
			src, replacement = next, ""
			at, remove = remove, at
		}
		if _, err := old.Snapshot(ctx); !errors.Is(err, treesitter.ErrTreeEdited) {
			t.Fatalf("edited Snapshot: %v", err)
		}
		inc, err := p.Parse(ctx, []byte(src), old)
		if err != nil {
			t.Fatal(err)
		}
		defer inc.Close()
		changed, err := inc.ChangedRanges(old)
		if err != nil {
			t.Fatal(err)
		}
		snap, err := inc.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		checkTree(t, "edited", inc, snap, []byte(src))
		if before.Source() != original || snap.Source() != src {
			t.Fatal("snapshot source changed or does not describe the edited input")
		}
		cached, err := inc.ChangedRanges(old)
		if err != nil || !slices.Equal(changed, cached) {
			t.Fatalf("ChangedRanges with a cached snapshot: %v, %v; first call %v", cached, err, changed)
		}
		if original == "[1,2]" && replacement0 == "3" && at0 == 3 && remove0 == 6 {
			want := []treesitter.Range{{StartByte: 3, EndByte: 4, StartPoint: treesitter.Point{Column: 3}, EndPoint: treesitter.Point{Column: 4}}}
			if !slices.Equal(changed, want) {
				t.Fatalf("ChangedRanges %+v, want %+v", changed, want)
			}
		}
		var prev uint32
		for i, r := range changed {
			if r.StartByte < prev || r.EndByte < r.StartByte ||
				r.StartPoint != fuzzBytePoint(src, r.StartByte) || r.EndPoint != fuzzBytePoint(src, r.EndByte) {
				t.Fatalf("invalid ChangedRanges[%d]: %+v (previous end %d)", i, r, prev)
			}
			prev = r.EndByte
		}
	})
}

func fuzzBytePoint(src string, offset uint32) treesitter.Point {
	prefix := src[:min(offset, uint32(len(src)))]
	return treesitter.Point{
		Row:    uint32(strings.Count(prefix, "\n")),
		Column: uint32(len(prefix) - strings.LastIndexByte(prefix, '\n') - 1),
	}
}
