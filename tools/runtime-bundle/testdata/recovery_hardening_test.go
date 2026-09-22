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
