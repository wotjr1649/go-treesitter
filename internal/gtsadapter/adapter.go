// Package gtsadapter is the sole import boundary to the pinned runtime.
package gtsadapter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sync/atomic"
	"time"
	"unicode/utf8"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/wotjr1649/go-treesitter/syntax"
)

// Adapter constructs worker-local parsers. It never reads process-global counters.
type Adapter struct{}

var _ syntax.Parser = Adapter{}

type tree struct {
	raw    *gts.Tree
	lang   *gts.Language
	source []byte
	nodes  []syntax.Node
	edited bool
}

func (t *tree) Nodes() []syntax.Node { return t.nodes }
func (t *tree) Close() {
	if t.raw != nil {
		t.raw.Release()
		t.raw = nil
	}
	t.source = nil
}

func (Adapter) Parse(ctx context.Context, req syntax.Request) (syntax.Result, error) {
	d := syntax.Diagnostics{InputBytes: len(req.Source), Language: req.Language,
		ExpectedEOFByte: uint32(len(req.Source)), Route: "not_run"}
	entry := grammars.DetectLanguageByName(req.Language)
	if req.Language == "" {
		entry = grammars.DetectLanguage(filepath.ToSlash(req.Filename))
	}
	valid := utf8.Valid(req.Source) && uint64(len(req.Source)) <= math.MaxUint32 && entry != nil
	if entry != nil {
		d.Language = entry.Name
	}
	var raw *gts.Tree
	var lang *gts.Language
	var parseErr error
	var profile gts.IncrementalParseProfile
	attempted := false
	source := req.Source
	if valid && ctx.Err() == nil {
		lang = entry.Language() // Grammar loading is outside the parse timeout.
		p := gts.NewParser(lang)
		source = bytes.Clone(req.Source)
		old, editErr := prepareEdit(req, lang)
		parseErr = editErr
		if editErr == nil && ctx.Err() == nil {
			var flag uint32
			stop := context.AfterFunc(ctx, func() { atomic.StoreUint32(&flag, 1) })
			p.SetCancellationFlag(&flag)
			budget := req.Timeout
			if deadline, ok := ctx.Deadline(); ok {
				remaining := time.Until(deadline)
				if budget <= 0 || remaining < budget {
					budget = remaining
				}
			}
			if budget > 0 {
				p.SetTimeoutMicros(uint64(max(1, budget.Microseconds())))
				d.DeadlineApplied = true
			}
			attempted = true
			if entry.TokenSourceFactory != nil {
				factory := func(src []byte) (gts.TokenSource, error) { return entry.TokenSourceFactory(src, lang), nil }
				if old != nil {
					raw, parseErr = p.ParseIncrementalWithTokenSourceFactory(source, old, factory)
				} else {
					raw, parseErr = p.ParseWithTokenSourceFactory(source, factory)
				}
			} else if old != nil {
				raw, profile, parseErr = p.ParseIncrementalProfiled(source, old)
			} else {
				raw, parseErr = p.Parse(source)
			}
			stop()
		}
	}
	// Collect every available fact before selecting an outcome. An earlier failure
	// never suppresses the stop receipt, root inspection, or missing-node walk.
	if parseErr != nil {
		d.ErrorType = fmt.Sprintf("%T", parseErr)
	}
	var nodes []syntax.Node
	if raw != nil {
		root := raw.RootNode()
		if root != nil {
			d.RootPresent = true
			d.RootStartByte, d.RootEndByte = root.StartByte(), root.EndByte()
			d.HasError = root.HasError()
			nodes = snapshot(root, lang)
			for _, n := range nodes {
				d.HasMissing = d.HasMissing || n.Missing
			}
		}
		rt := raw.ParseRuntime()
		d.StopReason, d.StoppedEarly = string(rt.StopReason), raw.ParseStoppedEarly()
		d.TokensConsumed, d.LastTokenEndByte = rt.TokensConsumed, rt.LastTokenEndByte
		d.ExpectedEOFByte = rt.ExpectedEOFByte
		d.Iterations, d.IterationLimit = rt.Iterations, rt.IterationLimit
		d.Nodes, d.NodeLimit = rt.NodesAllocated, rt.NodeLimit
		d.PeakStackDepth, d.StackDepthLimit = rt.PeakStackDepth, rt.StackDepthLimit
		d.ArenaBytes, d.ScratchBytes, d.MemoryBudgetBytes = rt.ArenaBytesAllocated, rt.ScratchBytesAllocated, rt.MemoryBudgetBytes
		d.Truncated = rt.Truncated
		d.ReusedOldTree = rt.IncrementalOldTreeReuseRoute || rt.CompactIncrementalReuseRoute
		d.FallbackReason = rt.CompactIncrementalFallbackReason
		d.ReuseReason = profile.ReuseUnsupportedReason
		d.FallbackDetailAvailable = req.Previous != nil
		d.Route = "classic"
		if rt.ForestFastPath {
			d.Route = "forest"
		}
		if rt.CompactReductions > 0 || rt.CompactIncrementalReuseRoute || rt.CompactIncrementalFullRecoveryRoute {
			d.Route = "compact"
		}
	}
	// Priority follows the nine-step contract after observation is complete.
	switch {
	case ctx.Err() != nil:
		d.Outcome = syntax.Cancelled
	case !valid:
		d.Outcome = syntax.UnsupportedInput
	case raw == nil && !attempted:
		d.Outcome = syntax.NotRun
	case raw == nil:
		d.Outcome = syntax.InvariantViolation
	case d.StopReason == string(gts.ParseStopTimeout):
		d.Outcome = syntax.Timeout
	case d.StopReason == string(gts.ParseStopCancelled):
		d.Outcome = syntax.Cancelled
	case d.StopReason == string(gts.ParseStopIterationLimit) || d.StopReason == string(gts.ParseStopStackDepthLimit) || d.StopReason == string(gts.ParseStopNodeLimit) || d.StopReason == string(gts.ParseStopMemoryBudget) || d.StopReason == string(gts.ParseStopReuseBudget):
		d.Outcome = syntax.ResourceLimit
	case d.StopReason == string(gts.ParseStopInvariantViolation) || parseErr != nil:
		d.Outcome = syntax.InvariantViolation
	case d.StoppedEarly || d.Truncated:
		d.Outcome = syntax.EarlyStop
	case !d.RootPresent:
		d.Outcome = syntax.InvariantViolation
	case d.RootStartByte != 0 || uint64(d.RootEndByte) != uint64(len(source)) || d.StopReason != string(gts.ParseStopAccepted):
		d.Outcome = syntax.EarlyStop
	case d.HasError || d.HasMissing:
		d.Outcome = syntax.AcceptedWithErrors
	default:
		d.Outcome = syntax.AcceptedClean
	}
	result := syntax.Result{Diagnostics: d}
	if d.Outcome == syntax.AcceptedClean {
		result.Tree = &tree{raw: raw, lang: lang, source: source, nodes: nodes}
		return result, nil
	}
	if raw != nil {
		raw.Release()
	}
	if d.Outcome == syntax.AcceptedWithErrors {
		// Retain the usable snapshot, but release the non-clean backend handle.
		result.Tree = &tree{nodes: nodes}
		return result, nil
	}
	return result, errors.New(string(d.Outcome))
}

