package main

import "fmt"

const memGateLimit = 16 << 20

// privGate decides the private-bytes criterion of one `bench mem` run (ADR-0022 §3 and its macOS amendment).
//
//	windows, linux: pass = priv_after - priv_start <= 16 MB
//	darwin:         pass = (priv_after - priv_start) - (rel_after - rel_start) <= 16 MB, and for op=snapshot the
//	                snapshot was collected (snap_collected=1). rel_* are the runtime's heap/released bytes: the vmmap
//	                footprint keeps counting pages the runtime already returned (MADV_FREE_REUSABLE).
//
// raw is always the plain delta; adjusted equals raw off darwin. A field the gate needs but the run did not print is
// a failure on darwin (a missing number must not read as a pass). The released growth is not clamped: a shrinking
// released count makes the adjusted delta larger than the raw one.
func privGate(goos, op string, kv map[string]int64) (raw, adjusted int64, pass bool, why string) {
	for _, k := range []string{"priv_start", "priv_after"} {
		if _, ok := kv[k]; !ok {
			return 0, 0, false, "missing " + k
		}
	}
	raw = kv["priv_after"] - kv["priv_start"]
	adjusted = raw
	if goos != "darwin" {
		return raw, adjusted, raw <= memGateLimit, ""
	}
	need := []string{"rel_start", "rel_after"}
	if op == "snapshot" {
		need = append(need, "snap_collected")
	}
	for _, k := range need {
		if _, ok := kv[k]; !ok {
			return raw, adjusted, false, "missing " + k
		}
	}
	adjusted = raw - (kv["rel_after"] - kv["rel_start"])
	switch {
	case op == "snapshot" && kv["snap_collected"] != 1:
		return raw, adjusted, false, "snapshot not collected"
	case adjusted > memGateLimit:
		return raw, adjusted, false, fmt.Sprintf("adjusted delta %d > %d", adjusted, memGateLimit)
	}
	return raw, adjusted, true, ""
}
