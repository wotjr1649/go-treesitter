package conformance

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestBalancingCancel drives only gen -balance-probe outputs, never product grammar modules.
func TestBalancingCancel(t *testing.T) {
	root := os.Getenv("TSG_BALANCE_PROBE_DIR")
	if root == "" {
		t.Skip("TSG_BALANCE_PROBE_DIR is not set: generate test-only modules with gen -balance-probe")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("TSG_NATIVE_EXE_DIR") == "" {
		t.Fatal("TSG_NATIVE_EXE_DIR is required")
	}
	nativeDir, err := filepath.Abs(os.Getenv("TSG_NATIVE_EXE_DIR"))
	if err != nil {
		t.Fatal(err)
	}
	out := outDir()
	if out != "" {
		out, err = filepath.Abs(out)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			name := lang
			if name == "go" {
				name = "golang"
			}
			dir := filepath.Join(root, name)
			var provenance struct {
				TestOnly string `json:"testOnly"`
				Name     string `json:"name"`
			}
			if err := json.Unmarshal(readFile(t, filepath.Join(dir, "provenance.json")), &provenance); err != nil {
				t.Fatal(err)
			}
			if provenance.TestOnly != "balancing-cancel" || provenance.Name != name {
				t.Fatalf("refusing non-probe module: %+v", provenance)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 11*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, "go", "test", "-mod=mod", "-count=1", "-v", "-timeout", "10m", "-run", "^TestBalancingCancelProbe$", ".")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off", "GOTOOLCHAIN=local", "GOPROXY=off", "TSG_NATIVE_EXE_DIR="+nativeDir, "TSG_CONF_OUT="+out)
			out, err := cmd.CombinedOutput()
			if d := outDir(); d != "" {
				if e := os.MkdirAll(filepath.Join(d, "balance"), 0o755); e != nil {
					t.Fatal(e)
				}
				if e := os.WriteFile(filepath.Join(d, "balance", name+".log"), out, 0o644); e != nil {
					t.Fatal(e)
				}
			}
			t.Logf("%s", out)
			if err != nil {
				t.Fatalf("probe %s: %v", name, err)
			}
		})
	}
}
