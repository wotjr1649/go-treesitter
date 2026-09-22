package gtsadapter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

var corpus = flag.String("corpus", "", "absolute corpus directory")
var cell = flag.String("cell", "", "A, B, C or D; measurement is opt-in")
var version = flag.String("expected-version", "", "expected resolved dependency")
var disabled = flag.Bool("compact-disabled", false, "disable compact admission")

type fixture struct {
	Name   string
	Bytes  int
	SHA256 string
	source []byte
}

func loadCorpus(t *testing.T) []fixture {
	t.Helper()
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(*corpus, name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var manifest struct {
		LicenseSHA256 string `json:"license_sha256"`
		Fixtures      []fixture
	}
	if err := json.Unmarshal(read("manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Fixtures) != 3 || fmt.Sprintf("%x", sha256.Sum256(read("LICENSE.md"))) != manifest.LicenseSHA256 {
		t.Fatal("corpus/license identity mismatch")
	}
	for i := range manifest.Fixtures {
		f := &manifest.Fixtures[i]
		if filepath.Base(f.Name) != f.Name {
			t.Fatal("unbounded corpus path")
		}
		f.source = read(f.Name)
		if len(f.source) != f.Bytes || fmt.Sprintf("%x", sha256.Sum256(f.source)) != f.SHA256 || bytes.ContainsRune(f.source, '\r') {
			t.Fatalf("provenance failure: %s", f.Name)
		}
	}
	return manifest.Fixtures
}

func TestCorpusIdentity(t *testing.T) {
	for _, f := range loadCorpus(t) {
		t.Logf("%s bytes=%d sha256=%s", f.Name, f.Bytes, f.SHA256)
	}
}

func TestComparison(t *testing.T) {
	if *cell == "" || *version == "" {
		t.Fatal("measurement requires explicit cell and expected-version")
	}
	fixtures := loadCorpus(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	info, err := exec.CommandContext(ctx, "go", "list", "-m", "-json", "github.com/odvcencio/gotreesitter").Output()
	if err != nil {
		t.Fatal(err)
	}
	var module struct {
		Path, Version, Dir, Sum string
		Replace                 any
	}
	if err := json.Unmarshal(info, &module); err != nil {
		t.Fatal(err)
	}
	if module.Version != *version || module.Replace != nil {
		t.Fatal("resolved comparison version mismatch")
	}
	identityFile := filepath.Join(*corpus, "..", "..", "internal", "provenance", "identities.json")
	identityBytes, err := os.ReadFile(identityFile)
	if err != nil {
		t.Fatal(err)
	}
	var identities struct {
		Grammars map[string]struct {
			Blob   string `json:"blob_sha256"`
			Commit string
		}
	}
	if err := json.Unmarshal(identityBytes, &identities); err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile(filepath.Join(module.Dir, "grammars", "grammar_blobs", "c_sharp.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(blob)) != identities.Grammars["c_sharp"].Blob {
		t.Fatal("C# grammar blob mismatch")
	}
	if !gts.AdmissionCandidateRouteDefault() {
		t.Fatal("default-route process configuration changed")
	}
	loadStarted := time.Now()
	lang := grammars.DetectLanguageByName("c_sharp").Language()
	loadNS := time.Since(loadStarted).Nanoseconds()
	t.Logf("cell=%s module=%s version=%s sum=%s grammar_commit=%s grammar_sha256=%x grammar_load_ns=%d repetitions=1 profiler=off", *cell, module.Path, module.Version, module.Sum, identities.Grammars["c_sharp"].Commit, sha256.Sum256(blob), loadNS)
	for _, f := range fixtures {
		p := gts.NewParser(lang)
		p.SetTimeoutMicros(60_000_000)
		if *disabled {
			p.SetAdmissionCandidateRoute(false)
		}
		routedBefore, fallbackBefore := gts.AdmissionCandidateCounters()
		started := time.Now()
		tree, err := p.Parse(f.source)
		elapsed := time.Since(started).Nanoseconds()
		routedAfter, fallbackAfter := gts.AdmissionCandidateCounters()
		if err != nil || tree == nil {
			if tree != nil {
				tree.Release()
			}
			t.Fatalf("parse failed: type=%T", err)
		}
		root := tree.RootNode()
		if root == nil {
			tree.Release()
			t.Fatal("missing root")
		}
		nodes, missing := 0, false
		stack := []*gts.Node{root}
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			nodes++
			missing = missing || n.IsMissing()
			for i := 0; i < n.ChildCount(); i++ {
				stack = append(stack, n.Child(i))
			}
		}
		rt := tree.ParseRuntime()
		route := "classic"
		if routedAfter > routedBefore {
			route = "compact"
		}
		if fallbackAfter > fallbackBefore {
			route = "classic_after_decline"
		}
		if rt.ForestFastPath {
			route = "forest"
		}
		fallback := ""
		if fallbackAfter > fallbackBefore {
			fallback = gts.AdmissionCandidateLastFallbackReason()
		}
		complete := !tree.ParseStoppedEarly() && !rt.Truncated && rt.StopReason == gts.ParseStopAccepted && root.StartByte() == 0 && root.EndByte() == uint32(len(f.source))
		outcome := "accepted_clean"
		if root.HasError() || missing {
			outcome = "accepted_with_errors"
		}
		if !complete {
			outcome = "incomplete"
		}
		record := map[string]any{"cell": *cell, "version": module.Version, "compact_disabled": *disabled, "file": f.Name, "source_sha256": f.SHA256, "input_bytes": f.Bytes, "outcome": outcome, "stop_reason": string(rt.StopReason), "stopped_early": tree.ParseStoppedEarly(), "truncated": rt.Truncated, "has_error": root.HasError(), "has_missing": missing, "root_start": root.StartByte(), "root_end": root.EndByte(), "route": route, "raw_fallback": fallback, "routed_delta": routedAfter - routedBefore, "fallback_delta": fallbackAfter - fallbackBefore, "walk_nodes": nodes, "allocated_nodes": rt.NodesAllocated, "tokens": rt.TokensConsumed, "last_token_end": rt.LastTokenEndByte, "expected_eof": rt.ExpectedEOFByte, "parse_api_ns": elapsed}
		encoded, err := json.Marshal(record)
		tree.Release()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("MEASUREMENT %s", encoded)
		if !complete {
			t.Fatal("incomplete cell; receipt retained")
		}
	}
}
