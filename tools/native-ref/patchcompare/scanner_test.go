package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestScannerExpectations(t *testing.T) {
	makeDiff := func(id string, before scannerOutcome, afterKind string) scannerDifference {
		after, err := parseScannerDump(dump(false, 3, afterKind))
		if err != nil {
			t.Fatal(err)
		}
		return scannerDifference{scannerResult{
			scannerInput{id, 3, sha([]byte("abc"))}, before, scannerOutcome{Status: "tree", Tree: &after},
		}, nil, "reviewed input-specific change"}
	}
	old, err := parseScannerDump(dump(false, 3, "before"))
	if err != nil {
		t.Fatal(err)
	}
	makeSidecar := func() scannerExpectation {
		crash := makeDiff("regression/second", scannerOutcome{Status: "baseline-crash"}, "recovered")
		crash.BaselineCrashes = map[string]string{"linux/amd64": "access-violation"}
		differences := []scannerDifference{
			makeDiff("corpus/first", scannerOutcome{Status: "tree", Tree: &old}, "new"),
			crash,
		}
		// The unchanged row also contributes to the all-results digest.
		unchanged := makeDiff("corpus/unchanged", scannerOutcome{Status: "tree", Tree: &old}, "before")
		rows := []scannerResult{differences[0].scannerResult, unchanged.scannerResult, differences[1].scannerResult}
		return scannerExpectation{1, "rust", json.RawMessage(`{"commit":"first","runtime":"second"}`), "inputs",
			[]scannerComparison{
				{"patches/rust/1.patch", "before", "after", scannerDigest(rows), "known scanner fix", "new pin with fix", differences},
				{"patches/rust/2.patch", "after", "last", "results", "known scanner fix", "new pin with fix", []scannerDifference{}},
			}}
	}

	for _, tc := range []struct {
		name, errorText string
		mutate          func(want, got *scannerExpectation)
	}{
		{"canonical identity", "", func(want, _ *scannerExpectation) {
			want.Identity = json.RawMessage(`{ "runtime" : "second", "commit" : "first" }`)
		}},
		{"identity drift", "build identity mismatch", func(want, _ *scannerExpectation) {
			want.Identity = json.RawMessage(`{"commit":"new","runtime":"second"}`)
		}},
		{"input drift", "input digest mismatch", func(_, got *scannerExpectation) {
			got.InputsSHA256 = "new inputs"
		}},
		{"patch ordering", "comparison 1 identity mismatch", func(want, _ *scannerExpectation) {
			want.Comparisons[0], want.Comparisons[1] = want.Comparisons[1], want.Comparisons[0]
		}},
		{"stale patch", "expected 2 scanner patches, found 1", func(_, got *scannerExpectation) {
			got.Comparisons = got.Comparisons[:1]
		}},
		{"source digest drift", "comparison 1 identity mismatch", func(_, got *scannerExpectation) {
			got.Comparisons[0].AfterSourcesSHA256 = "new scanner"
		}},
		{"all rows drift", "all-results digest mismatch", func(_, got *scannerExpectation) {
			pair := &got.Comparisons[0]
			drift := old
			drift.DumpSHA256 = sha(dump(false, 3, "both changed"))
			unchanged := makeDiff("corpus/unchanged", scannerOutcome{Status: "tree", Tree: &drift}, "both changed")
			rows := []scannerResult{pair.Differences[0].scannerResult, unchanged.scannerResult, pair.Differences[1].scannerResult}
			pair.ResultsSHA256 = scannerDigest(rows)
		}},
		{"new row", "new difference for \"corpus/first\"", func(want, _ *scannerExpectation) {
			want.Comparisons[0].Differences = want.Comparisons[0].Differences[1:]
		}},
		{"missing row", "missing difference for \"regression/second\"", func(_, got *scannerExpectation) {
			got.Comparisons[0].Differences = got.Comparisons[0].Differences[:1]
		}},
		{"changed clean tree", "changed difference for \"corpus/first\"", func(_, got *scannerExpectation) {
			got.Comparisons[0].Differences[0].After.Tree.DumpSHA256 = "drift"
		}},
		{"baseline no longer crashes", "changed difference for \"regression/second\"", func(_, got *scannerExpectation) {
			got.Comparisons[0].Differences[1].Before = scannerOutcome{Status: "tree", Tree: &old}
		}},
		{"changed crash class", "unreviewed abort baseline crash on linux/amd64", func(_, got *scannerExpectation) {
			got.Comparisons[0].Differences[1].BaselineCrashes = map[string]string{"linux/amd64": "abort"}
		}},
		{"unrecorded crash platform", "unreviewed access-violation baseline crash on linux/arm64", func(_, got *scannerExpectation) {
			got.Comparisons[0].Differences[1].BaselineCrashes = map[string]string{"linux/arm64": "access-violation"}
		}},
		{"other platform record", "", func(want, _ *scannerExpectation) {
			want.Comparisons[0].Differences[1].BaselineCrashes["windows/amd64"] = "stack-overflow"
		}},
		{"missing crash classes", "needs per-platform crash classes", func(want, _ *scannerExpectation) {
			want.Comparisons[0].Differences[1].BaselineCrashes = nil
		}},
		{"duplicate row", "duplicated or out of order", func(want, _ *scannerExpectation) {
			rows := want.Comparisons[0].Differences
			want.Comparisons[0].Differences = append(rows, rows[1])
		}},
		{"required reason", "reviewed reason and retirement condition", func(want, _ *scannerExpectation) {
			want.Comparisons[0].Reason = ""
		}},
		{"required clean review", "missing review", func(want, _ *scannerExpectation) {
			want.Comparisons[0].Differences[0].Review = ""
		}},
		{"required crash review", "missing review", func(want, _ *scannerExpectation) {
			want.Comparisons[0].Differences[1].Review = ""
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want, got := makeSidecar(), makeSidecar()
			tc.mutate(&want, &got)
			err := checkScannerExpectations(want, got)
			if (tc.errorText == "" && err != nil) || (tc.errorText != "" && (err == nil || !strings.Contains(err.Error(), tc.errorText))) {
				t.Fatalf("error = %v, want %q", err, tc.errorText)
			}
		})
	}
}

