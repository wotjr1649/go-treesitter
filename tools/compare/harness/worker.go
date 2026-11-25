package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OpWatchdog bounds one operation (a warm-up, or one prepared-and-measured repetition): the worker prints what it has
// with status "timeout" and exits 3.
var OpWatchdog = 30 * time.Second

// Main is the whole body of an adapter's main function.
//
//	worker -lang L -op parse|snapshot|edit|sexp|ssexp|esexp|ctl -mode time|alloc|rss|out -src FILE [-new FILE -edit S,OE,NE]
//	       [-reps N] [-out FILE]
//
// time/alloc run one untimed warm-up and then Reps operations; rss keeps the trees alive and reports the peak RSS
// (the orchestrator pairs it with an op=ctl control); out writes the S-expression to -out. The only stdout is one
// Result JSON line.
func Main(adapter string, open Opener) { os.Exit(Run(adapter, open, os.Args[1:], os.Stdout)) }

// Run is Main with injectable arguments and output (the harness tests drive it with a fake engine).
func Run(adapter string, open Opener, args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet(adapter, flag.ContinueOnError)
	lang := fs.String("lang", "", "language name")
	op := fs.String("op", "parse", "parse|snapshot|edit|sexp|esexp|ctl")
	mode := fs.String("mode", "time", "time|alloc|rss|out")
	srcFile := fs.String("src", "", "source file (the old text for edit ops)")
	newFile := fs.String("new", "", "edited text (edit ops)")
	editArg := fs.String("edit", "", "start,oldEnd,newEnd byte offsets")
	reps := fs.Int("reps", 5, "timed operations")
	outFile := fs.String("out", "", "S-expression output file")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	res := &Result{Schema: Schema, Adapter: adapter, Lang: *lang, Op: *op, Mode: *mode, Src: *srcFile, Reps: *reps, Status: "ok"}
	fail := func(status string, err error) int {
		res.Status, res.Error = status, bound(err.Error())
		res.PeakRSSKiB = peakRSSKiB()
		_ = json.NewEncoder(stdout).Encode(res)
		return 0 // the orchestrator reads the status; a nonzero exit means the process itself died
	}

	t0 := time.Now()
	sess, err := open(*lang)
	if errors.Is(err, ErrUnsupported) {
		return fail("unsupported", err)
	} else if err != nil {
		return fail("error", err)
	}
	defer sess.Close()
	res.SetupNs = time.Since(t0).Nanoseconds()
	res.Identity = sess.Identity()

	var src, newSrc []byte
	var edit Edit
	if *op != "ctl" || *srcFile != "" {
		if src, err = os.ReadFile(*srcFile); err != nil {
			return fail("error", err)
		}
		res.Bytes = len(src)
	}
	isEdit := *op == "edit" || *op == "esexp" || (*op == "ctl" && *newFile != "")
	if isEdit {
		if newSrc, err = os.ReadFile(*newFile); err != nil {
			return fail("error", err)
		}
		if edit, err = parseEdit(*editArg, src, newSrc); err != nil {
			return fail("error", err)
		}
		if *op == "ctl" { // the control loads the same bytes and nothing else
			isEdit = false
		}
	}
	if *reps < 1 || *reps > 1000 {
		return fail("error", fmt.Errorf("reps %d out of range", *reps))
	}

	one := func() (Tree, error) { return sess.Parse(src) } // the operation without its timing wrapper, per op
	switch *op {
	case "snapshot", "ssexp": // ssexp: the S-expression of an owned snapshot, for libraries with no live tree
		one = func() (Tree, error) { return sess.ParseSnapshot(src) }
	case "edit", "esexp":
		one = func() (Tree, error) {
			old, err := sess.Parse(src)
			if err != nil {
				return nil, err
			}
			if err := sess.Edit(old, edit); err != nil {
				sess.Release(old)
				return nil, err
			}
			nt, err := sess.Reparse(newSrc, old)
			sess.Release(old)
			return nt, err
		}
	}

	switch {
	case *op == "ctl":
		// control: initialized exactly like the measured process, nothing parsed
	case *mode == "out":
		t, err := one()
		if err != nil {
			return fail(statusOf(err), err)
		}
		s, hasErr, err := sess.Sexp(t)
		sess.Release(t)
		if err != nil {
			return fail(statusOf(err), err)
		}
		sum := sha256.Sum256([]byte(s))
		res.HasError, res.OutSHA256, res.OutBytes = hasErr, hex.EncodeToString(sum[:]), len(s)
		if err := os.WriteFile(*outFile, []byte(s), 0o644); err != nil {
			return fail("error", err)
		}
	case *mode == "rss":
		var keep []Tree
		if isEdit { // both trees stay alive for the incremental memory sample
			old, err := sess.Parse(src)
			if err == nil {
				if err = sess.Edit(old, edit); err == nil {
					var nt Tree
					if nt, err = sess.Reparse(newSrc, old); err == nil {
						keep = append(keep, old, nt)
					}
				}
			}
			if err != nil {
				return fail(statusOf(err), err)
			}
		} else {
			t, err := one()
			if err != nil {
				return fail(statusOf(err), err)
			}
			keep = append(keep, t)
		}
		runtime.KeepAlive(keep)
	case *mode == "time" || *mode == "alloc":
		var mu sync.Mutex        // guards res between the measuring goroutine and the watchdog
		watch := func() func() { // a hung operation leaves a partial result and a distinct status, not silence
			t := time.AfterFunc(OpWatchdog, func() {
				mu.Lock()
				res.Status, res.Error = "timeout", fmt.Sprintf("operation exceeded %s", OpWatchdog)
				res.PeakRSSKiB = peakRSSKiB()
				_ = json.NewEncoder(stdout).Encode(res)
				os.Exit(3)
			})
			return func() { t.Stop() }
		}
		if err := timed(sess, res, &mu, *op, *mode == "alloc", *reps, src, newSrc, edit, watch); err != nil {
			return fail(statusOf(err), err)
		}
	default:
		return fail("error", fmt.Errorf("unknown mode %q", *mode))
	}
	res.PeakRSSKiB = peakRSSKiB()
	if err := json.NewEncoder(stdout).Encode(res); err != nil {
		return 1
	}
	return 0
}

