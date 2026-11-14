package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/metrics"
	"strings"
)

// Diagnostics for the memory lane (BENCH_MEM_DIAG=1, never set by the gate runs): at each stage of mem() one "diag"
// line with the process counters and the Go runtime's own account of its heap, so that a footprint the OS still
// charges after Close can be told apart from a heap the program still holds:
//
//	d_heap_objects  bytes of live+unswept objects      d_heap_free      free spans not yet given back
//	d_heap_released free spans given back to the OS    d_heap_unused    in-use span space not used by objects
//	d_gc_live       heap marked live by the last GC    d_gcs            completed GC cycles
//
// On darwin platDiag adds the vmmap region-type summary (diag_darwin.go).
var diagOn = os.Getenv("BENCH_MEM_DIAG") != ""

var diagMetrics = []struct{ key, name string }{
	{"d_heap_objects", "/memory/classes/heap/objects:bytes"},
	{"d_heap_free", "/memory/classes/heap/free:bytes"},
	{"d_heap_released", "/memory/classes/heap/released:bytes"},
	{"d_heap_unused", "/memory/classes/heap/unused:bytes"},
	{"d_heap_stacks", "/memory/classes/heap/stacks:bytes"},
	{"d_go_total", "/memory/classes/total:bytes"},
	{"d_gc_live", "/gc/heap/live:bytes"},
	{"d_gcs", "/gc/cycles/total:gc-cycles"},
}

func diagStage(stage string) {
	if !diagOn {
		return
	}
	samples := make([]metrics.Sample, len(diagMetrics))
	for i, m := range diagMetrics {
		samples[i].Name = m.name
	}
	metrics.Read(samples)
	c := counters()
	var sb strings.Builder
	fmt.Fprintf(&sb, "diag stage=%s d_private=%d d_ws=%d", stage, c.privateUsage, c.workingSetSize)
	for i, s := range samples {
		if s.Value.Kind() == metrics.KindUint64 {
			fmt.Fprintf(&sb, " %s=%d", diagMetrics[i].key, s.Value.Uint64())
		}
	}
	fmt.Fprintf(&sb, " d_godebug=%q d_goversion=%s", os.Getenv("GODEBUG"), runtime.Version())
	fmt.Println(sb.String())
	platDiag(stage)
}