func TestScannerJSONAndInputs(t *testing.T) {
	for _, invalid := range []string{
		`{"identity":{"pin":"first","pin":"second"}}`,
		`{"grammar":"rust","Grammar":"yaml"}`,
		`{"comparisons":[{"patch":"first","patch":"second"}]}`,
		`{"grammar":"rust"} {"grammar":"yaml"}`,
	} {
		var out scannerBuild
		if err := decodeScannerJSON([]byte(invalid), &out, true); err == nil {
			t.Fatalf("accepted invalid manifest %s", invalid)
		}
	}
	if _, err := sortedScannerInputs([]input{{"corpus/duplicate", nil}, {"corpus/duplicate", nil}}); err == nil {
		t.Fatal("accepted duplicate input ID")
	}
	corpus := t.TempDir()
	scannerTestWrite(t, filepath.Join(corpus, "test.in"), []byte("é\r\n"))
	conformance := filepath.Join("..", "..", "conformance")
	for _, lang := range []string{"rust", "javascript", "bash", "ruby", "postgres", "yaml", "python"} {
		t.Run(lang, func(t *testing.T) {
			inputs, err := scannerInputs(lang, corpus, filepath.Join(conformance, "fuzz_test.go"), filepath.Join(conformance, "testdata"))
			if err != nil {
				t.Fatal(err)
			}
			seen := map[string]int{}
			for _, in := range inputs {
				prefix, _, _ := strings.Cut(in.ID, "/")
				seen[prefix]++
			}
			if seen["corpus"] != 1 || !bytes.Equal(inputs[0].Bytes, []byte("é\r\n")) || seen["fuzz"] == 0 || seen["regression"] == 0 ||
				((lang == "bash" || lang == "ruby") && seen["heredoc"] == 0) {
				t.Fatalf("unexpected input sources or corpus bytes: %v, %q", seen, inputs[0].Bytes)
			}
		})
	}
}

