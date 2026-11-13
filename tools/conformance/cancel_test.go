package conformance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	treesitter "github.com/wotjr1649/go-treesitter"
)

// largeFiles lists the $TSG_S15_DIR/large inputs per language (tsx has none of its own: it reuses typescript's
// lib.dom.d.ts, which tsx parses, with errors).
func largeFiles(t *testing.T, root, lang string) []string {
	t.Helper()
	dir := lang
	if lang == "tsx" {
		dir = "typescript"
	}
	m, _ := filepath.Glob(filepath.Join(root, "large", dir, "*"))
	sort.Slice(m, func(i, j int) bool { // smallest first: cheap files finish before the huge ones
		a, _ := os.Stat(m[i])
		b, _ := os.Stat(m[j])
		return a.Size() < b.Size()
	})
	if lang == "tsx" {
		var keep []string
		for _, f := range m {
			if strings.HasSuffix(f, "lib.dom.d.ts") {
				keep = append(keep, f)
			}
		}
		m = keep
	}
	if os.Getenv("TSG_S15_CANCEL") == "quick" && len(m) > 1 {
		m = m[:1]
	}
	return m
}

func hashSnapshot(s *treesitter.Snapshot) string {
	h := sha256.New()
	WriteDump(h, s)
	return hex.EncodeToString(h.Sum(nil))
}

// smallReference returns the first corpus file of lang together with its native dump.
func smallReference(t *testing.T, root, lang string) (src, dump []byte) {
	t.Helper()
	for _, r := range readCSV(t, filepath.Join(root, "corpus", "manifest.csv")) {
		if r[1] == lang && len(readFile(t, corpusFile(t, root, lang, filepath.Base(r[0])))) > 20 {
			name := filepath.Base(r[0])
			return readFile(t, corpusFile(t, root, lang, name)), readFile(t, filepath.Join(root, "native", "dumps", lang, name+".dump"))
		}
	}
	t.Fatalf("no small reference input for %s", lang)
	return
}

type cancelSample struct {
	fraction  float64
	delay     time.Duration
	completed bool
	wall      time.Duration
	overshoot time.Duration // return time - deadline; for a stopped parse this is detection + parser cleanup
}

