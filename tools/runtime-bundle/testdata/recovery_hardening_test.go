package gotreesitter

import "testing"

func TestSharedClosedRecoveryPrefix(t *testing.T) {
	lang := &Language{CompactPackedGSSVersionOrderCertified: true}
	scratch := &glrMergeScratch{language: lang, packedGSSVersionOrderActive: true}
	errorNode := &Node{symbol: errorSymbol}
	errorNode.setHasError(true)
	prefix := &gssNode{entry: newStackEntryNode(1, errorNode)}
	head := func(parent *gssNode) *gssNode {
		return &gssNode{prev: parent, entry: newStackEntryNode(2, &Node{symbol: 1})}
	}
	a, b := head(prefix), head(prefix)
	if !gssSharedClosedErrorPrefix(scratch, a, b) {
		t.Fatal("identical recovery prefix rejected")
	}
	copyPrefix := *prefix
	if gssSharedClosedErrorPrefix(scratch, a, head(&copyPrefix)) {
		t.Fatal("different recovery histories merged")
	}
	if gssSharedClosedErrorPrefix(scratch, a, head(nil)) {
		t.Fatal("path bypassing recovery prefix merged")
	}
	b.appendExtraLink(gssMainLink{prev: nil, entry: b.entry})
	if gssSharedClosedErrorPrefix(scratch, a, b) {
		t.Fatal("alternate path bypassing recovery prefix merged")
	}
	if gssSharedClosedErrorPrefix(&glrMergeScratch{}, a, head(prefix)) {
		t.Fatal("uncertified transaction changed")
	}
	aStack := glrStack{gss: gssStack{head: a}}
	bStack := glrStack{gss: gssStack{head: head(prefix)}, cPaused: true}
	if gssMainCanMergeWithScratch(scratch, &aStack, &bStack) ||
		gssMainCanMergeWithScratchPhase(scratch, &aStack, &bStack, "test") {
		t.Fatal("paused recovery merged")
	}
	bStack.cPaused = false
	if !gssMainCanMergeWithScratch(scratch, &aStack, &bStack) ||
		!gssMainCanMergeWithScratchPhase(scratch, &aStack, &bStack, "test") {
		t.Fatal("ordinary and instrumented merge decisions differ")
	}
}

func TestRecoveryAcceptanceOrder(t *testing.T) {
	p := &Parser{}
	first, last := glrStack{}, glrStack{}
	p.acceptStack(&first)
	p.acceptStack(&last)
	p.acceptStack(&first)
	if first.cAcceptOrder != 1 || last.cAcceptOrder != 2 {
		t.Fatal("accept order was not monotonic and idempotent")
	}
	if first.clone().cAcceptOrder != first.cAcceptOrder {
		t.Fatal("acceptance receipt lost while copying a stack")
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
	leaf := &Node{symbol: errorSymbol, startByte: 3, endByte: 4}
	if cost := p.rawStackEntryErrorCost(nil, newStackEntryNode(0, leaf)); cost != 0 {
		t.Fatalf("unrecognized-byte leaf was charged twice: %d", cost)
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
