// Command compare orchestrates the comparative benchmark inside the isolated workflow (.github/workflows/compare.yml).
// It imports no candidate library: workers (adapters/*) are separate binaries it starts as child processes.
//
//	compare select  -data D -large L -large-manifest M -out W   freeze the inputs and edits
//	compare run     -workload W -bin B -out R [-smoke]          paired timing/allocation/memory measurements
//	compare check   -workload W -bin B -refs F -out R           S-expression correctness against the native references
//	compare collect -in R -out C                                validate a result directory (schema, paths, sizes) and hash it
//	compare report  -in R [-out O]                              summary.md and the aggregate CSVs
//	compare seal    -dir D                                      cap the final artifact set and hash it
//	compare gate-nocc -builds F                                 fail unless the product's compiler-free build and smoke parse succeeded
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: compare select|run|check|collect|report|seal|gate-nocc [flags]")
		os.Exit(2)
	}
	cmds := map[string]func([]string) error{"select": cmdSelect, "run": cmdRun, "check": cmdCheck, "collect": cmdCollect, "report": cmdReport, "seal": cmdSeal, "gate-nocc": cmdGateNocc}
	f, ok := cmds[os.Args[1]]
	if !ok {
		fmt.Fprintln(os.Stderr, "unknown command", os.Args[1])
		os.Exit(2)
	}
	if err := f(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "compare:", err)
		os.Exit(1)
	}
}
