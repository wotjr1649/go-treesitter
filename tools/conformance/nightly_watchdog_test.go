package conformance

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNightlyHangWorker is a test-only scanner substitute that never polls a context.
func TestNightlyHangWorker(t *testing.T) {
	path := os.Getenv("TSG_NIGHTLY_HANG_INPUT")
	if path == "" {
		t.Skip("watchdog subprocess only")
	}
	if string(readFile(t, path)) != "hang\n" {
		t.Fatal("unexpected synthetic input")
	}
	if err := os.WriteFile(path+".started", []byte("started"), 0o600); err != nil {
		t.Fatal(err)
	}
	for {
	}
}

func TestNightlyWatchdog(t *testing.T) {
	if languages["json"] == nil {
		t.Skip("json grammar is not linked")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Setenv("TSG_CONF_OUT", root)
	for _, side := range []string{"engine", "native"} {
		t.Run(side, func(t *testing.T) {
			tmp := t.TempDir()
			input := filepath.Join(tmp, "old.bin")
			t.Setenv("TSG_NIGHTLY_HANG_INPUT", input)
			n := &nightly{t: t, lang: "json", tmp: tmp, exe: exe, limit: 5 * time.Second}
			if side == "engine" {
				n.workerTest = "TestNightlyHangWorker"
			} else {
				n.exeArgs = []string{"-test.run=^TestNightlyHangWorker$", "-test.timeout=0"}
			}
			c := kase{old: []byte("hang\n"), start: 5, oe: 5, ins: []byte("next")}
			start := time.Now()
			fs := n.check(c)
			if len(fs) != 1 || !strings.HasSuffix(fs[0].kind, side+"-timeout") {
				t.Fatalf("non-terminating %s input was not reported: %+v", side, fs)
			}
			if elapsed := time.Since(start); elapsed > 30*time.Second {
				t.Fatalf("watchdog took %s", elapsed)
			}
			if _, err := os.Stat(input + ".started"); err != nil {
				t.Fatalf("stub did not enter its non-terminating loop: %v", err)
			}
			n.lang = side
			n.report(7, 2, c, fs[0], true)
			dir := filepath.Join(root, "nightly", side+"-2")
			if !bytes.Equal(readFile(t, filepath.Join(dir, "old.bin")), c.old) || !bytes.Equal(readFile(t, filepath.Join(dir, "new.bin")), c.next()) {
				t.Fatal("timeout did not preserve the original input and edit")
			}
			if !bytes.Contains(readFile(t, filepath.Join(dir, "repro.txt")), []byte("shrunk=false")) {
				t.Fatal("timeout was shrunk")
			}
		})
	}
	// Both sides return to the supervisor, which can run the next grammar.
	if _, err := nightlyCommand(10*time.Second, exe, os.Environ(), "-test.run=^TestNightlyHangWorker$", "-test.timeout=0"); err != nil {
		t.Fatalf("next subprocess after the hang: %v", err)
	}
}

func TestNightlyEngineProcess(t *testing.T) {
	if languages["json"] == nil {
		t.Skip("json grammar is not linked")
	}
	n := &nightly{t: t, lang: "json", tmp: t.TempDir(), limit: 10 * time.Second}
	old, next := filepath.Join(n.tmp, "old.bin"), filepath.Join(n.tmp, "new.bin")
	c := kase{old: []byte("[1]"), start: 1, oe: 2, ins: []byte("2")}
	if err := os.WriteFile(old, c.old, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(next, c.next(), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := n.engine(c, old, next)
	if err != nil {
		t.Fatal(err)
	}
	if got.OldError != "" || got.NewError != "" || got.IncrementalError != "" || len(got.Old) == 0 || !bytes.Equal(got.New, got.Incremental) {
		t.Fatalf("worker lost fresh/incremental results: %+v", got)
	}
}
