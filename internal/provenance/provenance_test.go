package provenance_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestResolvedDependency(t *testing.T) {
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
		t.Fatalf("resolved dependency: %v\n%s", err, out)
	}
	t.Logf("%s", out)
}
