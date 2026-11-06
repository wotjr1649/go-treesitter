package gotreesitter

import "testing"

func TestClosedRecoveryGSSMergePreservesHistories(t *testing.T) {
	lang := &Language{CompactPackedGSSVersionOrderCertified: true}
	scratch := &glrMergeScratch{language: lang, packedGSSVersionOrderActive: true}
	p := &Parser{language: lang, mergeScratch: scratch}
	top := &Node{symbol: 1, startByte: 1, endByte: 1}

	// Equal heads that share the same closed recovery prefix remain eligible.
	sharedPrefix := recoveryInvariantPrefix(recoveryInvariantErrorNode(10), 20)
	a := recoveryInvariantStack(sharedPrefix, top)
	b := recoveryInvariantStack(sharedPrefix, top)
	scratch.cClosedRecoveryMerge = true // models the certified C action transaction
	if costA, costB := cStackErrorCostForMergeCached(scratch, lang, &a), cStackErrorCostForMergeCached(scratch, lang, &b); costA == 0 || costA != costB {
		t.Fatalf("shared-prefix fixture is not a positive equal-cost recovery: %d != %d", costA, costB)
	}
	recoveryInvariantCheckEligibility(t, scratch, &a, &b, true)
	if !tryGSSMainMergeForParser(p, &a, &b) {
		t.Fatal("identical closed recovery prefix rejected")
	}
	if a.gss.head.linkCount() != 1 {
		t.Fatalf("identical prefix created %d links, want one", a.gss.head.linkCount())
	}

	// Distinct histories with the same C cost may merge, but every predecessor
	// path must survive the real mutating merge entrypoint.
	scratch = &glrMergeScratch{language: lang, packedGSSVersionOrderActive: true, cClosedRecoveryMerge: true}
	p.mergeScratch = scratch
	errorA := recoveryInvariantErrorNode(11)
	errorB := recoveryInvariantErrorNode(12)
	a = recoveryInvariantStack(recoveryInvariantPrefix(errorA, 21), top)
	b = recoveryInvariantStack(recoveryInvariantPrefix(errorB, 22), top)
	costA, costB := cStackErrorCostForMergeCached(scratch, lang, &a), cStackErrorCostForMergeCached(scratch, lang, &b)
	if costA == 0 || costA != costB {
		t.Fatalf("distinct-history fixture is not a positive equal-cost recovery: %d != %d", costA, costB)
	}
	recoveryInvariantCheckEligibility(t, scratch, &a, &b, true)
	if !tryGSSMainMergeForParser(p, &a, &b) {
		t.Fatal("equal-cost closed recovery histories did not merge")
	}
	if got := recoveryInvariantBaseSymbols(a.gss.head); !got[21] || !got[22] ||
		!recoveryInvariantHasEntryNode(a.gss.head, errorA) || !recoveryInvariantHasEntryNode(a.gss.head, errorB) {
		t.Fatalf("merged GSS lost an error-history path: base symbols %v, error nodes %v/%v", got, recoveryInvariantHasEntryNode(a.gss.head, errorA), recoveryInvariantHasEntryNode(a.gss.head, errorB))
	}

	// A clean path that bypasses the recovery prefix has a different C cost.
	scratch = &glrMergeScratch{language: lang, packedGSSVersionOrderActive: true, cClosedRecoveryMerge: true}
	p.mergeScratch = scratch
	a = recoveryInvariantStack(recoveryInvariantPrefix(recoveryInvariantErrorNode(13), 23), top)
	cleanBase := &gssNode{entry: newStackEntryNode(1, &Node{symbol: 24, startByte: 0, endByte: 1})}
	b = glrStack{gss: gssStack{head: &gssNode{prev: cleanBase, entry: newStackEntryNode(2, top)}}, byteOffset: 1}
	if cStackErrorCostForMergeCached(scratch, lang, &a) == cStackErrorCostForMergeCached(scratch, lang, &b) {
		t.Fatal("clean bypass control unexpectedly has the recovery cost")
	}
	recoveryInvariantCheckEligibility(t, scratch, &a, &b, false)
	if tryGSSMainMergeForParser(p, &a, &b) {
		t.Fatal("clean path that bypasses recovery merged with the error history")
	}

	// A closed-recovery transaction cannot absorb paused or open recovery owners,
	// even when both candidates carry equal positive-cost error histories.
	scratch = &glrMergeScratch{language: lang, packedGSSVersionOrderActive: true, cClosedRecoveryMerge: true}
	p.mergeScratch = scratch
	a = recoveryInvariantStack(recoveryInvariantPrefix(recoveryInvariantErrorNode(14), 25), top)
	b = recoveryInvariantStack(recoveryInvariantPrefix(recoveryInvariantErrorNode(15), 26), top)
	a.cPaused, b.cPaused = true, true
	recoveryInvariantCheckEligibility(t, scratch, &a, &b, false)
	if cStackErrorCostForMergeCached(scratch, lang, &a) != cStackErrorCostForMergeCached(scratch, lang, &b) || tryGSSMainMergeForParser(p, &a, &b) {
		t.Fatal("equal-cost paused recovery histories merged")
	}

	a = recoveryInvariantStack(recoveryInvariantPrefix(recoveryInvariantErrorNode(16), 27), top)
	b = recoveryInvariantStack(recoveryInvariantPrefix(recoveryInvariantErrorNode(17), 28), top)
	a.cRec = &cRecoverState{summary: []cStackSummaryEntry{{state: 7, depth: 1}}, openErr: recoveryInvariantErrorNode(18)}
	b.cRec = &cRecoverState{summary: []cStackSummaryEntry{{state: 9, depth: 1}}, openErr: recoveryInvariantErrorNode(19)}
	recoveryInvariantCheckEligibility(t, scratch, &a, &b, false)
	if cStackErrorCostForMergeCached(scratch, lang, &a) != cStackErrorCostForMergeCached(scratch, lang, &b) || tryGSSMainMergeForParser(p, &a, &b) {
		t.Fatal("open recovery owners with distinct summaries merged in the closed-history gate")
	}

	// Outside the certified C transaction, and for an uncertified language,
	// error-bearing heads retain the conservative no-merge behavior.
	scratch.cClosedRecoveryMerge = false
	a = recoveryInvariantStack(recoveryInvariantPrefix(recoveryInvariantErrorNode(20), 30), top)
	b = recoveryInvariantStack(recoveryInvariantPrefix(recoveryInvariantErrorNode(21), 31), top)
	recoveryInvariantCheckEligibility(t, scratch, &a, &b, false)
	if tryGSSMainMergeForParser(p, &a, &b) {
		t.Fatal("closed error histories merged outside their transaction")
	}

	uncertified := &Language{}
	scratch = &glrMergeScratch{language: uncertified}
	p = &Parser{language: uncertified, mergeScratch: scratch}
	a = recoveryInvariantStack(recoveryInvariantPrefix(recoveryInvariantErrorNode(22), 32), top)
	b = recoveryInvariantStack(recoveryInvariantPrefix(recoveryInvariantErrorNode(23), 33), top)
	recoveryInvariantCheckEligibility(t, scratch, &a, &b, false)
	if tryGSSMainMergeForParser(p, &a, &b) {
		t.Fatal("uncertified error histories merged")
	}
}

