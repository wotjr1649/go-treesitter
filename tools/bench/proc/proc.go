// Package proc runs bounded commands whose descendants are cleaned up with them.
// On POSIX, nested supervisors share the outer command's process group.
// shortcut: an inner timeout kills only its leader (with WaitDelay for pipes);
// its descendants live until the outer command ends; use stronger isolation if needed.
package proc

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"sync"
	"time"
)

const MaxDuration = 30 * time.Minute

// Cmd preserves exec.Cmd's fields; use its Run, Output or Start/Wait methods.
type Cmd struct {
	*exec.Cmd
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	watchDone chan struct{}
	stop      func() error
	stopOnce  sync.Once
	stopErr   error
	waited    bool
}

func Command(name string, args ...string) *Cmd {
	return CommandContext(context.Background(), name, args...)
}

func CommandContext(ctx context.Context, name string, args ...string) *Cmd {
	if ctx == nil {
		panic("nil Context")
	}
	cmd := exec.Command(name, args...)
	cmd.WaitDelay = 5 * time.Second
	return &Cmd{Cmd: cmd, ctx: ctx}
}

func (c *Cmd) stopTree() error {
	c.stopOnce.Do(func() { c.stopErr = c.stop() })
	return c.stopErr
}

func (c *Cmd) Start() error {
	if c.done != nil || c.stop != nil { // a failed Start already ran its stop; a retry must not take the lock again
		return errors.New("exec: already started")
	}
	if err := c.ctx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(c.ctx)
	if _, ok := c.ctx.Deadline(); !ok {
		cancel()
		ctx, cancel = context.WithTimeout(c.ctx, MaxDuration)
	}
	stop, attach, err := prepare(c.Cmd)
	if err != nil {
		cancel()
		return err
	}
	c.stop = stop
	if err = c.Cmd.Start(); err != nil {
		cancel()
		if stopErr := c.stopTree(); stopErr != nil {
			return errors.Join(err, stopErr)
		}
		return err
	}
	if err = attach(); err != nil {
		cancel()
		// A failed assignment must never release a suspended child to run uncontained.
		killErr := c.Process.Kill()
		waitErr := c.Cmd.Wait()
		return errors.Join(err, c.stopTree(), killErr, waitErr)
	}
	c.ctx, c.cancel = ctx, cancel
	c.done, c.watchDone = make(chan struct{}), make(chan struct{})
	go func() {
		defer close(c.watchDone)
		select {
		case <-ctx.Done():
			if err := c.stopTree(); err != nil {
				c.Process.Kill()
			}
		case <-c.done:
		}
	}()
	return nil
}

func (c *Cmd) Wait() error {
	if c.done == nil {
		return errors.New("exec: not started")
	}
	if c.waited {
		return errors.New("exec: Wait was already called")
	}
	c.waited = true
	err := c.Cmd.Wait()
	close(c.done)
	<-c.watchDone
	stopErr := c.stopTree()
	ctxErr := c.ctx.Err()
	c.cancel()
	if err == nil {
		err = ctxErr
	}
	if stopErr != nil {
		return errors.Join(err, stopErr)
	}
	return err
}

func (c *Cmd) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}

func (c *Cmd) Output() ([]byte, error) {
	if c.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	var stdout bytes.Buffer
	stderr := limitedStderr{}
	c.Stdout = &stdout
	captureErr := c.Stderr == nil
	if captureErr {
		c.Stderr = &stderr
	}
	err := c.Run()
	if ee, ok := err.(*exec.ExitError); ok && captureErr {
		ee.Stderr = stderr.data
	}
	return stdout.Bytes(), err
}

// Keep error diagnostics bounded, as exec.Cmd.Output does.
type limitedStderr struct{ data []byte }

func (b *limitedStderr) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := (64 << 10) - len(b.data); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}

func (c *Cmd) CombinedOutput() ([]byte, error) {
	if c.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	if c.Stderr != nil {
		return nil, errors.New("exec: Stderr already set")
	}
	var output bytes.Buffer
	c.Stdout, c.Stderr = &output, &output
	err := c.Run()
	return output.Bytes(), err
}
