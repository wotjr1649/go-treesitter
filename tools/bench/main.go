// Command bench measures the public treesitter API for the engine gates (speed and memory), one grammar per binary
// (build tag lang_<name>). It is driven by a local harness that interleaves it with the native C
// executables in the same time window. Output is plain text on stdout; the driver parses it.
//
//	bench speed  -file F -reps N [-op parse|snapshot]   per rep: "rep,total_ms,parse_ms,info" (rep 0 is the warm-up)
//	bench corpus -list L -reps N                        per file: "name,bytes,outcome,ms0;ms1;..." (ms0 is the warm-up)
//	bench mem    -file F [-op parse|snapshot]           one line of key=value (one fresh process per run)
//
// mem also reports the ADR-0022 release criterion: priv_* are process private bytes (PROCESS_MEMORY_COUNTERS_EX
// PrivateUsage) and regions_*/region_commit_* count the engine's linear-memory regions found by VirtualQuery (private
// allocations of at least engineRegionMin reserved bytes) while the parser is alive and after Close + GC.
//
// speed/parse times Parse + Tree.Close (the native driver times parse + ts_tree_delete); parse_ms is Parse alone.
// speed/snapshot times ParseSnapshot (parse + chunked export + Go copy + tree delete). QPC resolution, like the spike.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strings"

	treesitter "github.com/wotjr1649/go-treesitter"
)

func fatal(a ...any) {
	fmt.Fprintln(os.Stderr, a...)
	os.Exit(1)
}

func newParser() *treesitter.Parser {
	p, err := treesitter.NewParser(language())
	if err != nil {
		fatal("NewParser:", err)
	}
	return p
}

func main() {
	if len(os.Args) < 2 {
		fatal("usage: bench speed|corpus|mem ...")
	}
	initPlatform()
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	file := fs.String("file", "", "input file")
	list := fs.String("list", "", "file with one input path per line")
	reps := fs.Int("reps", 5, "timed reps (a warm-up rep 0 is added)")
	op := fs.String("op", "parse", "parse | snapshot")
	fs.Parse(os.Args[2:])
	if *op != "parse" && *op != "snapshot" {
		fatal("bad -op", *op)
	}
	switch os.Args[1] {
	case "speed":
		speed(*file, *reps, *op)
	case "corpus":
		corpus(*list, *reps)
	case "mem":
		mem(*file, *op)
	default:
		fatal("unknown mode", os.Args[1])
	}
}

func speed(file string, reps int, op string) {
	src, err := os.ReadFile(file)
	if err != nil {
		fatal(err)
	}
	p := newParser()
	defer p.Close()
	ctx := context.Background()
	fmt.Printf("# lang=%s op=%s bytes=%d go=%s\n", language().Name(), op, len(src), runtime.Version())
	for i := 0; i <= reps; i++ {
		var total, parse float64
		var info string
		if op == "parse" {
			t0 := now()
			tree, err := p.Parse(ctx, src, nil)
			t1 := now()
			if err != nil {
				fatal("Parse:", err)
			}
			info = fmt.Sprintf("%s/%d", tree.Outcome(), tree.InputBytes())
			tree.Close()
			t2 := now()
			total, parse = ms(t0, t2), ms(t0, t1)
		} else {
			t0 := now()
			snap, err := p.ParseSnapshot(ctx, src)
			t1 := now()
			if err != nil {
				fatal("ParseSnapshot:", err)
			}
			total, parse = ms(t0, t1), ms(t0, t1)
			info = fmt.Sprintf("end=%d/%d nodes=%d", snap.Root().EndByte(), len(src), snap.Len())
		}
		fmt.Printf("%d,%.4f,%.4f,%s\n", i, total, parse, info)
	}
}

func corpus(list string, reps int) {
	b, err := os.ReadFile(list)
	if err != nil {
		fatal(err)
	}
	p := newParser()
	defer p.Close()
	ctx := context.Background()
	for _, path := range strings.Fields(string(b)) {
		src, err := os.ReadFile(path)
		if err != nil {
			fatal(err)
		}
		var samples []string
		outcome := ""
		for i := 0; i <= reps; i++ {
			t0 := now()
			tree, err := p.Parse(ctx, src, nil)
			if err != nil {
				outcome = "ERR:" + strings.ReplaceAll(err.Error(), ",", ";")
				samples = append(samples, "-1")
				break
			}
			outcome = tree.Outcome().String()
			tree.Close()
			samples = append(samples, fmt.Sprintf("%.4f", ms(t0, now())))
		}
		fmt.Printf("%s,%d,%s,%s\n", path[strings.LastIndexAny(path, `\/`)+1:], len(src), outcome, strings.Join(samples, ";"))
	}
}

// mem: start = working set at main (before the input is read); peak = OS peak working set of the whole process;
// after = working set once parser, tree/snapshot and input are dropped and the Go heap is returned (GC + FreeOSMemory).
func mem(file, op string) {
	start := counters()
	regStart, _ := engineRegions()
	src, err := os.ReadFile(file)
	if err != nil {
		fatal(err)
	}
	n := len(src)
	ctx := context.Background()
	p := newParser()
	afterInit := counters()
	var end int64 = -1
	var outcome string
	var nodes int
	var keptWS, peakParse uintptr
	var regParsed int
	var regParsedCommit uint64
	var privParsed uintptr
	if op == "parse" {
		tree, err := p.Parse(ctx, src, nil)
		if err != nil {
			fatal("Parse:", err)
		}
		outcome = tree.Outcome().String()
		end = int64(tree.InputBytes()) // the public Tree has no root accessor; end==bytes is checked in the snapshot op
		c := counters()
		peakParse, privParsed = c.peakWorkingSetSize, c.privateUsage
		regParsed, regParsedCommit = engineRegions()
		tree.Close()
	} else {
		snap, err := p.ParseSnapshot(ctx, src)
		if err != nil {
			fatal("ParseSnapshot:", err)
		}
		end, nodes, outcome = int64(snap.Root().EndByte()), snap.Len(), "snapshot"
		c := counters()
		peakParse, privParsed = c.peakWorkingSetSize, c.privateUsage
		regParsed, regParsedCommit = engineRegions() // the parser is still open here
		p.Close()                                    // instance gone, snapshot still alive: its working set is the snapshot's own size
		src = nil
		runtime.GC()
		debug.FreeOSMemory()
		keptWS = counters().workingSetSize
		runtime.KeepAlive(snap)
	}
	afterParse := counters()
	if err := p.Close(); err != nil {
		fatal("Close:", err)
	}
	src = nil
	runtime.GC()
	debug.FreeOSMemory()
	after := counters()
	regAfter, regAfterCommit := engineRegions()
	fmt.Printf("priv_start=%d priv_after_init=%d priv_parsed=%d priv_after=%d peak_priv=%d regions_start=%d regions_parsed=%d region_commit_parsed=%d regions_after=%d region_commit_after=%d ",
		start.privateUsage, afterInit.privateUsage, privParsed, after.privateUsage, after.peakPagefileUsage, regStart, regParsed, regParsedCommit, regAfter, regAfterCommit)
	fmt.Printf("op=%s bytes=%d end=%d outcome=%s nodes=%d start_ws=%d ws_after_init=%d peak_after_init=%d peak_after_parse=%d peak_ws=%d ws_after_close_gc=%d ws_instance_closed_snapshot_kept=%d ws_after_parse=%d\n",
		op, n, end, outcome, nodes, start.workingSetSize, afterInit.workingSetSize, afterInit.peakWorkingSetSize, peakParse, after.peakWorkingSetSize,
		after.workingSetSize, keptWS, afterParse.workingSetSize)
}
