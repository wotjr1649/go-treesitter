package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/wotjr1649/go-treesitter/tools/bench/proc"
	"github.com/wotjr1649/go-treesitter/tools/compare/harness"
)

// nocc builds every adapter as a released consumer with CGO_ENABLED=0 and no C compiler reachable (the build's PATH holds
// only the Go toolchain's bin directory), then parses a smoke fixture with each executable and judges the JSON answer
// and the exit status. Outcomes: ok | expected-unsupported (a cgo adapter whose go build says its files need cgo) |
// build-failure | smoke-failure. The gate (gate-nocc) fails unless the product is ok.
//
// The job runs on a hosted VM image that has a C compiler installed elsewhere; the claim is that CGO_ENABLED=0 builds
// need none and that none is on the build's PATH, not that the image has none.
var nocgoMessages = []string{"build constraints exclude all Go files", "requires cgo", "cgo: C compiler", "C source files not allowed when not using cgo"}

var noccAdapters = append([]string{"empty"}, adapterOrder...)

type noccOpts struct {
	adapters, gobin, modcache, out string
}

func cmdNocc(args []string) error {
	fs := flag.NewFlagSet("nocc", flag.ExitOnError)
	var o noccOpts
	fs.StringVar(&o.adapters, "adapters", "", "tools/compare/adapters")
	fs.StringVar(&o.gobin, "go-bin", "", "directory of the go binary (the build's whole PATH)")
	fs.StringVar(&o.modcache, "modcache", "", "staged module cache (offline)")
	fs.StringVar(&o.out, "out", "", "builds.csv to write")
	fs.Parse(args)
	if o.adapters == "" || o.gobin == "" || o.modcache == "" || o.out == "" {
		return errors.New("nocc needs -adapters, -go-bin, -modcache and -out")
	}
	return nocc(o)
}

func nocc(o noccOpts) error {
	for _, c := range []string{"gcc", "cc", "c99", "clang", "tcc", "ld"} {
		for _, n := range []string{c, c + ".exe"} {
			if _, err := os.Stat(filepath.Join(o.gobin, n)); err == nil {
				return fmt.Errorf("%s is in the Go bin directory", n)
			}
		}
	}
	work, err := os.MkdirTemp("", "compare-nocc-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	fixture := filepath.Join(work, "fixture.c")
	if err := os.WriteFile(fixture, []byte("int main(void) { return 0; }\n"), 0o644); err != nil {
		return err
	}
	env := []string{"PATH=" + o.gobin, "CGO_ENABLED=0", "GOFLAGS=-mod=readonly", "GOPROXY=off", "GOSUMDB=off", "GOENV=off",
		"GOWORK=off", "GOTOOLCHAIN=local", "GOMODCACHE=" + o.modcache, "GOCACHE=" + filepath.Join(work, "gocache"), "HOME=" + work, "TMPDIR=" + work, "LC_ALL=C"}
	sb := &sandbox{}
	defer sb.close()
	rows := [][]string{{"adapter", "mode", "status", "exit", "bytes", "delta_vs_empty_bytes", "seconds", "smoke_parse", "outcome", "diagnostics"}}
	var empty int64
	for _, a := range noccAdapters {
		bin := filepath.Join(work, "bin-"+a+exeExt())
		t0 := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		cmd := proc.CommandContext(ctx, filepath.Join(o.gobin, "go"), "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w", "-o", bin, ".")
		cmd.Dir, cmd.Env = filepath.Join(o.adapters, a), env
		lb := &limited{max: 64 << 10}
		cmd.Stdout, cmd.Stderr = lb, lb
		berr := cmd.Run()
		cancel()
		exit := 0
		if berr != nil {
			exit = 1
			var ee interface{ ExitCode() int }
			if errors.As(berr, &ee) && ee.ExitCode() > 0 {
				exit = ee.ExitCode()
			}
		}
		var size int64
		if st, err := os.Lstat(bin); err == nil && st.Mode().IsRegular() && berr == nil {
			size = st.Size()
		}
		diag := strings.NewReplacer(",", "_", "\"", "_").Replace(printable(lb.buf.String(), 300))
		smoke, outcome := "n/a", "ok"
		switch {
		case berr != nil || size == 0:
			outcome = "build-failure"
			if (a == "official" || a == "smacker") && containsAny(lb.buf.String(), nocgoMessages) {
				outcome = "expected-unsupported"
			}
		case a == "empty":
			empty = size
		default:
			smoke = smokeParse(sb, a, bin, fixture)
			if smoke != "ok" {
				outcome = "smoke-failure"
			}
		}
		delta := int64(0)
		if a != "empty" && size > 0 && empty > 0 {
			delta = size - empty
		}
		status := "ok"
		if berr != nil || size == 0 {
			status = "failed"
		}
		rows = append(rows, []string{a, "no-c-compiler CGO_ENABLED=0", status, fmt.Sprint(exit), fmt.Sprint(size), fmt.Sprint(delta),
			fmt.Sprint(int(time.Since(t0).Seconds())), smoke, outcome, diag})
	}
	var buf bytes.Buffer
	if err := csv.NewWriter(&buf).WriteAll(rows); err != nil {
		return err
	}
	return writeFileAll(o.out, buf.Bytes())
}

func exeExt() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func containsAny(s string, subs []string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

// smokeParse runs the adapter once and accepts only a clean exit and a result line that describes exactly this call.
func smokeParse(sb *sandbox, a, bin, fixture string) string {
	sp := sb.spawn(a, bin, []string{"-lang", "c", "-op", "parse", "-mode", "time", "-src", fixture, "-reps", "1"}, 120*time.Second, 64<<10)
	if sp.Failed || sp.Truncated || sp.TimedOut || sp.Exit != 0 || sp.Err != nil {
		return "exit-failure"
	}
	var res harness.Result
	line, _, _ := bytes.Cut(sp.Stdout, []byte("\n"))
	if err := json.Unmarshal(line, &res); err != nil || res.Schema != harness.Schema {
		return "bad-json"
	}
	if res.Adapter != a || res.Lang != "c" || res.Op != "parse" || res.Mode != "time" || res.Reps != 1 || len(res.SamplesNs) != 1 || res.Status != "ok" {
		return "bad-result:" + printable(res.Status, 20)
	}
	return "ok"
}
