package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/go-treesitter/tools/gen/grammarpatch"
)

func runtimeFixture(t *testing.T) (src, archive string) {
	t.Helper()
	dir := t.TempDir()
	src = filepath.Join(dir, "src")
	tree := filepath.Join(src, "tree-sitter-fake")
	if err := os.MkdirAll(filepath.Join(tree, "lib", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"lib/src/a.c": "int a;\n", "LICENSE": "MIT\n"} {
		if err := os.WriteFile(filepath.Join(tree, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	archive = filepath.Join(dir, "fake.tar.gz")
	if err := os.WriteFile(archive, []byte("archive bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	return src, archive
}

func TestRuntimePinsCommandAndGuard(t *testing.T) {
	src, archive := runtimeFixture(t)
	out := filepath.Join(t.TempDir(), "pins.json")
	args := []string{"-root", ".", "-src", src, "-extract-dir", "tree-sitter-fake", "-archive", archive,
		"-release", "release-0.27-9a1a129b", "-commit", strings.Repeat("ab", 20), "-out", out}
	var stdout bytes.Buffer
	if err := runRuntimePinsCommand(args, &stdout); err != nil {
		t.Fatal(err)
	}
	if moved, err := checkRehearsalPins("pins.json", out); err != nil || moved {
		t.Fatalf("runtime-only copy: moved=%v err=%v", moved, err)
	}
	pins, err := loadPins(out)
	if err != nil {
		t.Fatal(err)
	}
	rt := pins.Runtime
	if rt.Release != "release-0.27-9a1a129b" || rt.Archive != "fake.tar.gz" || rt.ExtractDir != "tree-sitter-fake" ||
		rt.ArchiveSHA256 != sumBytes([]byte("archive bytes")) || rt.LicenseSHA256 != sumBytes([]byte("MIT\n")) || rt.TreeSHA256 == "" {
		t.Fatalf("runtime block not rewritten: %+v", rt)
	}
	if rt.Repo == "" || len(rt.Dirs) == 0 || pins.Tools.TreeSitterCLI == "" {
		t.Fatalf("copy lost committed fields: %+v", pins)
	}
	if err := runRuntimePinsCommand(args, &stdout); err == nil {
		t.Fatal("existing -out was overwritten")
	}
	// Any change outside the runtime block is refused.
	b, _ := os.ReadFile(out)
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, bytes.Replace(b, []byte(pins.Tools.TreeSitterCLI), []byte("9.9.9"), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := checkRehearsalPins("pins.json", bad); err == nil {
		t.Fatal("pins copy with a hand-edited CLI version accepted")
	}
	// The committed file itself is never a valid -out, and bad identities are refused.
	for _, tc := range [][]string{
		{"-out", "pins.json"},
		{"-commit", "main"},
		{"-release", "../x"},
		{"-extract-dir", "../x"},
	} {
		a := append(append([]string{}, args...), tc...) // later flags win
		if err := runRuntimePinsCommand(a, &stdout); err == nil {
			t.Fatalf("%v accepted", tc)
		}
	}
}

func TestRuntimeRehearsalOutputBoundary(t *testing.T) {
	repo := t.TempDir()
	o := opts{repo: repo, out: filepath.Join(repo, "grammars", "json"), work: filepath.Join(repo, "work")}
	if err := validateTestOnlyOutput(o, "-runtime-rehearsal"); err == nil || !strings.Contains(err.Error(), "-runtime-rehearsal") {
		t.Fatalf("product output accepted: %v", err)
	}
}

// cliArchiveFixture writes a file named like the pinned asset of this host (never executed: its bytes are not a program).
func cliArchiveFixture(t *testing.T) string {
	t.Helper()
	pins, err := loadPins("pins.json")
	if err != nil {
		t.Fatal(err)
	}
	cli, ok := pins.Tools.TreeSitterCLIArchives[hostPlatform()]
	if !ok {
		t.Skipf("no pinned CLI for %s", hostPlatform())
	}
	f := filepath.Join(t.TempDir(), path.Base(cli.URL))
	if err := os.WriteFile(f, []byte("candidate cli archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestRuntimeCLIPairPins(t *testing.T) {
	src, archive := runtimeFixture(t)
	cli := cliArchiveFixture(t)
	base := []string{"-root", ".", "-src", src, "-extract-dir", "tree-sitter-fake", "-archive", archive,
		"-release", "r", "-commit", strings.Repeat("ab", 20)}
	run := func(extra ...string) (string, bytes.Buffer, error) {
		out := filepath.Join(t.TempDir(), "pins.json")
		var stdout bytes.Buffer
		err := runRuntimePinsCommand(append(append(append([]string{}, base...), "-out", out), extra...), &stdout)
		return out, stdout, err
	}
	out, stdout, err := run("-cli-version", "0.28.0", "-cli-archive", cli)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := checkRehearsalPins("pins.json", out)
	if err != nil || !moved {
		t.Fatalf("pair copy: moved=%v err=%v", moved, err)
	}
	pins, err := loadPins(out) // the swift exception is bound to the committed CLI and is dropped, so the copy loads
	if err != nil {
		t.Fatal(err)
	}
	a := pins.Tools.TreeSitterCLIArchives
	if pins.Tools.TreeSitterCLI != "0.28.0" || len(a) != 1 || a[hostPlatform()].SHA256 != sumBytes([]byte("candidate cli archive")) ||
		!strings.HasPrefix(a[hostPlatform()].URL, cliAssetBase+"v0.28.0/") {
		t.Fatalf("CLI block not rewritten: %+v", pins.Tools)
	}
	var id struct {
		CLI     *cliIdentity `json:"cli"`
		Release string       `json:"release"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &id); err != nil || id.CLI == nil || id.Release != "r" || id.CLI.ArchiveSHA256 != a[hostPlatform()].SHA256 {
		t.Fatalf("identity %s: %v", stdout.String(), err)
	}
	// A copy that moves the CLI may differ in nothing else: edit one more pin by hand.
	b, _ := os.ReadFile(out)
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, bytes.Replace(b, []byte(pins.Tools.Wasm2go), []byte("v9.9.9"), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := checkRehearsalPins("pins.json", bad); err == nil {
		t.Fatal("pair copy with another moved pin accepted")
	}
	// Refused inputs: one flag without the other, a bad version, a wrongly named archive.
	wrongName := filepath.Join(t.TempDir(), "other.gz")
	if err := os.WriteFile(wrongName, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, extra := range [][]string{{"-cli-version", "0.28.0"}, {"-cli-archive", cli}, {"-cli-version", "v0.28.0", "-cli-archive", cli},
		{"-cli-version", "0.28.0", "-cli-archive", wrongName}, {"-cli-version", "0.28.0", "-cli-archive", cli, "-cli-platform", "plan9-amd64"}} {
		if _, _, err := run(extra...); err == nil {
			t.Fatalf("%v accepted", extra)
		}
	}
}

func TestCandidateCLIRunsOnlySandboxed(t *testing.T) {
	// gen refuses a copy that moves the CLI without -cli-sandbox, and -cli-sandbox without such a copy, before anything runs.
	src, archive := runtimeFixture(t)
	cli := cliArchiveFixture(t)
	dir := t.TempDir()
	moved, plain := filepath.Join(dir, "moved.json"), filepath.Join(dir, "plain.json")
	for out, extra := range map[string][]string{moved: {"-cli-version", "0.28.0", "-cli-archive", cli}, plain: nil} {
		args := append([]string{"-root", ".", "-src", src, "-extract-dir", "tree-sitter-fake", "-archive", archive, "-release", "r",
			"-commit", strings.Repeat("ab", 20), "-out", out}, extra...)
		if err := runRuntimePinsCommand(args, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ pins, sandbox string }{{moved, ""}, {plain, "/x/nightly-sandbox"}} {
		o := opts{root: ".", runtimeRehearsal: tc.pins, cliSandbox: tc.sandbox, lang: "json"}
		if err := run(o); err == nil || !strings.Contains(err.Error(), "-cli-sandbox") {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
	// With the wrapper but without the bytes the pins copy hashed, nothing is executed.
	c := grammarpatch.Config{CLIVersion: "0.28.0"}
	pins, _ := loadPins(moved)
	o := opts{dl: t.TempDir(), treeSitter: filepath.Join(dir, "no-such-binary"), cliSandbox: filepath.Join(dir, "never-run")}
	if err := sandboxCandidateCLI(&c, o, pins.Tools.TreeSitterCLIArchives[hostPlatform()]); err == nil || c.Unpinned {
		t.Fatalf("candidate CLI accepted without its archive: %v", err)
	}
}