func recoveryInvariantCheckEligibility(t *testing.T, scratch *glrMergeScratch, a, b *glrStack, want bool) {
	t.Helper()
	ordinary := gssMainCanMergeWithScratch(scratch, a, b)
	instrumented := gssMainCanMergeWithScratchPhase(scratch, a, b, "test")
	if ordinary != instrumented || ordinary != want {
		t.Fatalf("ordinary/instrumented merge eligibility = %v/%v, want %v", ordinary, instrumented, want)
	}
}

func recoveryInvariantErrorNode(childSymbol Symbol) *Node {
	child := &Node{symbol: childSymbol, startByte: 0, endByte: 1}
	n := &Node{symbol: errorSymbol, startByte: 0, endByte: 1, children: []*Node{child}}
	n.setHasError(true)
	return n
}

func recoveryInvariantPrefix(errorNode *Node, baseSymbol Symbol) *gssNode {
	base := &gssNode{entry: newStackEntryNode(0, &Node{symbol: baseSymbol, startByte: 0, endByte: 1})}
	return &gssNode{prev: base, entry: newStackEntryNode(1, errorNode)}
}

func recoveryInvariantStack(prefix *gssNode, top *Node) glrStack {
	return glrStack{
		gss:        gssStack{head: &gssNode{prev: prefix, entry: newStackEntryNode(2, top)}},
		byteOffset: 1,
	}
}

func recoveryInvariantBaseSymbols(head *gssNode) map[Symbol]bool {
	symbols := make(map[Symbol]bool)
	seen := make(map[*gssNode]bool)
	var visit func(*gssNode)
	visit = func(n *gssNode) {
		if n == nil || seen[n] {
			return
		}
		seen[n] = true
		if n.entry.state == 0 {
			for i := 0; i < n.linkCount(); i++ {
				_, entry := n.link(i)
				if node := stackEntryNode(entry); node != nil {
					symbols[node.symbol] = true
				}
			}
			return
		}
		for i := 0; i < n.linkCount(); i++ {
			prev, _ := n.link(i)
			visit(prev)
		}
	}
	visit(head)
	return symbols
}

