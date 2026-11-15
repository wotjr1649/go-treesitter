package treesitter

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestFieldExportCancellationReachesHost(t *testing.T) {
	fl := newFakeLang(nil)
	p := newP(t, fl)
	tr := mustParse(t, p, sample)
	ctx, cancel := context.WithCancel(bg)
	defer cancel()
	f := fl.cur()
	observedStop := false
	f.nextHook = func(f *fakeModule) {
		cancel()
		deadline := time.Now().Add(5 * time.Second)
		for f.h.Stop.Load() == 0 && time.Now().Before(deadline) {
			runtime.Gosched()
		}
		observedStop = f.h.Stop.Load() != 0
	}
	snap, err := tr.Snapshot(ctx)
	wantPE(t, err, Cancelled, false)
	if snap != nil || !observedStop || f.h.Stop.Load() != 0 || f.count("export_end") != 1 {
		t.Fatalf("cancelled snapshot=%v observed host stop=%v remaining stop=%d export_end=%d", snap, observedStop, f.h.Stop.Load(), f.count("export_end"))
	}
	f.nextHook = nil
	snap, err = tr.Snapshot(bg)
	if err != nil || snap.Source() != sample || snap.Len() != 7 || len(fl.mods) != 1 {
		t.Fatalf("same tree after export cancellation: snapshot=%v err=%v instances=%d", snap, err, len(fl.mods))
	}
}

func TestFieldExportRefusedGrowWithoutTrap(t *testing.T) {
	for _, stage := range []string{"export_begin", "export_next", "export_err", "export_end"} {
		t.Run(stage, func(t *testing.T) {
			fl := newFakeLang(nil)
			p := newP(t, fl)
			tr := mustParse(t, p, sample)
			f := fl.cur()
			if stage == "export_err" {
				f.exportErrRefused = true
			} else {
				f.onCall = func(name string) {
					if name == stage {
						f.refused = true
					}
				}
			}
			snap, err := tr.Snapshot(bg)
			wantPE(t, err, ResourceLimit, true)
			if snap != nil || tr.valid() || len(fl.mods) != 2 || !f.released.Load() {
				t.Fatalf("refused grow snapshot=%v tree valid=%v instances=%d released=%v", snap, tr.valid(), len(fl.mods), f.released.Load())
			}
			if snap, err := p.ParseSnapshot(bg, []byte("next")); err != nil || snap.Root().Text() != "next" {
				t.Fatalf("recreated instance: snapshot=%v err=%v", snap, err)
			}
		})
	}
}