func prepareEdit(req syntax.Request, lang *gts.Language) (*gts.Tree, error) {
	if req.Previous == nil && req.Edit == nil {
		return nil, nil
	}
	old, ok := req.Previous.(*tree)
	if !ok || old.raw == nil || old.edited || old.lang != lang || req.Edit == nil {
		return nil, errors.New("invalid previous tree")
	}
	e := req.Edit
	if e.StartByte > e.OldEndByte || e.StartByte > e.NewEndByte || uint64(e.OldEndByte) > uint64(len(old.source)) || uint64(e.NewEndByte) > uint64(len(req.Source)) {
		return nil, errors.New("invalid edit range")
	}
	for _, edge := range []struct {
		source []byte
		offset uint32
	}{{old.source, e.StartByte}, {old.source, e.OldEndByte}, {req.Source, e.StartByte}, {req.Source, e.NewEndByte}} {
		if int(edge.offset) < len(edge.source) && !utf8.RuneStart(edge.source[edge.offset]) {
			return nil, errors.New("edit splits UTF-8 rune")
		}
	}
	if !bytes.Equal(old.source[:e.StartByte], req.Source[:e.StartByte]) || !bytes.Equal(old.source[e.OldEndByte:], req.Source[e.NewEndByte:]) {
		return nil, errors.New("edit does not describe source change")
	}
	old.raw.Edit(gts.InputEdit{StartByte: e.StartByte, OldEndByte: e.OldEndByte, NewEndByte: e.NewEndByte,
		StartPoint: point(old.source[:e.StartByte]), OldEndPoint: point(old.source[:e.OldEndByte]), NewEndPoint: point(req.Source[:e.NewEndByte])})
	old.edited = true
	return old.raw, nil
}

func point(prefix []byte) gts.Point {
	return gts.Point{Row: uint32(bytes.Count(prefix, []byte{'\n'})), Column: uint32(len(prefix) - bytes.LastIndexByte(prefix, '\n') - 1)}
}

func snapshot(root *gts.Node, lang *gts.Language) []syntax.Node {
	type visit struct {
		node   *gts.Node
		parent int
		field  string
	}
	stack := []visit{{root, -1, ""}}
	var nodes []syntax.Node
	for len(stack) > 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n := v.node
		start, end := n.StartPoint(), n.EndPoint()
		index := len(nodes)
		nodes = append(nodes, syntax.Node{Type: n.Type(lang), Field: v.field, Parent: v.parent, Named: n.IsNamed(), Extra: n.IsExtra(), Missing: n.IsMissing(), Error: n.IsError(), StartByte: n.StartByte(), EndByte: n.EndByte(), StartPoint: syntax.Point{Row: start.Row, Column: start.Column}, EndPoint: syntax.Point{Row: end.Row, Column: end.Column}})
		for i := n.ChildCount() - 1; i >= 0; i-- {
			stack = append(stack, visit{n.Child(i), index, n.FieldNameForChild(i, lang)})
		}
	}
	return nodes
}