func recoveryInvariantHasEntryNode(head *gssNode, want *Node) bool {
	seen := make(map[*gssNode]bool)
	var visit func(*gssNode) bool
	visit = func(n *gssNode) bool {
		if n == nil || seen[n] {
			return false
		}
		seen[n] = true
		for i := 0; i < n.linkCount(); i++ {
			prev, entry := n.link(i)
			if stackEntryNode(entry) == want {
				return true
			}
			if visit(prev) {
				return true
			}
		}
		return false
	}
	return visit(head)
}

func TestRecoveryAcceptanceOrder(t *testing.T) {
	for _, packed := range []bool{true, false} {
		name := "nonpacked"
		if packed {
			name = "packed"
		}
		t.Run(name, func(t *testing.T) {
			lang := &Language{CompactPackedGSSVersionOrderCertified: packed}
			scratch := &glrMergeScratch{language: lang, packedGSSVersionOrderActive: packed}
			p := &Parser{language: lang, mergeScratch: scratch, errorCostCompetition: true}
			first := glrStack{entries: []stackEntry{newStackEntryNode(0, recoveryInvariantErrorNode(40))}}
			last := glrStack{entries: []stackEntry{newStackEntryNode(0, recoveryInvariantErrorNode(41))}}
			p.acceptStack(&first)
			p.acceptStack(&last)
			p.acceptStack(&first)
			if first.cAcceptOrder != 1 || last.cAcceptOrder != 2 {
				t.Fatal("accept order was not monotonic and idempotent")
			}
			if first.clone().cAcceptOrder != first.cAcceptOrder {
				t.Fatal("acceptance receipt lost while copying a stack")
			}
			if firstCost, lastCost := p.cStackResultErrorCost(&first), p.cStackResultErrorCost(&last); firstCost == 0 || firstCost != lastCost {
				t.Fatalf("acceptance-order fixture is not a positive equal-cost pair: %d != %d", firstCost, lastCost)
			}
			if got := stackCompareForResultSelection(p, nil, &last, &first, false); got <= 0 {
				t.Fatalf("later accepted positive-cost candidate did not win result comparison: %d", got)
			}
			if got := stackCompareForResultSelection(p, nil, &first, &last, false); got >= 0 {
				t.Fatalf("earlier accepted positive-cost candidate won the reverse comparison: %d", got)
			}
		})
	}
}

func TestEOFTransitions(t *testing.T) {
	states := []LexState{
		{EOF: -1, Default: -1, Transitions: []LexTransition{{Lo: 0, Hi: 0, NextState: 1}}},
		{EOF: -1, Default: -1, AcceptToken: 1},
		{EOF: 3, Default: -1, Transitions: []LexTransition{{Lo: 0, Hi: 0, NextState: 1}}},
		{EOF: -1, Default: -1, AcceptToken: 2},
	}
	for _, included := range []bool{false, true} {
		for _, start := range []uint32{0, 2} {
			lexer := NewLexer(states, []byte("abcx"))
			lexer.zeroWidthTokens = []bool{false, true, true}
			end := uint32(4)
			if included {
				end = 3
				lexer.setIncludedRanges([]Range{{StartByte: 1, EndByte: end, StartPoint: Point{Column: 1}, EndPoint: Point{Column: end}}})
			}
			lexer.pos, lexer.col = int(end), end
			token := lexer.Next(start)
			want := Symbol(1)
			if start == 2 {
				want = 2 // The explicit EOF transition precedes the rune-zero edge.
			}
			if token.Symbol != want || token.StartByte != end || token.EndByte != end ||
				token.StartPoint != (Point{Column: end}) || token.EndPoint != token.StartPoint {
				t.Fatalf("included=%v state=%d: %+v", included, start, token)
			}
		}
	}
	// A malformed EOF cycle cannot keep the lexer in an unbounded loop.
	lexer := NewLexer([]LexState{{EOF: 0, Default: -1}}, nil)
	if token := lexer.Next(0); token.Symbol != 0 || token.EndByte != 0 {
		t.Fatalf("EOF cycle returned a token: %+v", token)
	}
}

