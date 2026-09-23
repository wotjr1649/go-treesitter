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

	gts "github.com/wotjr1649/go-treesitter/internal/runtime"
	"github.com/wotjr1649/go-treesitter/internal/runtime/grammars"
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
		ExpectedEOFByte: uint64(len(req.Source)), Route: "not_run",
		InputLimitBytes: req.Limits.MaxInputBytes, SnapshotNodeLimit: req.Limits.MaxSnapshotNodes}
	if ctx == nil {
		d.Outcome = syntax.NotRun
		return failed(d, errors.New("nil parse context"))
	}
	if err := ctx.Err(); err != nil {
		d.Outcome = syntax.Cancelled
		return failed(d, err)
	}
	limits := req.Limits
	if req.Timeout < 0 || limits.MaxInputBytes < 0 || limits.MaxSnapshotNodes < 0 || limits.MemoryBudgetBytes < 0 || limits.IterationLimit < 0 || limits.NodeLimit < 0 || limits.StackDepthLimit < 0 {
		d.Outcome = syntax.NotRun
		return failed(d, errors.New("negative parse limit"))
	}
	if limits.MaxInputBytes > 0 && len(req.Source) > limits.MaxInputBytes {
		d.Outcome, d.LimitReason = syntax.ResourceLimit, "input_bytes"
		return failed(d, errors.New(string(d.Outcome)))
	}
	entry := grammars.DetectLanguageByName(req.Language)
	if req.Language == "" {
		entry = grammars.DetectLanguage(filepath.ToSlash(req.Filename))
	}
	valid := uint64(len(req.Source)) <= math.MaxUint32 && utf8.Valid(req.Source) && entry != nil
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
		p.SetMemoryBudgetBytes(limits.MemoryBudgetBytes)
		p.SetParseWorkLimits(gts.ParseWorkLimits{IterationLimit: limits.IterationLimit, NodeLimit: limits.NodeLimit, StackDepthLimit: limits.StackDepthLimit})
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
	var snapshotErr error
	if raw != nil {
		rt := raw.ParseRuntime()
		// The runtime charges growth beyond retained slabs. Admit a result only
		// when its reported arena/scratch footprint fits the caller's budget too.
		memoryExceeded := limits.MemoryBudgetBytes > 0 &&
			(rt.ArenaBytesAllocated > limits.MemoryBudgetBytes || rt.ScratchBytesAllocated > limits.MemoryBudgetBytes-rt.ArenaBytesAllocated)
		root := raw.RootNode()
		if root != nil {
			d.RootPresent = true
			d.RootStartByte, d.RootEndByte = root.StartByte(), root.EndByte()
			d.HasError = root.HasError()
			if memoryExceeded {
				snapshotErr, d.LimitReason = errRuntimeMemory, "runtime_memory"
			} else {
				nodes, snapshotErr = boundedSnapshot(ctx, root, lang, limits.MaxSnapshotNodes)
			}
			d.SnapshotNodes, d.SnapshotComplete = len(nodes), snapshotErr == nil
			if errors.Is(snapshotErr, errSnapshotLimit) {
				d.LimitReason = "snapshot_nodes"
			}
			for _, n := range nodes {
				d.HasMissing = d.HasMissing || n.Missing
			}
		}
		d.StopReason, d.StoppedEarly = string(rt.StopReason), raw.ParseStoppedEarly()
		d.TokensConsumed, d.LastTokenEndByte = rt.TokensConsumed, rt.LastTokenEndByte
		d.ExpectedEOFByte = uint64(rt.ExpectedEOFByte)
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
	case d.RootStartByte > d.RootEndByte || uint64(d.RootEndByte) != uint64(len(source)) ||
		uint64(d.LastTokenEndByte) != uint64(len(source)) || d.ExpectedEOFByte != uint64(len(source)) ||
		d.StopReason != string(gts.ParseStopAccepted):
		d.Outcome = syntax.EarlyStop
	case errors.Is(snapshotErr, errSnapshotLimit) || errors.Is(snapshotErr, errRuntimeMemory):
		d.Outcome = syntax.ResourceLimit
	case snapshotErr != nil:
		d.Outcome = syntax.InvariantViolation
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
	if err := ctx.Err(); err != nil {
		return failed(d, err)
	}
	return failed(d, errors.New(string(d.Outcome)))
}

// Preserve an upstream error type when available, otherwise record our own.
// Error text never enters the receipt, because upstream text may contain source.
func failed(d syntax.Diagnostics, err error) (syntax.Result, error) {
	if d.ErrorType == "" {
		d.ErrorType = fmt.Sprintf("%T", err)
	}
	return syntax.Result{Diagnostics: d}, err
}

func prepareEdit(req syntax.Request, lang *gts.Language) (*gts.Tree, error) {
	if req.Previous == nil && req.Edit == nil {
		return nil, nil
	}
	old, ok := req.Previous.(*tree)
	if !ok || old == nil || old.raw == nil || old.edited || old.lang != lang || req.Edit == nil {
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
	nodes, err := boundedSnapshot(context.Background(), root, lang, 0)
	if err != nil {
		panic(err) // Test-only full snapshot of an existing runtime tree.
	}
	return nodes
}

var errSnapshotLimit = errors.New("snapshot node limit")
var errRuntimeMemory = errors.New("runtime memory receipt exceeds limit")

// Keep traversal state proportional to depth, including for very wide roots.
// Holding every sibling on a stack would defeat a small MaxSnapshotNodes cap.
func boundedSnapshot(ctx context.Context, root *gts.Node, lang *gts.Language, limit int) ([]syntax.Node, error) {
	type frame struct {
		node             *gts.Node
		parent           int
		field            string
		index, nextChild int
	}
	stack := []frame{{node: root, parent: -1, index: -1}}
	var nodes []syntax.Node
	for len(stack) > 0 {
		v := &stack[len(stack)-1]
		if v.index == -1 {
			if len(nodes)%256 == 0 {
				if err := ctx.Err(); err != nil {
					return nodes, err
				}
			}
			if limit > 0 && len(nodes) == limit {
				return nodes, errSnapshotLimit
			}
			n := v.node
			if n == nil {
				return nodes, errors.New("nil snapshot node")
			}
			start, end := n.StartPoint(), n.EndPoint()
			v.index = len(nodes)
			nodes = append(nodes, syntax.Node{Type: n.Type(lang), Field: v.field, Parent: v.parent, Named: n.IsNamed(), Extra: n.IsExtra(), Missing: n.IsMissing(), Error: n.IsError(), StartByte: n.StartByte(), EndByte: n.EndByte(), StartPoint: syntax.Point{Row: start.Row, Column: start.Column}, EndPoint: syntax.Point{Row: end.Row, Column: end.Column}})
		}
		if v.nextChild == v.node.ChildCount() {
			stack = stack[:len(stack)-1]
			continue
		}
		i := v.nextChild
		v.nextChild++
		stack = append(stack, frame{node: v.node.Child(i), parent: v.index, field: v.node.FieldNameForChild(i, lang), index: -1})
	}
	return nodes, nil
}
