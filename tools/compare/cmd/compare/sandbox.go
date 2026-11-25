package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wotjr1649/go-treesitter/tools/bench/proc"
)

// Candidates and the product run as the measurer's own user in the same VM (threat model: the ephemeral runner VM is the
// isolation boundary, see README.md). This file only keeps what the measurement itself needs: a private working directory
// for the files a worker writes, watchdogs, bounded output and bounded, link-refusing file reads.
type sandbox struct {
	mu   sync.Mutex
	root string
	dirs map[string]string
}

// dir returns the adapter's working directory (created once, under a fresh temporary root).
func (s *sandbox) dir(ad string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == "" {
		r, err := os.MkdirTemp("", "compare-work-")
		if err != nil {
			return "", err
		}
		s.root = r
	}
	if d, ok := s.dirs[ad]; ok {
		return d, nil
	}
	d := filepath.Join(s.root, ad)
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	if s.dirs == nil {
		s.dirs = map[string]string{}
	}
	s.dirs[ad] = d
	return d, nil
}

// close removes the working directories.
func (s *sandbox) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root != "" {
		os.RemoveAll(s.root)
	}
}

// limited keeps the first max bytes and remembers that more was written.
type limited struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (l *limited) Write(p []byte) (int, error) {
	room := l.max - l.buf.Len()
	if room < len(p) {
		l.truncated = true
	}
	if room > 0 {
		l.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

// Spawned is what one process left behind. Failed is true when the process could not be started or reaped cleanly
// (a start error, a wait error other than a plain nonzero exit, or a watchdog kill); a Failed block is never complete.
type Spawned struct {
	Stdout    []byte
	Stderr    string
	Exit      int
	TimedOut  bool
	Failed    bool
	Truncated bool
	Wall      time.Duration
	Err       error
}

// spawn runs bin in the adapter's directory with a clean environment, a watchdog and bounded output.
func (s *sandbox) spawn(ad, bin string, args []string, timeout time.Duration, maxOut int) Spawned {
	var sp Spawned
	dir, err := s.dir(ad)
	if err != nil {
		sp.Err, sp.Failed = err, true
		return sp
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := proc.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = []string{"GOMAXPROCS=1", "GOGC=100", "GOENV=off", "LC_ALL=C", "TZ=UTC", "TMPDIR=" + dir, "HOME=" + dir}
	out, errb := &limited{max: maxOut}, &limited{max: 4 << 10}
	cmd.Stdout, cmd.Stderr = out, errb
	t0 := time.Now()
	err = cmd.Run()
	sp.Wall = time.Since(t0)
	sp.Stdout, sp.Truncated = out.buf.Bytes(), out.truncated
	sp.Stderr = printable(errb.buf.String(), 512)
	sp.TimedOut = ctx.Err() != nil
	if err != nil {
		sp.Err = err
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			sp.Exit = ee.ExitCode()
		}
		// a plain nonzero exit is a result; a signal death, start/wait/kill error or timeout is a failed spawn
		sp.Failed = ee == nil || sp.Exit <= 0
	}
	if sp.TimedOut {
		sp.Failed = true
	}
	return sp
}

// readBounded reads a regular file a candidate wrote, never following a link and never more than max bytes.
func readBounded(path string, max int64) ([]byte, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", filepath.Base(path))
	}
	if st.Size() > max {
		return nil, fmt.Errorf("%s has %d bytes, over the %d byte limit", filepath.Base(path), st.Size(), max)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err == nil && int64(len(b)) > max {
		err = errors.New("file grew past the limit")
	}
	return b, err
}

// printable keeps visible ASCII only (everything else becomes '_') and at most n bytes: text from a candidate
// never reaches a report, a log line or a file name unchanged.
func printable(s string, n int) string {
	var b strings.Builder
	for i := 0; i < len(s) && b.Len() < n; i++ {
		c := s[i]
		if c >= 0x20 && c < 0x7f {
			b.WriteByte(c)
		} else if c == '\n' || c == '\t' {
			b.WriteByte(' ')
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}
