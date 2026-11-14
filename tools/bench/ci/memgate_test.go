package main

import "testing"

// The five snapshot cases that failed the raw gate in bench run 43577876668 (macos-latest), from
// .scratch/s15/p17/run43577876668/bench-macos-latest/mem-diag/*-snapshot-default.txt: priv_* from the final line,
// rel_after = d_heap_released at stage=final (after GC + FreeOSMemory), rel_start = d_heap_released at stage=init (the
// recorded runs have no reading at main, so the first available one stands in). d_snapshot_collected=true in all five.
var recorded = []struct {
	name                                     string
	privStart, privAfter, relStart, relAfter int64
}{
	{"golang/opGen.go", 3294208, 35441868, 2719744, 35577856},
	{"golang/rewriteAMD64.go", 3277824, 29360128, 3645440, 35553280},
	{"json/generated_5mb.json", 3245056, 88814387, 9297920, 111124480},
	{"tsql/corpus_go_concat_1mb.sql", 3245056, 24641536, 7905280, 21454848},
	{"typescript/typescript.js", 3326976, 94686412, 7856128, 106897408},
}

func kvOf(ps, pa, rs, ra, coll int64) map[string]int64 {
	return map[string]int64{"priv_start": ps, "priv_after": pa, "rel_start": rs, "rel_after": ra, "snap_collected": coll}
}

func TestPrivGateRecordedDarwin(t *testing.T) {
	for _, c := range recorded {
		kv := kvOf(c.privStart, c.privAfter, c.relStart, c.relAfter, 1)
		if _, _, ok, _ := privGate("windows", "snapshot", kv); ok {
			t.Errorf("%s: the raw gate must fail (this is why the amendment exists)", c.name)
		}
		raw, adj, ok, why := privGate("darwin", "snapshot", kv)
		if !ok {
			t.Errorf("%s: adjusted gate failed: %s (raw %d adjusted %d)", c.name, why, raw, adj)
		}
		if raw != c.privAfter-c.privStart {
			t.Errorf("%s: raw %d", c.name, raw)
		}
		if adj != raw-(c.relAfter-c.relStart) {
			t.Errorf("%s: adjusted %d", c.name, adj)
		}
	}
}

func TestPrivGateSynthetic(t *testing.T) {
	const mb = 1 << 20
	for _, c := range []struct {
		name, goos, op string
		kv             map[string]int64
		want           bool
	}{
		{"released does not grow", "darwin", "snapshot", kvOf(3*mb, 53*mb, 5*mb, 5*mb, 1), false},
		{"released grows less than the delta by > 16 MB", "darwin", "snapshot", kvOf(3*mb, 53*mb, 5*mb, 30*mb, 1), false},
		{"released covers the delta", "darwin", "snapshot", kvOf(3*mb, 53*mb, 5*mb, 55*mb, 1), true},
		{"snapshot not collected", "darwin", "snapshot", kvOf(3*mb, 53*mb, 5*mb, 55*mb, 0), false},
		{"parse op needs no snap_collected", "darwin", "parse", map[string]int64{"priv_start": 3 * mb, "priv_after": 10 * mb, "rel_start": 1, "rel_after": 1}, true},
		{"parse op over 16 MB with no release", "darwin", "parse", map[string]int64{"priv_start": 3 * mb, "priv_after": 30 * mb, "rel_start": 1, "rel_after": 1}, false},
		{"missing rel_after", "darwin", "parse", map[string]int64{"priv_start": 3 * mb, "priv_after": 4 * mb, "rel_start": 1}, false},
		{"missing rel_start", "darwin", "parse", map[string]int64{"priv_start": 3 * mb, "priv_after": 4 * mb, "rel_after": 1}, false},
		{"missing snap_collected", "darwin", "snapshot", map[string]int64{"priv_start": 3 * mb, "priv_after": 4 * mb, "rel_start": 1, "rel_after": 1}, false},
		{"missing priv_after", "darwin", "parse", map[string]int64{"priv_start": 3 * mb, "rel_start": 1, "rel_after": 1}, false},
		{"linux keeps the raw gate: pass", "linux", "snapshot", map[string]int64{"priv_start": 3 * mb, "priv_after": 19 * mb}, true},
		{"linux keeps the raw gate: released growth does not help", "linux", "snapshot", kvOf(3*mb, 53*mb, 5*mb, 55*mb, 1), false},
		{"windows keeps the raw gate", "windows", "parse", map[string]int64{"priv_start": 3 * mb, "priv_after": 40 * mb}, false},
	} {
		if _, _, ok, why := privGate(c.goos, c.op, c.kv); ok != c.want {
			t.Errorf("%s: pass=%v want %v (%s)", c.name, ok, c.want, why)
		}
	}
}
