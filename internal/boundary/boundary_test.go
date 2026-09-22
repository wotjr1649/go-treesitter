package boundary

import (
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const allowedImporter = "github.com/wotjr1649/go-treesitter/internal/gtsadapter"
const runtimePrefix = "github.com/wotjr1649/go-treesitter/internal/runtime"

func TestUpstreamImportBoundary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-json", "./...")
	cmd.Dir = "../.."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("package graph: %v", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(out)))
	for {
		var pkg struct {
			ImportPath                         string
			Imports, TestImports, XTestImports []string
		}
		err := decoder.Decode(&pkg)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, group := range [][]string{pkg.Imports, pkg.TestImports, pkg.XTestImports} {
			for _, dependency := range group {
				if dependency == "github.com/odvcencio/gotreesitter" || strings.HasPrefix(dependency, "github.com/odvcencio/gotreesitter/") {
					t.Errorf("external upstream import remains: %s imports %s", pkg.ImportPath, dependency)
				}
				bundled := dependency == runtimePrefix || strings.HasPrefix(dependency, runtimePrefix+"/")
				inside := pkg.ImportPath == runtimePrefix || strings.HasPrefix(pkg.ImportPath, runtimePrefix+"/")
				if bundled && !inside && pkg.ImportPath != allowedImporter {
					t.Errorf("boundary violation: %s imports %s; only %s is allowed", pkg.ImportPath, dependency, allowedImporter)
				}
			}
		}
	}
}