// timed runs the warm-up and the measured loop. The prepared old tree and every release stay outside the counters.
func timed(sess Session, res *Result, mu *sync.Mutex, op string, alloc bool, reps int, src, newSrc []byte, e Edit, watch func() func()) error {
	type step struct {
		prep func() (Tree, error) // outside the measurement
		do   func(Tree) (Tree, error)
	}
	var s step
	switch op {
	case "parse":
		s.do = func(Tree) (Tree, error) { return sess.Parse(src) }
	case "snapshot":
		s.do = func(Tree) (Tree, error) { return sess.ParseSnapshot(src) }
	case "edit":
		s.prep = func() (Tree, error) { return sess.Parse(src) }
		s.do = func(old Tree) (Tree, error) {
			if err := sess.Edit(old, e); err != nil {
				return nil, err
			}
			return sess.Reparse(newSrc, old)
		}
	default:
		return fmt.Errorf("op %q has no timed mode", op)
	}
	run := func(measure func(func() (Tree, error)) (Tree, error)) error {
		defer watch()()
		var old Tree
		if s.prep != nil {
			t, err := s.prep()
			if err != nil {
				return err
			}
			old = t
		}
		nt, err := measure(func() (Tree, error) { return s.do(old) })
		if old != nil {
			sess.Release(old)
		}
		if err == nil {
			sess.Release(nt)
		}
		return err
	}
	w0 := time.Now()
	if err := run(func(f func() (Tree, error)) (Tree, error) { return f() }); err != nil {
		return err
	}
	mu.Lock()
	res.WarmupNs = time.Since(w0).Nanoseconds()
	mu.Unlock()
	var m0, m1 runtime.MemStats
	for i := 0; i < reps; i++ {
		err := run(func(f func() (Tree, error)) (Tree, error) {
			if alloc {
				runtime.ReadMemStats(&m0)
				t, err := f()
				runtime.ReadMemStats(&m1)
				mu.Lock()
				res.Mallocs += m1.Mallocs - m0.Mallocs
				res.TotalAlloc += m1.TotalAlloc - m0.TotalAlloc
				mu.Unlock()
				return t, err
			}
			t0 := time.Now()
			t, err := f()
			d := time.Since(t0).Nanoseconds()
			mu.Lock()
			res.SamplesNs = append(res.SamplesNs, d)
			mu.Unlock()
			return t, err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func statusOf(err error) string {
	if errors.Is(err, ErrUnsupported) {
		return "unsupported"
	}
	return "error"
}

func parseEdit(arg string, old, new []byte) (Edit, error) {
	parts := strings.Split(arg, ",")
	if len(parts) != 3 {
		return Edit{}, fmt.Errorf("-edit wants start,oldEnd,newEnd, got %q", arg)
	}
	var v [3]uint32
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 32)
		if err != nil {
			return Edit{}, err
		}
		v[i] = uint32(n)
	}
	return MakeEdit(old, new, v[0], v[1], v[2])
}

func bound(s string) string {
	if len(s) > 512 {
		return s[:512] + "..."
	}
	return s
}

// peakRSSKiB reads the worker's own high-water mark: a parent's rusage would also count what the parent held at fork.
func peakRSSKiB() int64 {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, l := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(l, "VmHWM:"); ok {
			f := strings.Fields(v)
			if len(f) > 0 {
				n, _ := strconv.ParseInt(f[0], 10, 64)
				return n
			}
		}
	}
	return 0
}