// TestCancelThenReparse is the required real-C regression: a parse halted by its context must leave the parser clean.
// After every cancelled parse (small and huge inputs, cancel landing at several points of the parse, which includes the
// balancing phase at its end) the same parser must return the correct tree for a DIFFERENT source, and finally the
// complete tree of the large source itself.
func TestCancelThenReparse(t *testing.T) {
	root := dataRoot(t)
	fractions := []float64{0.02, 0.1, 0.3, 0.5, 0.7, 0.8, 0.9, 0.95, 0.98, 0.995}
	if os.Getenv("TSG_S15_CANCEL") == "quick" {
		fractions = []float64{0.05, 0.5, 0.9, 0.97}
	}
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			smallSrc, smallDump := smallReference(t, root, lang)
			p := newParser(t, lang)
			requireSmall := func(when string) {
				t.Helper()
				tr, err := p.Parse(bg, smallSrc, nil)
				if err != nil {
					t.Fatalf("%s: parse of a different source failed: %v", when, err)
				}
				defer tr.Close()
				snap, err := tr.Snapshot(bg)
				if err != nil {
					t.Fatalf("%s: %v", when, err)
				}
				if snap.Root().EndByte() != uint32(len(smallSrc)) || snap.Root().Text() != string(smallSrc[snap.Root().StartByte():]) {
					t.Fatalf("%s: root does not belong to the new source (end %d, len %d)", when, snap.Root().EndByte(), len(smallSrc))
				}
				if got := Dump(snap); string(got) != string(smallDump) {
					t.Fatalf("%s: the tree is not the one of the new source: %s", when, firstDiff(got, smallDump))
				}
			}
			requireSmall("baseline")

			for _, file := range largeFiles(t, root, lang) {
				big := readFile(t, file)
				name := filepath.Base(file)
				// Baseline: the uncancelled parse (timing and expected dump hash).
				// The first parse also pays for the instance's memory growth, so take the fastest of three.
				var full time.Duration
				var tr *treesitter.Tree
				var err error
				for i := 0; i < 3; i++ {
					if tr != nil {
						tr.Close()
					}
					t0 := time.Now()
					tr, err = p.Parse(bg, big, nil)
					if d := time.Since(t0); i == 0 || d < full {
						full = d
					}
					if err != nil {
						t.Fatalf("%s: baseline parse: %v", name, err)
					}
				}
				t1 := time.Now()
				snap, err := tr.Snapshot(bg)
				export := time.Since(t1)
				if err != nil {
					t.Fatal(err)
				}
				wantHash, nodes := hashSnapshot(snap), snap.Len()
				tr.Close()
				t.Logf("%s: %d bytes, %d nodes, parse %v, export %v", name, len(big), nodes, full, export)

				var samples []cancelSample
				for _, f := range fractions {
					delay := time.Duration(float64(full) * f)
					ctx, cancel := context.WithTimeout(bg, delay)
					start := time.Now()
					tree, err := p.Parse(ctx, big, nil)
					wall := time.Since(start)
					cancel()
					s := cancelSample{fraction: f, delay: delay, wall: wall, overshoot: wall - delay}
					if err == nil {
						s.completed = true // the parse beat the deadline: a complete tree must be returned
						if tree.InputBytes() != len(big) {
							t.Fatalf("%s f=%.3f: completed tree has InputBytes %d", name, f, tree.InputBytes())
						}
						tree.Close()
					} else {
						var pe *treesitter.ParseError
						switch {
						case !errors.As(err, &pe):
							t.Fatalf("%s f=%.3f: want *ParseError, got %T %v", name, f, err, err)
						case tree != nil:
							t.Fatalf("%s f=%.3f: a tree was returned together with an error", name, f)
						case pe.Outcome != treesitter.Timeout || !errors.Is(err, treesitter.ErrTimeout) ||
							!errors.Is(err, context.DeadlineExceeded) || errors.Is(err, treesitter.ErrCancelled):
							t.Fatalf("%s f=%.3f: want Timeout, got %v", name, f, err)
						case pe.InstanceReset:
							t.Fatalf("%s f=%.3f: a cancelled parse must not reset the instance: %v", name, f, err)
						}
					}
					samples = append(samples, s)
					requireSmall(fmt.Sprintf("%s after cancel at %.3f of the parse", name, f))
				}

				// Cancellation by context.Cancel (not deadline) in the middle of the parse.
				cctx, cancelNow := context.WithCancel(bg)
				timer := time.AfterFunc(full/2, cancelNow)
				start := time.Now()
				tree, err := p.Parse(cctx, big, nil)
				wall := time.Since(start)
				timer.Stop()
				cancelNow()
				cancelled := false
				if err != nil {
					cancelled = true
					var pe *treesitter.ParseError
					if tree != nil || !errors.As(err, &pe) || pe.Outcome != treesitter.Cancelled || !errors.Is(err, treesitter.ErrCancelled) ||
						!errors.Is(err, context.Canceled) || pe.InstanceReset {
						t.Fatalf("%s: context.Cancel: %v", name, err)
					}
				} else {
					tree.Close()
				}
				requireSmall(name + " after context.Cancel")

				// The same large source parsed again must give exactly the baseline tree.
				tr, err = p.Parse(bg, big, nil)
				if err != nil {
					t.Fatalf("%s: re-parse after cancellations: %v", name, err)
				}
				snap, err = tr.Snapshot(bg)
				if err != nil {
					t.Fatal(err)
				}
				if got := hashSnapshot(snap); got != wantHash || snap.Root().EndByte() != uint32(len(big)) {
					t.Fatalf("%s: re-parse differs from the baseline tree (%s vs %s)", name, got[:12], wantHash[:12])
				}
				tr.Close()

				var stopped, done int
				var maxOver, maxLate time.Duration
				var report []string
				for _, s := range samples {
					if s.completed {
						done++
					} else {
						stopped++
						maxOver = max(maxOver, s.overshoot)
						if s.fraction >= 0.9 {
							maxLate = max(maxLate, s.overshoot)
						}
					}
					report = append(report, fmt.Sprintf("%.3f:%s/%s", s.fraction, map[bool]string{true: "done", false: "stop"}[s.completed], s.overshoot.Round(10*time.Microsecond)))
				}
				summary(t, map[string]any{"lang": lang, "file": name, "bytes": len(big), "nodes": nodes,
					"parseMs": float64(full.Microseconds()) / 1000, "exportMs": float64(export.Microseconds()) / 1000,
					"deadlineSamples": strings.Join(report, " "), "stopped": stopped, "completedBeforeDeadline": done,
					"maxReturnOvershootMs": float64(maxOver.Microseconds()) / 1000, "maxLateOvershootMs": float64(maxLate.Microseconds()) / 1000,
					"contextCancelStopped": cancelled, "contextCancelWallMs": float64(wall.Microseconds()) / 1000})
			}
		})
	}
}

