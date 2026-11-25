package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wotjr1649/go-treesitter/tools/bench/proc"
)

// sandbox separates the measurer (this process, which writes every result file) from the candidate processes.
// With UIDBase > 0 each adapter runs as its own user UIDBase+index in its own directory under WorkRoot; the measurer
// (root with only CAP_SETUID/SETGID/CHOWN/DAC_OVERRIDE/FOWNER in the container) owns the inputs, binaries and
// results, none of which a candidate user can write. With UIDBase == 0 (local tests) the credential switch is skipped.
type sandbox struct {
	UIDBase  int
	WorkRoot string
	Adapters []string
	mu       sync.Mutex
	dirs     map[string]string
}

func (s *sandbox) uid(ad string) int {
	if s.UIDBase <= 0 {
		return 0
	}
	for i, a := range s.Adapters {
		if a == ad {
			return s.UIDBase + i
		}
	}
	return s.UIDBase + len(s.Adapters) // unknown adapters still never run as the measurer
}

// dir returns the adapter's private writable directory, creating it once (owned by the adapter's user).
func (s *sandbox) dir(ad string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.dirs[ad]; ok {
		return d, nil
	}
	var d string
	var err error
	if s.WorkRoot != "" {
		d = filepath.Join(s.WorkRoot, ad, "run")
		err = os.MkdirAll(d, 0o700)
	} else {
		d, err = os.MkdirTemp("", "compare-"+ad+"-")
	}
	if err != nil {
		return "", err
	}
	if u := s.uid(ad); u > 0 {
		for _, p := range []string{d, filepath.Dir(d)} {
			if err := chown(p, u); err != nil {
				return "", err
			}
		}
	}
	if s.dirs == nil {
		s.dirs = map[string]string{}
	}
	s.dirs[ad] = d
	return d, nil
}

// clean empties the adapter's directory so one run's files never reach the next.
func (s *sandbox) clean(ad string) {
	d, err := s.dir(ad)
	if err != nil {
		return
	}
	ents, _ := os.ReadDir(d)
	for _, e := range ents {
		os.RemoveAll(filepath.Join(d, e.Name()))
	}
}

type limited struct {
	buf bytes.Buffer
	max int
}

func (l *limited) Write(p []byte) (int, error) {
	if room := l.max - l.buf.Len(); room > 0 {
		l.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

// Spawned is what one candidate process left behind, as bytes the measurer reads itself.
type Spawned struct {
	Stdout   []byte
	Stderr   string
	Exit     int
	TimedOut bool
	Wall     time.Duration
	Err      error
}

// spawn runs bin as the adapter's user in its directory with a clean environment, a watchdog and bounded output.
func (s *sandbox) spawn(ad, bin string, args []string, timeout time.Duration, maxOut int) Spawned {
	var sp Spawned
	dir, err := s.dir(ad)
	if err != nil {
		sp.Err = err
		return sp
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := proc.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = []string{"GOMAXPROCS=1", "GOGC=100", "GOENV=off", "LC_ALL=C", "TZ=UTC", "TMPDIR=" + dir, "HOME=" + dir}
	if err := setCredential(cmd.Cmd, s.uid(ad)); err != nil {
		sp.Err = err
		return sp
	}
	out, errb := &limited{max: maxOut}, &limited{max: 4 << 10}
	cmd.Stdout, cmd.Stderr = out, errb
	t0 := time.Now()
	err = cmd.Run()
	sp.Wall = time.Since(t0)
	sp.Stdout = out.buf.Bytes()
	sp.Stderr = printable(errb.buf.String(), 512)
	sp.TimedOut = ctx.Err() != nil
	if err != nil {
		sp.Err = err
		var ee interface{ ExitCode() int }
		if errors.As(err, &ee) {
			sp.Exit = ee.ExitCode()
		}
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