func TestScannerNoPatches(t *testing.T) {
	dir := t.TempDir()
	build, expected := filepath.Join(dir, "build.json"), filepath.Join(dir, "expectations.json")
	scannerTestJSON(t, build, scannerBuild{
		Grammar: "json", Platform: runtime.GOOS + "/" + runtime.GOARCH,
		Comparisons: []scannerPair{},
	})
	if err := runScanner(build, expected, "", "", "", "", filepath.Join(dir, "empty-out")); err != nil {
		t.Fatal(err)
	}
	scannerTestWrite(t, expected, []byte("obsolete"))
	out := filepath.Join(dir, "stale-out")
	if err := runScanner(build, expected, "", "", "", "", out); err == nil || !strings.Contains(err.Error(), "stale scanner expectations") {
		t.Fatalf("stale check error = %v", err)
	}
	var report scannerReport
	if err := readScannerJSON(filepath.Join(out, "report.json"), &report); err != nil || report.Pass || len(report.Failures) != 1 {
		t.Fatalf("failed report = %+v, %v", report, err)
	}
}

func TestScannerCandidateAndCheck(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TSG_PATCHCOMPARE_TEST_DUMP", "tree")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	exeData, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "after"+filepath.Ext(self))
	if err := os.WriteFile(other, exeData, 0755); err != nil {
		t.Fatal(err)
	}
	exeSHA := sha(exeData)
	build := filepath.Join(dir, "build.json")
	scannerTestJSON(t, build, scannerBuild{
		Grammar: "rust", Identity: json.RawMessage(`{"ref":"fixture"}`), Platform: runtime.GOOS + "/" + runtime.GOARCH, Compiler: "fixture",
		Comparisons: []scannerPair{{"patches/rust/patch.patch", self, other, exeSHA, exeSHA, sha([]byte("base")), sha([]byte("patched"))}},
	})
	corpus := filepath.Join(dir, "corpus")
	source := filepath.Join(dir, "fuzz_test.go")
	scannerTestWrite(t, filepath.Join(corpus, "test.in"), []byte("corpus"))
	scannerTestWrite(t, source, []byte("package example\nfunc FuzzParseRust() { fuzzParse(f, \"rust\", \"\") }"))
	regressions := filepath.Join(dir, "regressions")
	oddName := "regression/quoted '<'"
	scannerTestWrite(t, filepath.Join(regressions, "grammar-text-patches.json"), []byte(`{"rust":[{"name":"quoted '<'","source":"special"}]}`))
	sidecar := filepath.Join(dir, "expectations.json")
	if err := runScanner(build, "", sidecar, corpus, source, regressions, filepath.Join(dir, "candidate")); err != nil {
		t.Fatal(err)
	}
	var reviewed scannerExpectation
	if err := readScannerJSON(sidecar, &reviewed); err != nil {
		t.Fatal(err)
	}
	pair := &reviewed.Comparisons[0]
	if len(pair.Differences) != 1 || pair.Differences[0].ID != oddName || pair.Differences[0].Review != "" {
		t.Fatalf("unexpected candidate differences: %+v", pair.Differences)
	}
	pair.Reason, pair.RetireWhen, pair.Differences[0].Review = "scanner fix", "when pinned upstream contains the fix", "expected rename on special input"
	scannerTestJSON(t, sidecar, reviewed)
	out := filepath.Join(dir, "check")
	if err := runScanner(build, sidecar, "", corpus, source, regressions, out); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(scannerArtifact(out, "inputs", oddName, ".in"))
	if err != nil || string(saved) != "special" {
		t.Fatalf("saved input = %q, %v", saved, err)
	}
	if err := runScanner(build, "", sidecar, corpus, source, regressions, filepath.Join(dir, "overwrite")); err == nil || !strings.Contains(err.Error(), "candidate file must be new") {
		t.Fatalf("candidate overwrite error = %v", err)
	}
	if err := runScanner(build, filepath.Join(dir, "missing.json"), "", corpus, source, regressions, filepath.Join(dir, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing expectations error = %v", err)
	}

	t.Setenv("TSG_PATCHCOMPARE_TEST_DUMP", "ordinary-exit")
	out = filepath.Join(dir, "process-error")
	newSidecar := filepath.Join(dir, "failure-candidate.json")
	if err := runScanner(build, "", newSidecar, corpus, source, regressions, out); err == nil || !strings.Contains(err.Error(), "native comparison failures") {
		t.Fatalf("process check error = %v", err)
	}
	if _, err := os.Stat(newSidecar); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected candidate from a failed process: %v", err)
	}
	var report scannerReport
	if err := readScannerJSON(filepath.Join(out, "report.json"), &report); err != nil {
		t.Fatal(err)
	}
	row := report.Results[0][0]
	if report.Pass || row.Before.Status != "process-error" || row.After.Status != "process-error" {
		t.Fatalf("failed report = %+v", report)
	}
	for _, side := range []string{"before", "after"} {
		artifact := scannerArtifact(filepath.Join(out, "pairs", "001"), side, row.ID, ".dump")
		if _, err := os.Stat(artifact); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(artifact + ".stderr")
		if err != nil || !bytes.Contains(b, []byte("fixture failure")) {
			t.Fatalf("%s stderr = %q, %v", side, b, err)
		}
	}
}

