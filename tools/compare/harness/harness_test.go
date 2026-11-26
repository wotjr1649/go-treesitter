package harness

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMakeEditPoints(t *testing.T) {
	old := []byte("ab\ncd ef\n")
	nw := []byte("ab\ncd\nef\n") // the space becomes a newline: equal length, different points
	e, err := MakeEdit(old, nw, 5, 6, 6)
	if err != nil {
		t.Fatal(err)
	}
	if e.StartRow != 1 || e.StartCol != 2 || e.OldEndRow != 1 || e.OldEndCol != 3 || e.NewEndRow != 2 || e.NewEndCol != 0 {
		t.Fatalf("points %+v", e)
	}
	for _, bad := range [][3]uint32{{6, 5, 5}, {5, 99, 6}, {5, 6, 7}} {
		if _, err := MakeEdit(old, nw, bad[0], bad[1], bad[2]); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
}

func TestCompareLevels(t *testing.T) {
	ref := "(program (function name: (identifier) body: (block)))"
	for _, c := range []struct{ cand, level string }{
		{ref, LevelExact},
		{"(program\n  (function\n    name: (identifier) body: (block)))", LevelTree},
		{"(program (function (identifier) (block)))", LevelShape},
		{"(program (function name: (identifier) body: (ERROR)))", LevelMismatch},
		{"(program", LevelUnparsed},
	} {
		if got := Compare(ref, c.cand); got.Level != c.level {
			t.Errorf("%q: %s, want %s (%+v)", c.cand, got.Level, c.level, got)
		}
	}
	d := Compare(ref, "(program (function name: (identifier)))")
	if d.Level != LevelMismatch || d.Missing != 1 || d.Extra != 0 || !strings.Contains(d.FirstDiff, "ends") {
		t.Fatalf("%+v", d)
	}
}

// Same kinds in the same preorder but different parents must never grade as a match (review R8 P1-6).
func TestCompareDistinguishesParents(t *testing.T) {
	d := Compare("(root (a (b)) (c))", "(root (a) (b) (c))")
	if d.Level != LevelMismatch {
		t.Fatalf("different parents graded %q", d.Level)
	}
	if d := Compare("(root (a (b)) (c))", "(root (a (b)) (c))"); d.Level != LevelExact {
		t.Fatal(d)
	}
	if d := Compare("(root (a (b)) (c))", "(root\n (a\n (b))\n (c))"); d.Level != LevelTree {
		t.Fatal(d)
	}
}

// UNEXPECTED token text is discarded only under the separately labelled lossy level.
func TestCompareLossyUnexpected(t *testing.T) {
	d := Compare("(root (ERROR (UNEXPECTED 'x')))", "(root (ERROR (UNEXPECTED 'y')))")
	if d.Level != LevelLossy {
		t.Fatalf("%+v", d)
	}
	if d := Compare("(root (ERROR (UNEXPECTED 'x')))", "(root (ERROR))"); d.Level == LevelTree || d.Level == LevelExact {
		t.Fatalf("%+v", d)
	}
}

func TestSexpMarksAndEscapes(t *testing.T) {
	s := "(source (ERROR (UNEXPECTED '\\'')) (call function: (id) arguments: (args (MISSING \")\\\"\"))))"
	n, err := ParseSexp(s)
	if err != nil {
		t.Fatal(err)
	}
	if !n.Kids[0].Err || n.Kids[0].Kind != "ERROR" || !n.Kids[1].Kids[1].Kids[0].Missing {
		t.Fatalf("%+v", n.Kids)
	}
	if got := FormatSexp(&SNode{Kind: "a", Kids: []*SNode{{Field: "f", Kind: "b"}, {Kind: "m", Missing: true}}}); got != "(a f: (b) (MISSING m))" {
		t.Fatal(got)
	}
}

type fakeSess struct {
	Base
	live, edits int
	failParse   bool
}

func (f *fakeSess) Identity() map[string]string { return map[string]string{"lib": "fake"} }
func (f *fakeSess) Parse(src []byte) (Tree, error) {
	if f.failParse {
		return nil, errors.New("boom")
	}
	f.live++
	return string(src), nil
}
func (f *fakeSess) Edit(Tree, Edit) error                    { f.edits++; return nil }
func (f *fakeSess) Reparse(src []byte, _ Tree) (Tree, error) { f.live++; return string(src), nil }
func (f *fakeSess) Release(Tree)                             { f.live-- }
func (f *fakeSess) Sexp(t Tree) (string, bool, error)        { return "(" + t.(string) + ")", false, nil }

func runFake(t *testing.T, sess *fakeSess, args ...string) Result {
	t.Helper()
	var out bytes.Buffer
	open := func(lang string) (Session, error) {
		if lang == "nope" {
			return nil, ErrUnsupported
		}
		return sess, nil
	}
	if rc := Run("fake", open, args, &out); rc != 0 {
		t.Fatalf("rc %d", rc)
	}
	var r Result
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	return r
}

func TestWorkerOps(t *testing.T) {
	dir := t.TempDir()
	src, nw := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	os.WriteFile(src, []byte("xyz abc"), 0o644)
	os.WriteFile(nw, []byte("xyz abd"), 0o644)

	s := &fakeSess{}
	r := runFake(t, s, "-lang", "c", "-op", "parse", "-mode", "time", "-src", src, "-reps", "3")
	if r.Status != "ok" || len(r.SamplesNs) != 3 || s.live != 0 || r.Bytes != 7 {
		t.Fatalf("%+v live=%d", r, s.live)
	}
	s = &fakeSess{}
	r = runFake(t, s, "-lang", "c", "-op", "edit", "-mode", "time", "-src", src, "-new", nw, "-edit", "6,7,7", "-reps", "4")
	if r.Status != "ok" || len(r.SamplesNs) != 4 || s.edits != 5 || s.live != 0 { // warm-up + 4
		t.Fatalf("%+v edits=%d live=%d", r, s.edits, s.live)
	}
	r = runFake(t, &fakeSess{}, "-lang", "c", "-op", "parse", "-mode", "alloc", "-src", src, "-reps", "2")
	if r.Status != "ok" || len(r.SamplesNs) != 0 || r.Mallocs == 0 {
		t.Fatalf("alloc %+v", r)
	}
	out := filepath.Join(dir, "o")
	r = runFake(t, &fakeSess{}, "-lang", "c", "-op", "esexp", "-mode", "out", "-src", src, "-new", nw, "-edit", "6,7,7", "-out", out)
	if b, _ := os.ReadFile(out); string(b) != "(xyz abd)" || r.OutBytes != 9 || r.OutSHA256 == "" {
		t.Fatalf("%q %+v", b, r)
	}
	if r = runFake(t, &fakeSess{}, "-lang", "nope", "-op", "parse", "-src", src); r.Status != "unsupported" {
		t.Fatalf("%+v", r)
	}
	if r = runFake(t, &fakeSess{failParse: true}, "-lang", "c", "-op", "parse", "-src", src); r.Status != "error" || r.Error != "boom" {
		t.Fatalf("%+v", r)
	}
	if r = runFake(t, &fakeSess{}, "-lang", "c", "-op", "snapshot", "-src", src); r.Status != "unsupported" {
		t.Fatalf("snapshot should be unsupported by Base: %+v", r)
	}
	if r = runFake(t, &fakeSess{}, "-lang", "c", "-op", "edit", "-src", src, "-new", nw, "-edit", "0,1,1"); r.Status != "error" {
		t.Fatalf("inconsistent edit accepted: %+v", r)
	}
}

type slowSess struct{ fakeSess }

func (s *slowSess) Parse(src []byte) (Tree, error) {
	time.Sleep(300 * time.Millisecond)
	return s.fakeSess.Parse(src)
}

// The watchdog exits the process, so the test re-executes itself.
func TestWatchdogReportsTimeout(t *testing.T) {
	if os.Getenv("HARNESS_WATCHDOG_CHILD") == "1" {
		OpWatchdog = 50 * time.Millisecond
		Run("slow", func(string) (Session, error) { return &slowSess{}, nil }, []string{"-lang", "c", "-op", "parse", "-src", os.Getenv("HARNESS_SRC"), "-reps", "2"}, os.Stdout)
		return
	}
	src := filepath.Join(t.TempDir(), "a")
	os.WriteFile(src, []byte("x"), 0o644)
	cmd := exec.Command(os.Args[0], "-test.run=TestWatchdogReportsTimeout")
	cmd.Env = append(os.Environ(), "HARNESS_WATCHDOG_CHILD=1", "HARNESS_SRC="+src)
	out, err := cmd.Output()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 3 {
		t.Fatalf("exit: %v", err)
	}
	var r Result
	line, _, _ := strings.Cut(string(out), "\n")
	if json.Unmarshal([]byte(line), &r) != nil || r.Status != "timeout" {
		t.Fatalf("%q", out)
	}
}