func TestRecoveryRawLeafCost(t *testing.T) {
	p := &Parser{}
	leaf := newLeafNodeInArena(nil, errorSymbol, true, 3, 4, Point{Column: 3}, Point{Column: 4})
	leaf.setHasError(true)
	if cost := p.rawStackEntryErrorCost(nil, newStackEntryNode(0, leaf)); cost != 0 {
		t.Fatalf("unrecognized-byte leaf was charged twice: %d", cost)
	}
	emptyWrapper := &Node{symbol: errorSymbol, startByte: 4, endByte: 4}
	emptyWrapper.setHasError(true)
	if cost := p.rawStackEntryErrorCost(nil, newStackEntryNode(0, emptyWrapper)); cost != cErrCostPerRecovery {
		t.Fatalf("empty C ERROR parent lost its wrapper cost: %d", cost)
	}
	leaf.symbol = 1
	leaf.setMissing(true)
	leaf.endByte = leaf.startByte
	if cost := p.rawStackEntryErrorCost(nil, newStackEntryNode(0, leaf)); cost != 610 {
		t.Fatalf("missing terminal lost its C recovery cost: %d", cost)
	}
	// A hidden missing leaf survives only in the raw shape. Repeated cost
	// queries and a later node version must preserve that distinction.
	arena := newNodeArena(arenaClassIncremental)
	p.cNodeMemoCache = make([]cNodeMemoCacheEntry, cNodeMemoCacheInitialSize)
	p.cNodeMemoEpoch = 1
	parent := &Node{symbol: 2, ownerArena: arena}
	parent.rawShape = p.captureRawShape(nil, arena, parent.symbol, 0, []stackEntry{newStackEntryNode(0, leaf)}, 0, 1)
	for range 3 {
		if cost := p.cNodeErrorCost(parent); cost != 610 {
			t.Fatalf("hidden missing cost changed across memo hits: %d", cost)
		}
	}
	older := &Node{symbol: 3, ownerArena: arena}
	older.rawShape = p.captureRawShape(nil, arena, older.symbol, 0, []stackEntry{newStackEntryNode(0, parent)}, 0, 1)
	clean := &Node{symbol: 1}
	parent.rawShape = p.captureRawShape(nil, arena, parent.symbol, 0, []stackEntry{newStackEntryNode(0, clean)}, 0, 1)
	nodeBumpEquivVersionBeforePublication(parent)
	if cost := p.cNodeErrorCost(parent); cost != 0 {
		t.Fatalf("old raw cost survived node version change: %d", cost)
	}
	if cost := p.cNodeErrorCost(older); cost != 610 {
		t.Fatalf("current node memo replaced an older captured shape: %d", cost)
	}
}

func TestAcceptRootPreservesMissing(t *testing.T) {
	arena := newNodeArena(arenaClassIncremental)
	lang := &Language{RecoveryStackVersionOrderEnabled: true, SymbolMetadata: []SymbolMetadata{
		{}, {Visible: true, Named: true}, {Visible: true, Named: true}, {}, {Visible: true, Named: true},
	}}
	childErrors := true
	p := &Parser{language: lang, errorCostCompetition: true, mergeScratch: &glrMergeScratch{language: lang, arena: arena, childErrors: &childErrors}}
	leaf := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
	missing := newLeafNodeInArena(arena, 3, false, 1, 1, Point{Column: 1}, Point{Column: 1})
	missing.setMissing(true)
	missing.setHasError(true)
	root := p.newRecoveryParentNodeInArena(arena, 2, true, []*Node{leaf}, 0)
	root.setHasError(true)
	root.rawShape = captureRawShapeForEntries(arena, 2, 0, []stackEntry{newStackEntryNode(1, leaf), newStackEntryNode(2, missing)})
	comment := newLeafNodeInArena(arena, 4, true, 0, 0, Point{}, Point{})
	comment.setExtra(true)
	stack := glrStack{accepted: true, entries: []stackEntry{newStackEntryNode(0, nil), newStackEntryNode(1, comment), newStackEntryNode(1, root)}, byteOffset: 1}
	before := p.cStackResultErrorCost(&stack)
	if before != 610 {
		t.Fatalf("invalid fixture: cost before rebuild = %d", before)
	}
	if reason := p.cAcceptRootRebuild(&stack, arena, nil, nil); reason != ParseStopNone {
		t.Fatalf("rebuild stopped: %v", reason)
	}
	after := p.cStackResultErrorCost(&stack)
	rebuilt := stackEntryNode(stack.top())
	t.Logf("before=%d after=%d public_children=%d raw_ref=%d has_error=%v", before, after, len(rebuilt.children), rebuilt.rawShape, rebuilt.HasError())
	if len(rebuilt.children) != 2 || rebuilt.children[0] != comment || rebuilt.children[1] != leaf || !rebuilt.HasError() {
		t.Fatal("accept-root rebuild changed visible children or lost the error flag")
	}
	if after != before {
		t.Fatalf("accept-root rebuild lost hidden missing cost: before=%d after=%d", before, after)
	}
}
