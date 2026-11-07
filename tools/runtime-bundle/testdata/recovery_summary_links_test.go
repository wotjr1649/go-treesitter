package gotreesitter

import "testing"

// Private invariants for c-oracle-recovery-summary-all-links.patch, run by tools/runtime-bundle/check.py.
// The capacity and extras cases are adapted from review reproductions of an earlier draft.

func summaryLinksNode(state StateID, prev *gssNode, extra bool, end uint32) *gssNode {
	payload := &Node{endByte: end}
	payload.setExtra(extra)
	depth := uint32(1)
	if prev != nil {
		depth = prev.depth + 1
	}
	return &gssNode{entry: newStackEntryNode(state, payload), prev: prev, depth: depth}
}

// summaryLinksCReference follows C stack.c stack__iter literally: stopped iterators are erased at once,
// alternate links are pushed while the array holds fewer than MAX_ITERATOR_COUNT, then link 0 advances in place.
func summaryLinksCReference(head *gssNode) []cStackSummaryEntry {
	type iterator struct {
		node  *gssNode
		depth int
	}
	its := []iterator{{head, 0}}
	var out []cStackSummaryEntry
	for len(its) > 0 {
		for i, size := 0, len(its); i < size; i++ {
			cur := its[i]
			n := cur.node
			if cur.depth <= cRecoverMaxSummaryDepth {
				duplicate := false
				for j := len(out) - 1; j >= 0; j-- {
					if int(out[j].depth) < cur.depth {
						break
					}
					if int(out[j].depth) == cur.depth && out[j].state == n.entry.state {
						duplicate = true
						break
					}
				}
				if !duplicate {
					out = append(out, cStackSummaryEntry{state: n.entry.state, depth: uint8(cur.depth)})
				}
			}
			if cur.depth > cRecoverMaxSummaryDepth || n.prev == nil {
				its = append(its[:i], its[i+1:]...)
				i--
				size--
				continue
			}
			for j := 1; j <= n.linkCount(); j++ {
				li := j
				if j == n.linkCount() {
					li = 0
				} else if len(its) >= 64 {
					continue
				}
				prev, entry := n.link(li)
				next := iterator{prev, cur.depth}
				if cEntryCountsTowardDepth(entry) {
					next.depth++
				}
				if li == 0 {
					its[i] = next
				} else if prev != nil {
					its = append(its, next)
				}
			}
		}
	}
	return out
}

func summaryLinksStates(entries []cStackSummaryEntry) map[[2]int]bool {
	set := map[[2]int]bool{}
	for _, e := range entries {
		set[[2]int{int(e.depth), int(e.state)}] = true
	}
	return set
}

func TestRecoverySummaryLinksIteratorCapacity(t *testing.T) {
	base := &gssNode{entry: stackEntry{state: 1}, depth: 1}
	left := summaryLinksNode(9001, base, false, 1)
	right := summaryLinksNode(9002, base, false, 1)
	branch := summaryLinksNode(9000, left, false, 1)
	branch.appendExtraLink(gssMainLink{prev: right, entry: branch.entry})
	leaves := make([]*gssNode, 64)
	for i := range leaves {
		leaves[i] = base
	}
	leaves[63] = branch
	for len(leaves) > 1 {
		next := make([]*gssNode, len(leaves)/2)
		for i := range next {
			n := summaryLinksNode(StateID(100+len(leaves)), leaves[2*i], false, 1)
			n.appendExtraLink(gssMainLink{prev: leaves[2*i+1], entry: n.entry})
			next[i] = n
		}
		leaves = next
	}
	reference := summaryLinksCReference(leaves[0])
	want := summaryLinksStates(reference)
	got := summaryLinksStates(cRecordSummaryAllLinks(leaves[0]))
	reached := false
	for _, e := range reference {
		reached = reached || e.state == 9002
	}
	if !reached {
		t.Fatal("invalid reproduction: the C reference walk did not reach the alternate link")
	}
	if len(got) != len(want) {
		t.Fatalf("summary entry set differs from the C reference walk: got %d, want %d", len(got), len(want))
	}
	for k := range want {
		if !got[k] {
			t.Fatalf("summary lacks C entry depth=%d state=%d", k[0], k[1])
		}
	}
}

func TestRecoverySummaryLinksElectionSeesEveryDepth(t *testing.T) {
	base := &gssNode{entry: stackEntry{state: 1}, depth: 1}
	primary := summaryLinksNode(20, base, false, 0)
	target := summaryLinksNode(30, base, false, 0)
	alternate := summaryLinksNode(50, target, false, 0)
	head := summaryLinksNode(50, primary, false, 1)
	extraPayload := &Node{endByte: 1}
	extraPayload.setExtra(true)
	head.appendExtraLink(gssMainLink{prev: alternate, entry: newStackEntryNode(head.entry.state, extraPayload)})
	summary := cRecordSummaryAllLinks(head)
	for i := 1; i < len(summary); i++ {
		if summary[i].depth < summary[i-1].depth {
			t.Fatalf("summary is not depth ordered: %+v", summary)
		}
	}
	stacks := []glrStack{{cRec: &cRecoverState{summary: summary}}}
	var scratch cRecoverElectionScratch
	scratch.prepare(1, 100)
	scratch.members = append(scratch.members, 0)
	scratch.prepareMemberCursors(1)
	seen := false
	for d := 0; d <= cRecoverMaxSummaryDepth+1; d++ {
		iter := scratch.beginDepth(d)
		for {
			_, entry, status := iter.next(stacks)
			if status == cRecoverElectionIterExhausted {
				break
			}
			if status == cRecoverElectionIterCandidate && entry.state == 30 {
				seen = true
			}
		}
	}
	if !seen {
		t.Fatalf("election skipped the alternate-link entry state 30: %+v", summary)
	}
}

func TestRecoverySummaryLinksSingleChain(t *testing.T) {
	base := &gssNode{entry: stackEntry{state: 1}, depth: 1}
	n := summaryLinksNode(10, base, false, 3)
	n = summaryLinksNode(11, n, true, 4)
	n = summaryLinksNode(12, n, false, 6)
	n = &gssNode{entry: stackEntry{state: cErrorState}, prev: n, depth: n.depth + 1}
	var entries []stackEntry
	for m := n; m != nil; m = m.prev {
		entries = append(entries, m.entry)
	}
	want := (&Parser{}).cRecordSummary(entries)
	got := cRecordSummaryAllLinks(n)
	if len(got) != len(want) {
		t.Fatalf("single-chain summary differs: got %+v want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("single-chain summary differs at %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}
