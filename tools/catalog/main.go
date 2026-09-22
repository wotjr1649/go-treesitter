// Command catalog checks one bounded grammar sample through the public API.
// Run each language in its own process to isolate grammar caches and failures.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"time"

	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/syntax"
)

type observation struct {
	Complete             bool
	Outcome              syntax.Outcome
	HasError, HasMissing bool
	StopReason, Digest   string
	Nodes                int
}

func observe(result syntax.Result) observation {
	var nodes []syntax.Node
	if result.Tree != nil {
		nodes = result.Tree.Nodes()
	}
	data, err := json.Marshal(nodes)
	if err != nil {
		panic(err)
	}
	return observation{result.Complete(), result.Outcome, result.HasError, result.HasMissing,
		result.StopReason, fmt.Sprintf("%x", sha256.Sum256(data)), len(nodes)}
}

func closeTree(result syntax.Result) {
	if result.Tree != nil {
		result.Tree.Close()
	}
}

func main() {
	language := flag.String("language", "", "required registered grammar name")
	repeat := flag.Int("repeat", 3, "fresh runs (1..100)")
	flag.Parse()
	source, err := io.ReadAll(io.LimitReader(os.Stdin, (1<<20)+1))
	if err != nil || len(source) < 2 || len(source) > 1<<20 || *language == "" || *repeat < 1 || *repeat > 100 {
		fmt.Fprintln(os.Stderr, "require language, 2..1048576 source bytes, and 1..100 repeats")
		os.Exit(2)
	}
	parser := treesitter.New()
	request := syntax.Request{Language: *language, Source: source, Timeout: 2 * time.Second,
		Limits: syntax.Limits{MaxInputBytes: 1 << 20, MaxSnapshotNodes: 100_000, MemoryBudgetBytes: 64 << 20}}
	receipt := struct {
		Language, SourceSHA256                                   string
		Fresh                                                    []observation
		Deterministic, Clean, EditEqual, Cancellation, Admission bool
		Errors                                                   []string
		FailureDiagnostics                                       []syntax.Diagnostics
		EditIncremental, EditFresh                               []syntax.Node
	}{Language: *language, SourceSHA256: fmt.Sprintf("%x", sha256.Sum256(source)), Deterministic: true, Clean: true}
	for i := 0; i < *repeat; i++ {
		result, err := parser.Parse(context.Background(), request)
		entry := observe(result)
		receipt.Fresh = append(receipt.Fresh, entry)
		receipt.Clean = receipt.Clean && err == nil && entry.Complete && entry.Outcome == syntax.AcceptedClean
		if err != nil || !entry.Complete || entry.Outcome != syntax.AcceptedClean {
			receipt.FailureDiagnostics = append(receipt.FailureDiagnostics, result.Diagnostics)
		}
		receipt.Deterministic = receipt.Deterministic && entry == receipt.Fresh[0]
		closeTree(result)
		closeTree(result) // Close is idempotent.
	}
	first, err := parser.Parse(context.Background(), request)
	if err == nil && first.Complete() && first.Tree != nil {
		edited := bytes.Clone(source)
		edited = append(edited, '\n')
		request.Source = edited
		request.Previous = first.Tree
		request.Edit = &syntax.Edit{StartByte: uint32(len(source)), OldEndByte: uint32(len(source)), NewEndByte: uint32(len(edited))}
		incremental, editErr := parser.Parse(context.Background(), request)
		request.Previous, request.Edit = nil, nil
		fresh, freshErr := parser.Parse(context.Background(), request)
		receipt.EditEqual = editErr == nil && freshErr == nil && incremental.Complete() && fresh.Complete() &&
			incremental.Tree != nil && fresh.Tree != nil && incremental.Outcome == fresh.Outcome &&
			reflect.DeepEqual(incremental.Tree.Nodes(), fresh.Tree.Nodes())
		if !receipt.EditEqual {
			receipt.FailureDiagnostics = append(receipt.FailureDiagnostics, incremental.Diagnostics, fresh.Diagnostics)
			if incremental.Tree != nil {
				receipt.EditIncremental = incremental.Tree.Nodes()
			}
			if fresh.Tree != nil {
				receipt.EditFresh = fresh.Tree.Nodes()
			}
		}
		closeTree(incremental)
		closeTree(fresh)
	}
	closeTree(first)
	request.Source, request.Previous, request.Edit = source, nil, nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stopped, err := parser.Parse(ctx, request)
	receipt.Cancellation = err != nil && stopped.Outcome == syntax.Cancelled && stopped.Tree == nil
	closeTree(stopped)
	request.Limits.MaxInputBytes = len(source) - 1
	limited, err := parser.Parse(context.Background(), request)
	receipt.Admission = err != nil && limited.Outcome == syntax.ResourceLimit && limited.Tree == nil && limited.LimitReason == "input_bytes"
	closeTree(limited)
	for label, passed := range map[string]bool{"fresh_clean": receipt.Clean, "determinism": receipt.Deterministic,
		"edit_equal": receipt.EditEqual, "cancellation": receipt.Cancellation, "input_admission": receipt.Admission} {
		if !passed {
			receipt.Errors = append(receipt.Errors, label)
		}
	}
	sort.Strings(receipt.Errors)
	if err := json.NewEncoder(os.Stdout).Encode(receipt); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if len(receipt.Errors) > 0 {
		os.Exit(1)
	}
}
