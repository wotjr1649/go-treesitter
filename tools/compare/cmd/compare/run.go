package main

import (
	"bytes"
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
	Key    string         `json:"key"`
	Lang   string         `json:"lang"`
	Cand   string         `json:"candidate"`
	Op     string         `json:"op"`
	Group  string         `json:"group"`
	Input  string         `json:"input"`
	State  string         `json:"state"` // measured (complete) | partial | failed | incomplete | unsupported | unavailable
	Reason string         `json:"reason,omitempty"`
	Want   int            `json:"rounds_wanted"`
	OK     map[string]int `json:"rounds_ok"` // complete paired rounds per pass (time, alloc)
	bad    bool
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
	uidBase                 int
	workRoot                string
}

// readiness is the preflight verdict of one adapter on one language.
type readiness struct{ kind, reason string } // kind: "" ready, "unsupported", "failed"

type runner struct {
	sb    *sandbox
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
	fs.IntVar(&o.uidBase, "uid-base", 0, "run adapter i as user uid-base+i (Linux sandbox; 0 keeps the measurer's user)")
	fs.StringVar(&o.workRoot, "work-root", "", "directory holding one private writable directory per adapter")
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
	r := &runner{o: o, until: time.Now().Add(o.budget), sb: &sandbox{UIDBase: o.uidBase, WorkRoot: o.workRoot, Adapters: o.adapters}}
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
	ready := map[string]map[string]readiness{}         // adapter -> lang -> verdict
	for _, a := range o.adapters {
		ident[a], ready[a] = map[string]map[string]string{}, map[string]readiness{}
		for _, lang := range o.langs {
			row := r.exec(Row{Pass: "preflight", Cell: a + "/" + lang, Role: roleOf(a), Adapter: a, Lang: lang, Op: "ctl"}, []string{"-lang", lang, "-op", "ctl", "-mode", "rss"}, 1)
			switch {
			case row.Res == nil || row.Exit != 0 || row.TimedOut:
				ready[a][lang] = readiness{"failed", "worker failed: " + row.Err}
			case row.Res.Status == "unsupported":
				ready[a][lang] = readiness{"unsupported", row.Res.Error}
			case row.Res.Status != "ok":
				ready[a][lang] = readiness{"failed", row.Res.Status + ": " + row.Res.Error}
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
	r.finalize()
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

func (r *runner) inputsFor(lang string) (small, large []Input) {
	return r.w.inputsFor(lang, r.o.smoke)
}

func (r *runner) planCells(lang, cand, op string, ready map[string]map[string]readiness) {
	small, large := r.inputsFor(lang)
	for _, in := range append(small, large...) {
		c := Cell{Key: strings.Join([]string{lang, cand, op, in.ID}, "|"), Lang: lang, Cand: cand, Op: op, Group: in.Group, Input: in.ID, State: "planned", OK: map[string]int{}, Want: r.rounds(in.Group)}
		switch {
		case ready[refAdapter][lang].kind != "":
			c.State, c.Reason = "unavailable", "product: "+ready[refAdapter][lang].reason
		case ready[cand][lang].kind == "unsupported":
			c.State, c.Reason = "unsupported", ready[cand][lang].reason
		case ready[cand][lang].kind == "failed":
			c.State, c.Reason = "failed", ready[cand][lang].reason
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

func (r *runner) rounds(group string) int {
	if group == "small" {
		return r.o.sRounds
	}
	return r.o.lRounds
}

// complete reports whether a worker row is a finished, normally exiting block with every scheduled sample.
func complete(row Row, pass string, reps int) bool {
	res := row.Res
	if res == nil || res.Status != "ok" || row.Exit != 0 || row.TimedOut || res.Reps != reps {
		return false
	}
	if pass == "time" {
		if len(res.SamplesNs) != reps {
			return false
		}
		for _, s := range res.SamplesNs {
			if s < 0 {
				return false
			}
		}
	}
	return true
}

// pairs runs the paired time (or allocation) blocks: per round, the product and the candidate one after the other.
// A cell advances only through complete rounds; a failed or short block stops that cell and keeps its raw rows.
func (r *runner) pairs(pass string) {
	for _, group := range []string{"large", "small"} {
		rounds, reps := r.o.lRounds, r.o.lReps
		if group == "small" {
			rounds, reps = r.o.sRounds, r.o.sReps
		}
		for round := 0; round < rounds; round++ {
			for i := range r.cells {
				c := &r.cells[i]
				if c.Group != group || c.State != "planned" || c.bad || c.OK[pass] != round {
					continue
				}
				if time.Now().After(r.until) {
					r.over = true
				}
				if r.over {
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
					if !complete(row, pass, reps) {
						ok = false
						if row.Res != nil && row.Res.Status == "unsupported" && ad == c.Cand {
							c.State, c.Reason = "unsupported", row.Res.Error
						}
					}
				}
				switch {
				case c.State == "unsupported":
				case ok:
					c.OK[pass]++
				default:
					c.bad, c.Reason = true, fmt.Sprintf("a worker block was not complete in round %d, pass %s", round, pass)
				}
			}
		}
	}
}

// finalize gives every planned cell its terminal state: measured only when both passes finished every scheduled round.
func (r *runner) finalize() {
	for i := range r.cells {
		c := &r.cells[i]
		if c.State != "planned" {
			continue
		}
		switch {
		case c.OK["time"] == c.Want && c.OK["alloc"] == c.Want:
			c.State = "measured"
		case c.bad && c.OK["time"]+c.OK["alloc"] > 0:
			c.State = "partial"
		case c.bad:
			c.State = "failed"
		default:
			c.State, c.Reason = "incomplete", "wall budget exhausted before every scheduled round ran"
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

// exec runs one worker as its adapter's user with a watchdog, records the Row and returns it. The measurer parses the
// result line itself; whatever text the candidate produced is reduced to bounded printable ASCII first.
func (r *runner) exec(row Row, args []string, reps int) Row {
	bin := filepath.Join(r.o.bin, row.Adapter)
	timeout := time.Duration(reps+1)*r.o.perOp + r.o.perOp
	sp := r.sb.spawn(row.Adapter, bin, args, timeout, 64<<10)
	r.sb.clean(row.Adapter)
	row.WallNs, row.Exit, row.TimedOut = sp.Wall.Nanoseconds(), sp.Exit, sp.TimedOut
	if sp.Err != nil {
		row.Err = printable(sp.Err.Error(), 300)
	}
	if sp.Stderr != "" {
		row.Err += " | stderr: " + sp.Stderr
	}
	var res harness.Result
	line, _, _ := bytes.Cut(sp.Stdout, []byte("\n"))
	if jerr := json.Unmarshal(line, &res); jerr == nil && res.Schema == harness.Schema && boundResult(&res) {
		row.Res = &res
	} else if row.Err == "" {
		row.Err = "no valid result line on stdout"
	}
	_ = r.enc.Encode(row)
	return row
}

// boundResult clamps and sanitizes the candidate-controlled fields of a Result; false rejects it.
func boundResult(res *harness.Result) bool {
	if len(res.SamplesNs) > 1000 || res.Reps < 0 || res.Reps > 1000 {
		return false
	}
	switch res.Status {
	case "ok", "unsupported", "error", "timeout":
	default:
		return false
	}
	res.Error = printable(res.Error, 400)
	res.Adapter, res.Lang, res.Op, res.Mode, res.Src = printable(res.Adapter, 40), printable(res.Lang, 40), printable(res.Op, 20), printable(res.Mode, 20), printable(res.Src, 200)
	id := map[string]string{}
	for k, v := range res.Identity {
		if len(id) < 64 {
			id[printable(k, 120)] = printable(v, 200)
		}
	}
	res.Identity = id
	res.OutSHA256 = printable(res.OutSHA256, 64)
	return true
}

func (r *runner) finish(ident map[string]map[string]map[string]string, ready map[string]map[string]readiness, start time.Time) {
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
