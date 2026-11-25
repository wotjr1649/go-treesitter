package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/wotjr1649/go-treesitter/tools/bench/proc"
	"github.com/wotjr1649/go-treesitter/tools/compare/harness"
)

const refAdapter = "product"

// declared lists the operations each adapter's public API is documented to support (plan phase 0). A declared gap is
// a named "unsupported" cell; a declared capability that the worker rejects is recorded as such, never hidden.
var declared = map[string][]string{
	"product":      {"parse", "snapshot", "edit"},
	"official":     {"parse", "edit"},
	"smacker":      {"parse", "edit"},
	"gotreesitter": {"parse", "edit"},
	"bonsai":       {"snapshot"},
	"malivvan":     {"parse"},
}

var adapterOrder = []string{"product", "official", "smacker", "gotreesitter", "bonsai", "malivvan"}

// Row is one worker process: the raw Result plus the context the aggregation needs. Nothing is summarized here.
type Row struct {
	Cell     string          `json:"cell"`
	Pass     string          `json:"pass"` // time | alloc | rss | ctl | preflight
	Round    int             `json:"round"`
	Order    string          `json:"order,omitempty"` // AB: product first
	Role     string          `json:"role"`            // product | candidate
	Adapter  string          `json:"adapter"`
	Lang     string          `json:"lang"`
	Op       string          `json:"op"`
	Group    string          `json:"group,omitempty"`
	Input    string          `json:"input,omitempty"`
	Exit     int             `json:"exit"`
	TimedOut bool            `json:"timed_out,omitempty"`
	WallNs   int64           `json:"wall_ns"`
	Err      string          `json:"err,omitempty"`
	Res      *harness.Result `json:"result,omitempty"`
}

// Cell is one expected grammar x library x operation x input; its State is final when the run ends.
type Cell struct {
	Key    string `json:"key"`
	Lang   string `json:"lang"`
	Cand   string `json:"candidate"`
	Op     string `json:"op"`
	Group  string `json:"group"`
	Input  string `json:"input"`
	State  string `json:"state"` // measured | unsupported | unavailable | incomplete | failed
	Reason string `json:"reason,omitempty"`
}

type runOpts struct {
	workload, bin, out      string
	langs, adapters         []string
	smoke                   bool
	lRounds, lReps, sRounds int
	sReps, rssRuns          int
	budget                  time.Duration
	perOp                   time.Duration
	seed                    string
}

type runner struct {
	o     runOpts
	w     Workload
	rows  *os.File
	enc   *json.Encoder
	cells []Cell
	until time.Time
	over  bool
}

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	var o runOpts
	var langs, adapters string
	fs.StringVar(&o.workload, "workload", "", "workload directory (select output)")
	fs.StringVar(&o.bin, "bin", "", "directory holding one worker binary per adapter")
	fs.StringVar(&o.out, "out", "", "result directory to create")
	fs.StringVar(&langs, "langs", "c,go,javascript,json,python", "languages")
	fs.StringVar(&adapters, "adapters", strings.Join(adapterOrder, ","), "adapters (product first)")
	fs.BoolVar(&o.smoke, "smoke", false, "reduced smoke mode: one round, two reps, two small inputs and one large input per language")
	fs.IntVar(&o.lRounds, "large-rounds", 6, "paired rounds per large input")
	fs.IntVar(&o.lReps, "large-reps", 5, "timed reps per large block")
	fs.IntVar(&o.sRounds, "small-rounds", 4, "paired rounds per small input")
	fs.IntVar(&o.sReps, "small-reps", 7, "timed reps per small block")
	fs.IntVar(&o.rssRuns, "rss-runs", 3, "fresh processes per memory sample and control")
	fs.DurationVar(&o.budget, "budget", 35*time.Minute, "wall budget; later cells are recorded incomplete")
	fs.DurationVar(&o.perOp, "op-watchdog", 30*time.Second, "watchdog per operation (a block gets reps+1 of them)")
	fs.StringVar(&o.seed, "seed", "", "manifest seed (default: the workload's)")
	fs.Parse(args)
	o.langs, o.adapters = strings.Split(langs, ","), strings.Split(adapters, ",")
	if o.workload == "" || o.bin == "" || o.out == "" {
		return errors.New("run needs -workload, -bin and -out")
	}
	if o.smoke {
		o.lRounds, o.lReps, o.sRounds, o.sReps, o.rssRuns = 1, 2, 1, 2, 1
	}
	return run(o)
}