// The baseline may crash (recorded per platform, portable row); the candidate may neither crash nor end short.
func TestScannerCrashAndShortDump(t *testing.T) {
	dir := t.TempDir()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	exeData, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	before, after := filepath.Join(dir, "before"+filepath.Ext(self)), filepath.Join(dir, "after"+filepath.Ext(self))
	for _, exe := range []string{before, after} {
		if err := os.WriteFile(exe, exeData, 0755); err != nil {
			t.Fatal(err)
		}
	}
	platform := runtime.GOOS + "/" + runtime.GOARCH
	writeBuild := func(name, afterSHA string) string {
		build := filepath.Join(dir, name)
		scannerTestJSON(t, build, scannerBuild{
			Grammar: "rust", Identity: json.RawMessage(`{"ref":"fixture"}`), Platform: platform, Compiler: "fixture",
			Comparisons: []scannerPair{{"patches/rust/patch.patch", before, after, sha(exeData), afterSHA, sha([]byte("base")), sha([]byte("patched"))}},
		})
		return build
	}
	build := writeBuild("build.json", sha(exeData))
	corpus, source, regressions := filepath.Join(dir, "corpus"), filepath.Join(dir, "fuzz_test.go"), filepath.Join(dir, "regressions")
	scannerTestWrite(t, filepath.Join(corpus, "test.in"), []byte("corpus"))
	scannerTestWrite(t, source, []byte("package example\nfunc FuzzParseRust() { fuzzParse(f, \"rust\", \"\") }"))
	scannerTestWrite(t, filepath.Join(regressions, "grammar-text-patches.json"), []byte(`{"rust":[{"name":"case","source":"special"}]}`))
	run := func(mode, buildFile, name string) (scannerExpectation, error) {
		t.Setenv("TSG_PATCHCOMPARE_TEST_DUMP", mode)
		sidecar := filepath.Join(dir, name+"-candidate.json")
		err := runScanner(buildFile, "", sidecar, corpus, source, regressions, filepath.Join(dir, name))
		var got scannerExpectation
		if err == nil {
			err = readScannerJSON(sidecar, &got)
		}
		return got, err
	}

	got, err := run("crash-before", build, "baseline-crash")
	if err != nil {
		t.Fatal(err)
	}
	rows := got.Comparisons[0].Differences
	if len(rows) < 2 {
		t.Fatalf("baseline crash rows = %+v", rows)
	}
	for _, row := range rows {
		if row.Before != (scannerOutcome{Status: "baseline-crash"}) || row.BaselineCrashes[platform] != "access-violation" || len(row.BaselineCrashes) != 1 {
			t.Fatalf("baseline crash row %q = %+v, crashes %v; want a portable status and the %s class", row.ID, row.Before, row.BaselineCrashes, platform)
		}
	}
	if _, err := run("crash-after", build, "candidate-crash"); err == nil || !strings.Contains(err.Error(), "native comparison failures") {
		t.Fatalf("candidate crash error = %v", err)
	}
	if _, err := run("short-after", build, "short"); err == nil || !strings.Contains(err.Error(), "root ends at") {
		t.Fatalf("short candidate dump error = %v", err)
	}
	if _, err := run("tree", writeBuild("tampered.json", sha([]byte("other"))), "tampered"); err == nil || !strings.Contains(err.Error(), "executable digest mismatch") {
		t.Fatalf("tampered executable digest error = %v", err)
	}
}