// TestCancelDuringSnapshot: a cancelled export keeps the tree intact; a later Snapshot must succeed and equal an
// uninterrupted export of the same source.
func TestCancelDuringSnapshot(t *testing.T) {
	root := dataRoot(t)
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			files := largeFiles(t, root, lang)
			file := files[len(files)-1] // the biggest one
			big := readFile(t, file)
			p := newParser(t, lang)

			ref, err := p.Parse(bg, big, nil) // reference export
			if err != nil {
				t.Fatal(err)
			}
			t0 := time.Now()
			refSnap, err := ref.Snapshot(bg)
			export := time.Since(t0)
			if err != nil {
				t.Fatal(err)
			}
			wantHash := hashSnapshot(refSnap)

			tree, err := p.Parse(bg, big, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { tree.Close() }()
			var landed []string
			for _, f := range []float64{0.05, 0.3, 0.6, 0.9} {
				ctx, cancel := context.WithTimeout(bg, time.Duration(float64(export)*f))
				start := time.Now()
				snap, err := tree.Snapshot(ctx)
				wall := time.Since(start)
				cancel()
				var pe *treesitter.ParseError
				switch {
				case err == nil: // finished first: the snapshot is cached, so the next sample is a cache hit
					landed = append(landed, fmt.Sprintf("%.2f:done", f))
					if hashSnapshot(snap) != wantHash {
						t.Fatal("completed snapshot differs from the reference")
					}
				case !errors.As(err, &pe) || pe.Outcome != treesitter.Timeout || pe.InstanceReset || snap != nil:
					t.Fatalf("f=%.2f: %v", f, err)
				default:
					landed = append(landed, fmt.Sprintf("%.2f:stop/%v", f, (wall-time.Duration(float64(export)*f)).Round(10*time.Microsecond)))
					// The tree must still be exportable, and an export that was stopped must not have been cached.
					again, err := tree.Snapshot(bg)
					if err != nil {
						t.Fatalf("f=%.2f: Snapshot after a cancelled export: %v", f, err)
					}
					if hashSnapshot(again) != wantHash {
						t.Fatalf("f=%.2f: snapshot after a cancelled export differs from the reference", f)
					}
					// Re-parse to get an uncached tree for the next sample.
					tree.Close()
					if tree, err = p.Parse(bg, big, nil); err != nil {
						t.Fatal(err)
					}
				}
			}
			summary(t, map[string]any{"lang": lang, "file": filepath.Base(file), "nodes": refSnap.Len(),
				"exportMs": float64(export.Microseconds()) / 1000, "samples": strings.Join(landed, " ")})
		})
	}
}