func run(o runOpts) error {
	r := &runner{o: o, until: time.Now().Add(o.budget)}
	raw, err := os.ReadFile(filepath.Join(o.workload, "workload.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &r.w); err != nil || r.w.Schema != harness.Schema {
		return fmt.Errorf("workload.json: schema or syntax (%v)", err)
	}
	if o.seed == "" {
		r.o.seed = r.w.Seed
	}
	if o.adapters[0] != refAdapter {
		return errors.New("the first adapter must be " + refAdapter)
	}
	if err := os.MkdirAll(o.out, 0o755); err != nil {
		return err
	}
	if r.rows, err = os.Create(filepath.Join(o.out, "measurements.jsonl")); err != nil {
		return err
	}
	defer r.rows.Close()
	r.enc = json.NewEncoder(r.rows)
	start := time.Now()

	ident := map[string]map[string]map[string]string{} // adapter -> lang -> identity
	ready := map[string]map[string]string{}            // adapter -> lang -> "" ready, or the unavailability reason
	for _, a := range o.adapters {
		ident[a], ready[a] = map[string]map[string]string{}, map[string]string{}
		for _, lang := range o.langs {
			row := r.exec(Row{Pass: "preflight", Cell: a + "/" + lang, Role: roleOf(a), Adapter: a, Lang: lang, Op: "ctl"}, []string{"-lang", lang, "-op", "ctl", "-mode", "rss"}, 1)
			switch {
			case row.Res == nil:
				ready[a][lang] = "worker failed: " + row.Err
			case row.Res.Status != "ok":
				ready[a][lang] = row.Res.Status + ": " + row.Res.Error
			default:
				ident[a][lang] = row.Res.Identity
			}
		}
	}

	for _, lang := range o.langs {
		for _, cand := range o.adapters[1:] {
			for _, op := range []string{"parse", "snapshot", "edit"} {
				r.planCells(lang, cand, op, ready)
			}
		}
	}
	for _, pass := range []string{"time", "alloc"} {
		r.pairs(pass)
	}
	r.memory()
	r.finish(ident, ready, start)
	return nil
}

func roleOf(a string) string {
	if a == refAdapter {
		return "product"
	}
	return "candidate"
}

func has(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// inputsFor selects the inputs of a language: all of them, or in smoke mode two small and the smallest large one.
func (r *runner) inputsFor(lang string) (small, large []Input) {
	for _, in := range r.w.Inputs {
		if in.Lang != lang {
			continue
		}
		if in.Group == "small" {
			small = append(small, in)
		} else {
			large = append(large, in)
		}
	}
	if r.o.smoke {
		small = small[:min(2, len(small))]
		if len(large) > 1 {
			best := large[0]
			for _, in := range large {
				if in.Bytes < best.Bytes {
					best = in
				}
			}
			large = []Input{best}
		}
	}
	return
}

func (r *runner) planCells(lang, cand, op string, ready map[string]map[string]string) {
	small, large := r.inputsFor(lang)
	for _, in := range append(small, large...) {
		c := Cell{Key: strings.Join([]string{lang, cand, op, in.ID}, "|"), Lang: lang, Cand: cand, Op: op, Group: in.Group, Input: in.ID, State: "planned"}
		switch {
		case ready[refAdapter][lang] != "":
			c.State, c.Reason = "unavailable", "product: "+ready[refAdapter][lang]
		case ready[cand][lang] != "":
			c.State, c.Reason = "unsupported", ready[cand][lang]
		case !has(declared[cand], op):
			c.State, c.Reason = "unsupported", "public API has no such operation (declared)"
		case !has(declared[refAdapter], op):
			c.State, c.Reason = "unsupported", "reference side has no such operation"
		case op == "edit" && in.Token == nil:
			c.State, c.Reason = "unavailable", "no applicable single-line equal-length token edit in this input"
		}
		r.cells = append(r.cells, c)
	}
}

func (r *runner) args(pass, op string, in Input, mode string, reps int) []string {
	a := []string{"-lang", in.Lang, "-op", op, "-mode", mode, "-reps", strconv.Itoa(reps), "-src", filepath.Join(r.o.workload, filepath.FromSlash(in.Path))}
	if op == "edit" {
		a = append(a, "-new", filepath.Join(r.o.workload, filepath.FromSlash(in.Token.NewPath)),
			"-edit", fmt.Sprintf("%d,%d,%d", in.Token.Start, in.Token.OldEnd, in.Token.NewEnd))
	}
	return a
}

// firstIsProduct fixes the AB/BA order of round 0 from the manifest seed and the cell, then alternates.
func (r *runner) firstIsProduct(key string, round int) bool {
	h := sha256.Sum256([]byte(r.o.seed + "|" + key))
	return (binary.BigEndian.Uint64(h[:8])+uint64(round))%2 == 0
}

// pairs runs the paired time (or allocation) blocks: per round, the product and the candidate one after the other.
func (r *runner) pairs(pass string) {
	for _, group := range []string{"large", "small"} {
		rounds, reps := r.o.lRounds, r.o.lReps
		if group == "small" {
			rounds, reps = r.o.sRounds, r.o.sReps
		}
		for round := 0; round < rounds; round++ {
			for i := range r.cells {
				c := &r.cells[i]
				if c.Group != group || c.State != "planned" && c.State != "measured" {
					continue
				}
				if time.Now().After(r.until) {
					r.over = true
				}
				if r.over {
					if c.State == "planned" {
						c.State, c.Reason = "incomplete", "wall budget exhausted before this cell ran"
					}
					continue
				}
				in := r.input(c.Input)
				order := "AB"
				first, second := refAdapter, c.Cand
				if !r.firstIsProduct(c.Key, round) {
					order, first, second = "BA", c.Cand, refAdapter
				}
				ok := true
				for _, ad := range []string{first, second} {
					row := Row{Cell: c.Key, Pass: pass, Round: round, Order: order, Role: roleOf(ad), Adapter: ad, Lang: c.Lang, Op: c.Op, Group: group, Input: c.Input}
					row = r.exec(row, r.args(pass, c.Op, in, pass, reps), reps)
					if row.Res == nil || row.Res.Status != "ok" {
						ok = false
						if row.Res != nil && row.Res.Status == "unsupported" && ad == c.Cand {
							c.State, c.Reason = "unsupported", row.Res.Error
						}
					}
				}
				switch {
				case c.State == "unsupported":
				case ok && c.State == "planned":
					c.State = "measured"
				case !ok && c.State != "failed":
					c.State, c.Reason = "failed", fmt.Sprintf("a worker did not finish cleanly in round %d, pass %s", round, pass)
				}
			}
		}
	}
}

func (r *runner) input(id string) Input {
	for _, in := range r.w.Inputs {
		if in.ID == id {
			return in
		}
	}
	panic("unknown input " + id)
}

// memory runs the peak-RSS samples (large inputs only): fresh processes for the operation and the no-parse control.
func (r *runner) memory() {
	for i := range r.cells {
		c := &r.cells[i]
		if c.Group != "large" || c.State != "measured" || time.Now().After(r.until) {
			continue
		}
		in := r.input(c.Input)
		for k := 0; k < r.o.rssRuns; k++ {
			for _, ad := range []string{refAdapter, c.Cand} {
				r.exec(Row{Cell: c.Key, Pass: "rss", Round: k, Role: roleOf(ad), Adapter: ad, Lang: c.Lang, Op: c.Op, Group: "large", Input: c.Input}, r.args("rss", c.Op, in, "rss", 1), 1)
				ctl := r.args("rss", "ctl", in, "rss", 1)
				if c.Op == "edit" { // the control loads the same two files
					ctl = append(ctl, "-new", filepath.Join(r.o.workload, filepath.FromSlash(in.Token.NewPath)), "-edit", fmt.Sprintf("%d,%d,%d", in.Token.Start, in.Token.OldEnd, in.Token.NewEnd))
				}
				r.exec(Row{Cell: c.Key, Pass: "ctl", Round: k, Role: roleOf(ad), Adapter: ad, Lang: c.Lang, Op: c.Op, Group: "large", Input: c.Input}, ctl, 1)
			}
		}
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

// exec runs one worker with a clean environment and a watchdog, records the Row and returns it.
func (r *runner) exec(row Row, args []string, reps int) Row {
	bin := filepath.Join(r.o.bin, row.Adapter)
	timeout := time.Duration(reps+1)*r.o.perOp + r.o.perOp
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := proc.CommandContext(ctx, bin, args...)
	cmd.Env = []string{"GOMAXPROCS=1", "GOGC=100", "GOENV=off", "LC_ALL=C", "TZ=UTC"}
	var out, errb = &limited{max: 64 << 10}, &limited{max: 4 << 10}
	cmd.Stdout, cmd.Stderr = out, errb
	t0 := time.Now()
	err := cmd.Run()
	row.WallNs = time.Since(t0).Nanoseconds()
	if err != nil {
		row.Err = err.Error()
		var ee interface{ ExitCode() int }
		if errors.As(err, &ee) {
			row.Exit = ee.ExitCode()
		}
		row.TimedOut = ctx.Err() != nil
	}
	if s := strings.TrimSpace(errb.buf.String()); s != "" {
		row.Err += " | stderr: " + s[:min(len(s), 512)]
	}
	var res harness.Result
	if jerr := json.Unmarshal(out.buf.Bytes(), &res); jerr == nil && res.Schema == harness.Schema {
		row.Res = &res
	} else if row.Err == "" {
		row.Err = "no result line on stdout"
	}
	_ = r.enc.Encode(row)
	return row
}

func (r *runner) finish(ident map[string]map[string]map[string]string, ready map[string]map[string]string, start time.Time) {
	must(writeJSON(filepath.Join(r.o.out, "cells.json"), r.cells))
	counts := map[string]int{}
	for _, c := range r.cells {
		counts[c.State]++
	}
	host, _ := os.Hostname()
	_ = host // the host name is not recorded: it identifies the runner, not the experiment
	must(writeJSON(filepath.Join(r.o.out, "run.json"), map[string]any{
		"schema": harness.Schema, "seed": r.o.seed, "smoke": r.o.smoke,
		"rounds": map[string]int{"large": r.o.lRounds, "small": r.o.sRounds},
		"reps":   map[string]int{"large": r.o.lReps, "small": r.o.sReps}, "rss_runs": r.o.rssRuns,
		"budget_seconds": r.o.budget.Seconds(), "op_watchdog_seconds": r.o.perOp.Seconds(),
		"started": start.UTC().Format(time.RFC3339), "ended": time.Now().UTC().Format(time.RFC3339),
		"budget_exhausted": r.over, "cell_states": counts,
		"identity": ident, "unavailable": ready,
		"env":  map[string]string{"GOMAXPROCS": "1", "GOGC": "100", "LC_ALL": "C"},
		"host": hostInfo(), "go": runtime.Version(),
	}))
}

// hostInfo records the CPU model and kernel (Linux); nothing that names the machine or its owner.
func hostInfo() map[string]string {
	h := map[string]string{"goos": runtime.GOOS, "goarch": runtime.GOARCH, "cpus": strconv.Itoa(runtime.NumCPU())}
	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if k, v, ok := strings.Cut(l, ":"); ok && (strings.TrimSpace(k) == "model name" || strings.TrimSpace(k) == "Model") {
				h["cpu"] = strings.TrimSpace(v)
				break
			}
		}
	}
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		h["kernel"] = strings.TrimSpace(string(b))
	}
	return h
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "compare:", err)
		os.Exit(1)
	}
}
