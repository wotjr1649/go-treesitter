package treesitter_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	treesitter "github.com/wotjr1649/go-treesitter"
	"github.com/wotjr1649/go-treesitter/syntax"
)

func ExampleNew() {
	result, err := treesitter.New().Parse(context.Background(), syntax.Request{
		Filename: "hello.go", Source: []byte("package hello\nfunc Hello() {}\n"),
		Timeout: time.Second,
	})
	if result.Tree != nil {
		defer result.Tree.Close()
	}
	if err != nil || !result.Complete() {
		fmt.Println("parse did not complete")
		return
	}
	fmt.Println(result.Outcome)
	// Output: accepted_clean
}

func TestExternalConsumer(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	module, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	var goVersion string
	for _, line := range strings.Split(string(module), "\n") {
		if strings.HasPrefix(line, "go ") {
			goVersion = strings.TrimSpace(strings.TrimPrefix(line, "go "))
		}
	}
	if goVersion == "" {
		t.Fatal("missing module Go version")
	}
	dir := t.TempDir()
	for _, copy := range []struct{ from, to string }{{"testdata/consumer/main.go", "main.go"}, {"go.sum", "go.sum"}} {
		data, err := os.ReadFile(copy.from)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, copy.to), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	data := fmt.Sprintf("module example.com/consumer\n\ngo %s\n\nrequire github.com/wotjr1649/go-treesitter v0.0.0\nreplace github.com/wotjr1649/go-treesitter => %q\n", goVersion, filepath.ToSlash(root))
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", "-mod=mod", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=", "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("external consumer: %v\n%s", err, out)
	}
	if string(out) != "external consumer: fresh, edit, errors, cancellation, release OK\n" {
		t.Fatalf("unexpected consumer receipt: %q", out)
	}
	t.Logf("%s", out)
	if err := os.Mkdir(filepath.Join(dir, "blocked"), 0700); err != nil {
		t.Fatal(err)
	}
	negative := "package main\nimport _ \"github.com/wotjr1649/go-treesitter/internal/gtsadapter\"\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "blocked", "main.go"), []byte(negative), 0600); err != nil {
		t.Fatal(err)
	}
	blocked := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-o", filepath.Join(dir, "blocked.exe"), "./blocked")
	blocked.Dir, blocked.Env = dir, cmd.Env
	out, err = blocked.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "use of internal package") || !strings.Contains(string(out), "not allowed") {
		t.Fatalf("internal import was not rejected as expected: %v\n%s", err, out)
	}
	t.Log("external consumer: internal adapter import rejected")
}
