package provenance_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/go-treesitter/internal/provenance"
)

func TestBundledRuntime(t *testing.T) {
	if err := provenance.VerifyRuntime("../.."); err != nil {
		t.Fatal(err)
	}
	// An ordinary executable carries dependency build information even on Go
	// versions where the test executable itself omits the dependency list.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "buildinfo.exe")
	if out, err := exec.CommandContext(ctx, "go", "build", "-o", binary, "./testdata/buildinfo").CombinedOutput(); err != nil {
		t.Fatalf("build identity probe: %v\n%s", err, out)
	}
	out, err := exec.CommandContext(ctx, binary).CombinedOutput()
	if err != nil {
		t.Fatalf("bundled runtime build information: %v\n%s", err, out)
	}
	t.Logf("%s", out)
}

func TestRuntimeRejectsTampering(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"internal/runtime", "tools/runtime-bundle", "internal/provenance"} {
		if err := os.CopyFS(filepath.Join(root, name), os.DirFS(filepath.Join("../..", name))); err != nil {
			t.Fatal(err)
		}
	}
	if err := provenance.VerifyRuntime(root); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "internal/runtime/grammars/runtime/javascript_scanner.go")
	original, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, append(append([]byte(nil), original...), '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if err := provenance.VerifyRuntime(root); err == nil || !strings.Contains(err.Error(), "file identity mismatch") {
		t.Fatalf("changed scanner accepted: %v", err)
	}
	if err := os.WriteFile(file, original, 0600); err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(root, "internal/runtime/unrecorded.go")
	if err := os.WriteFile(unknown, []byte("package gotreesitter\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := provenance.VerifyRuntime(root); err == nil || !strings.Contains(err.Error(), "unrecorded runtime file") {
		t.Fatalf("additional runtime file accepted: %v", err)
	}
	if err := os.Remove(unknown); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := provenance.VerifyRuntime(root); err == nil || !strings.Contains(err.Error(), "inventory incomplete") {
		t.Fatalf("missing scanner accepted: %v", err)
	}
}
