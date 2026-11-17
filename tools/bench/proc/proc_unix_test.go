//go:build !windows

package proc

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestProcessGroupOwnership(t *testing.T) {
	for _, marker := range []string{"", "1"} {
		t.Run("marker="+marker, func(t *testing.T) {
			t.Setenv("TSG_TOOL_TREE", marker)
			cmd := exec.Command("not-executed")
			cmd.Env = []string{"CUSTOM=value"}
			stop, _, err := prepare(cmd)
			if err != nil {
				t.Fatal(err)
			}
			defer stop()
			if cmd.SysProcAttr.Setpgid != (marker != "1") {
				t.Errorf("Setpgid=%v for marker %q", cmd.SysProcAttr.Setpgid, marker)
			}
			if got := cmd.Env[len(cmd.Env)-1]; got != "TSG_TOOL_TREE=1" {
				t.Errorf("child marker=%q", got)
			}
		})
	}
}

func TestProcessRejectsUnsafePID(t *testing.T) {
	for _, pid := range []int{-1, 0, 1} {
		if err := stopUnix(&exec.Cmd{Process: &os.Process{Pid: pid}}, true); err == nil {
			t.Errorf("accepted pid %d", pid)
		}
	}
}

func TestProcessSignalStopsLiveTree(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP} {
		t.Run(sig.String(), func(t *testing.T) {
			t.Setenv("TSG_TOOL_TREE", "")
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "supervisor.pid")
			// The raw supervisor receives the signal; its proc child owns the group.
			cmd := exec.CommandContext(ctx, os.Args[0], helperArgs("nested-parent", path)...)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { cmd.Process.Kill(); cmd.Wait() }()
			waitPID(t, path)
			child := waitPID(t, path+".inner")
			defer syscall.Kill(-child, syscall.SIGKILL)
			grandchild := waitPID(t, path+".inner.grandchild")
			if err := cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			err := cmd.Wait()
			var ws syscall.WaitStatus
			if ee, ok := err.(*exec.ExitError); ok {
				ws, _ = ee.Sys().(syscall.WaitStatus)
			}
			if !ws.Signaled() || ws.Signal() != sig {
				t.Errorf("supervisor ended with %v, want death by %v", err, sig)
			}
			assertGone(t, child)
			assertGone(t, grandchild)
		})
	}
}
