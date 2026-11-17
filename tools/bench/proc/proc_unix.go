//go:build !windows

package proc

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var liveMu sync.Mutex
var live = make(map[*exec.Cmd]bool)
var signalOnce sync.Once

func stopUnix(cmd *exec.Cmd, group bool) error {
	if cmd.Process == nil {
		return nil
	}
	if cmd.Process.Pid <= 1 {
		return fmt.Errorf("invalid child pid %d", cmd.Process.Pid)
	}
	var err error
	if group {
		err = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	} else {
		err = cmd.Process.Kill()
	}
	if errors.Is(err, syscall.ESRCH) || errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

func prepare(cmd *exec.Cmd) (func() error, func() error, error) {
	signalOnce.Do(func() {
		signals := make(chan os.Signal, 1)
		// SIGQUIT keeps Go's default (stack dump); the three below terminate by default and are re-raised after cleanup.
		for _, s := range []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP} {
			if !signal.Ignored(s) { // keep a signal the parent chose to ignore ignored
				signal.Notify(signals, s)
			}
		}
		go func() {
			sig := <-signals
			liveMu.Lock()
			for command, group := range live {
				stopUnix(command, group)
			}
			// Die by the same signal so callers (shell loops) see a signal death, not an exit code.
			// Delivery is asynchronous (another thread may take it), so wait before the exit-code fallback,
			// as the runtime's own dieFromSignal does.
			signal.Reset(sig)
			syscall.Kill(os.Getpid(), sig.(syscall.Signal))
			time.Sleep(time.Second)
			os.Exit(128 + int(sig.(syscall.Signal)))
		}()
	})
	group := os.Getenv("TSG_TOOL_TREE") != "1"
	cmd.Env = append(cmd.Environ(), "TSG_TOOL_TREE=1")
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = group
	// Serialize spawn through attachment so a signal cannot miss a starting child.
	liveMu.Lock()
	live[cmd] = group
	starting := true
	stop := func() error {
		if !starting {
			liveMu.Lock()
		}
		defer liveMu.Unlock()
		starting = false
		delete(live, cmd)
		return stopUnix(cmd, group)
	}
	attach := func() error {
		starting = false
		liveMu.Unlock()
		return nil
	}
	return stop, attach, nil
}
