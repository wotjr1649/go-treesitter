package proc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func helperArgs(mode, path string) []string {
	return []string{"-test.run=^TestProcessHelper$", "--", mode, path}
}

func TestProcessHelper(t *testing.T) {
	if len(os.Args) < 4 || os.Args[len(os.Args)-3] != "--" {
		return
	}
	mode, path := os.Args[len(os.Args)-2], os.Args[len(os.Args)-1]
	switch mode {
	case "grandchild":
		if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			panic(err)
		}
		time.Sleep(time.Minute)
	case "parent", "exiting-parent":
		// Raw exec here deliberately leaves containment to the outer command under test.
		child := exec.Command(os.Args[0], helperArgs("grandchild", path+".grandchild")...)
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			panic(err)
		}
		if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			panic(err)
		}
		if mode == "exiting-parent" {
			for i := 0; i < 500; i++ {
				if _, err := os.Stat(path + ".grandchild"); err == nil {
					os.Exit(0)
				}
				time.Sleep(10 * time.Millisecond)
			}
			os.Exit(3)
		}
		time.Sleep(time.Minute)
	case "nested-parent":
		child := Command(os.Args[0], helperArgs("parent", path+".inner")...)
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			panic(err)
		}
		if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			panic(err)
		}
		if err := child.Wait(); err != nil {
			panic(err)
		}
	case "output":
		fmt.Fprint(os.Stdout, "stdout")
		fmt.Fprint(os.Stderr, "stderr")
	case "failure":
		fmt.Fprint(os.Stderr, "failure")
		os.Exit(7)
	default:
		panic(mode)
	}
	os.Exit(0)
}

func TestProcessNestedTree(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "outer.pid")
	cmd := CommandContext(ctx, os.Args[0], helperArgs("nested-parent", path)...)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.stopTree(); cmd.Wait() }()
	pids := []int{waitPID(t, path), waitPID(t, path+".inner"), waitPID(t, path+".inner.grandchild")}
	t.Logf("nested process IDs=%v", pids)
	if err := cmd.Wait(); err == nil {
		t.Error("nested timeout succeeded")
	}
	for _, pid := range pids {
		assertGone(t, pid)
	}
}

func waitPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(string(data)); err == nil {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("child did not start: %s", path)
	return 0
}

func assertGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("process %d survived containment", pid)
	if p, err := os.FindProcess(pid); err == nil {
		p.Kill()
	}
}

func TestProcessTree(t *testing.T) {
	for _, mode := range []string{"timeout", "cancel", "exiting-parent", "inherited-pipe"} {
		t.Run(mode, func(t *testing.T) {
			timeout := 3 * time.Second
			if mode == "inherited-pipe" {
				timeout = 10 * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			path := filepath.Join(t.TempDir(), "child.pid")
			childMode := "parent"
			if mode == "exiting-parent" || mode == "inherited-pipe" {
				childMode = "exiting-parent"
			}
			cmd := CommandContext(ctx, os.Args[0], helperArgs(childMode, path)...)
			var output bytes.Buffer
			cmd.Stdout, cmd.Stderr = &output, &output
			if cmd.WaitDelay != 5*time.Second {
				t.Fatalf("WaitDelay=%s", cmd.WaitDelay)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { cmd.stopTree(); cmd.Wait() }()
			child, grandchild := waitPID(t, path), waitPID(t, path+".grandchild")
			// Preserve identities even when the test's temporary directory is removed.
			identities, _ := json.Marshal([]int{child, grandchild})
			t.Logf("mode=%s process IDs=%s", mode, identities)
			if mode == "cancel" {
				cancel()
			}
			started := time.Now()
			err := cmd.Wait()
			if mode == "inherited-pipe" && !errors.Is(err, exec.ErrWaitDelay) {
				t.Errorf("inherited-pipe Wait=%v", err)
			}
			if mode != "exiting-parent" && err == nil {
				t.Error("timeout/cancel succeeded")
			}
			if time.Since(started) > 10*time.Second {
				t.Error("Wait exceeded bound")
			}
			assertGone(t, child)
			assertGone(t, grandchild)
		})
	}
}

func TestProcessOutput(t *testing.T) {
	cmd := Command(os.Args[0], helperArgs("output", "unused")...)
	out, err := cmd.Output()
	if err != nil || string(out) != "stdout" {
		t.Fatalf("Output=%q, %v", out, err)
	}
	cmd = Command(os.Args[0], helperArgs("output", "unused")...)
	out, err = cmd.CombinedOutput()
	if err != nil || string(out) != "stdoutstderr" {
		t.Fatalf("CombinedOutput=%q, %v", out, err)
	}
	cmd = Command(os.Args[0], helperArgs("failure", "unused")...)
	_, err = cmd.Output()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 7 || string(ee.Stderr) != "failure" {
		t.Fatalf("failure=%#v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := CommandContext(ctx, os.Args[0]).Run(); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled Run=%v", err)
	}
}

func TestProcessDeadline(t *testing.T) {
	for _, duration := range []time.Duration{0, time.Minute, time.Hour} {
		t.Run(duration.String(), func(t *testing.T) {
			ctx := context.Background()
			if duration != 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, duration)
				defer cancel()
			}
			started := time.Now()
			cmd := CommandContext(ctx, os.Args[0], helperArgs("output", "unused")...)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer cmd.Wait()
			got, ok := cmd.ctx.Deadline()
			if want, explicit := ctx.Deadline(); explicit {
				if !ok || !got.Equal(want) {
					t.Errorf("deadline=%v, want caller deadline %v", got, want)
				}
			} else if !ok || got.Before(started.Add(MaxDuration)) || got.After(time.Now().Add(MaxDuration)) {
				t.Errorf("default deadline=%v", got)
			}
		})
	}
}

func TestProcessStartError(t *testing.T) {
	err := Command(filepath.Join(t.TempDir(), "missing-executable")).Run()
	switch err.(type) {
	case *os.PathError, *exec.Error:
	default:
		t.Fatalf("start error lost its concrete type: %T: %v", err, err)
	}
}

func TestProcessStartRetryAfterFailure(t *testing.T) {
	c := Command(filepath.Join(t.TempDir(), "missing-executable"))
	if err := c.Start(); err == nil {
		t.Fatal("first Start succeeded")
	}
	if err := c.Start(); err == nil {
		t.Fatal("second Start of a failed command succeeded")
	}
	// The failed attempts must not leave the start lock held.
	done := make(chan error, 1)
	go func() { done <- Command(os.Args[0], helperArgs("output", "")...).Run() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("a new command could not start after a failed one")
	}
}
