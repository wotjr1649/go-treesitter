package conformance

import (
	"errors"
	"strings"
	"testing"

	treesitter "github.com/wotjr1649/go-treesitter"
)

func TestQueryMatchLimitIsPerCall(t *testing.T) {
	language := languages["html"]
	if language == nil {
		t.Skip("html not linked")
	}
	src := []byte("<ul>" + strings.Repeat("<li>a</li>", 300) + "</ul>")
	q, err := treesitter.NewQuery(language(), "(element (element) @a (element) @b (element) @c)")
	if err != nil {
		t.Fatal(err)
	}
	parse := func() *treesitter.Tree {
		p := newParser(t, "html")
		tree, err := p.Parse(bg, src, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(tree.Close)
		return tree
	}

	const maxMatches = 128 // Bound the test's work when the cursor has ample room for states.
	type result struct {
		count int
		err   error
	}
	run := func(tree *treesitter.Tree, limit uint32) (r result) {
		for _, err := range tree.Matches(bg, q, &treesitter.QueryOptions{MatchLimit: limit}) {
			if err != nil {
				r.err = err
				break
			}
			r.count++
			if r.count == maxMatches {
				break
			}
		}
		return r
	}
	const unlimited = ^uint32(0)
	fresh := map[uint32]result{}
	for _, limit := range []uint32{0, 1, 8, 64, unlimited} {
		r := run(parse(), limit)
		if limit == 0 || limit == unlimited {
			if r.count != maxMatches || r.err != nil {
				t.Fatalf("fresh limit %d: got %d matches, err=%v", limit, r.count, r.err)
			}
		} else if !errors.Is(r.err, treesitter.ErrQueryMatchLimit) {
			t.Fatalf("fresh limit %d: got %d matches, err=%v, want ErrQueryMatchLimit", limit, r.count, r.err)
		}
		fresh[limit] = r
	}
	tree := parse()
	for i, limit := range []uint32{8, 8, 64, 8, 1, 0, 1, unlimited, 0, 1} {
		got, want := run(tree, limit), fresh[limit]
		if got.count != want.count || !errors.Is(got.err, want.err) {
			t.Errorf("call %d, limit %d: got %d matches, err=%v; fresh got %d, err=%v", i+1, limit, got.count, got.err, want.count, want.err)
		}
	}
}