func TestScannerNativeProtocol(t *testing.T) {
	valid := dump(false, 3, "root")
	for _, bad := range [][]byte{
		[]byte("invalid dump"),
		bytes.Replace(valid, []byte("error=0"), []byte("error=2"), 1),
		bytes.Replace(valid, []byte("nodes=1"), []byte("nodes=2"), 1),
		valid[:len(valid)-1],
		append(bytes.Replace(valid, []byte("nodes=1"), []byte("nodes=2"), 1), []byte("invalid node row\n")...),
	} {
		if _, err := parseScannerDump(bad); err == nil {
			t.Fatalf("accepted malformed dump %q", bad)
		}
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TSG_PATCHCOMPARE_TEST_DUMP", "slow")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, _, kind, err := execScanner(ctx, self, "dump"); kind != "timeout" || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timed-out native command = %s, %v", kind, err)
	}
	if _, _, kind, err := execScanner(context.Background(), filepath.Join(t.TempDir(), "missing.exe")); kind != "start-error" || err == nil {
		t.Fatalf("missing native command = %s, %v", kind, err)
	}

	if runtime.GOOS == "windows" {
		t.Setenv("TSG_PATCHCOMPARE_TEST_DUMP", "access-violation")
		dir := t.TempDir()
		for _, before := range []bool{true, false} {
			got, err := scannerDump(self, "unused.in", filepath.Join(dir, fmt.Sprintf("%t.dump", before)), 0, before)
			want := scannerOutcome{Status: "access-violation"}
			if before {
				want = scannerOutcome{Status: "baseline-crash", Crash: "access-violation"}
			}
			if got != want || (err == nil) != before {
				t.Fatalf("access violation (baseline %t) = %+v, %v", before, got, err)
			}
		}
	} else {
		if _, _, kind, err := execScanner(context.Background(), "/bin/sh", "-c", "ulimit -c 0; kill -SEGV $$"); kind != "access-violation" || err == nil {
			t.Fatalf("SIGSEGV native command = %s, %v", kind, err)
		}
	}
}

func TestMain(m *testing.M) {
	if mode := os.Getenv("TSG_PATCHCOMPARE_TEST_DUMP"); mode != "" && len(os.Args) > 1 && os.Args[1] == "dump" {
		isAfter := strings.HasPrefix(filepath.Base(os.Args[0]), "after")
		switch {
		case mode == "crash-before" && !isAfter, mode == "crash-after" && isAfter:
			scannerTestCrash()
		case mode == "short-after" && isAfter:
			b, _ := os.ReadFile(os.Args[2])
			_, _ = os.Stdout.Write(dump(false, max(len(b)-1, 0), "short"))
			os.Exit(0)
		case strings.HasSuffix(mode, "-before"), strings.HasSuffix(mode, "-after"):
			mode = "tree"
		}
		switch mode {
		case "slow":
			time.Sleep(10 * time.Second)
		case "ordinary-exit":
			fmt.Fprintln(os.Stderr, "fixture failure")
			os.Exit(3)
		case "access-violation":
			code := uint32(0xc0000005)
			os.Exit(int(code))
		case "tree":
			b, err := os.ReadFile(os.Args[2])
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(3)
			}
			root := "original"
			if string(b) == "special" && strings.HasPrefix(filepath.Base(os.Args[0]), "after") {
				root = "fixed"
			}
			_, _ = os.Stdout.Write(dump(false, len(b), root))
			os.Exit(0)
		}
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// scannerTestCrash ends the process the way a crashing native scanner does.
func scannerTestCrash() {
	if runtime.GOOS == "windows" {
		os.Exit(int(uint32(0xc0000005))) // STATUS_ACCESS_VIOLATION
	}
	// The Go runtime ignores a SIGSEGV sent with kill, so a shell takes over this process and receives it.
	err := syscall.Exec("/bin/sh", []string{"sh", "-c", "ulimit -c 0; kill -SEGV $$"}, os.Environ())
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func scannerTestWrite(t *testing.T, p string, data []byte) {
	t.Helper()
	if err := write(p, data); err != nil {
		t.Fatal(err)
	}
}

func scannerTestJSON(t *testing.T, p string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	scannerTestWrite(t, p, data)
}
