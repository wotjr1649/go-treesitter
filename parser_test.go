package treesitter_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
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
	consumer, err := os.ReadFile("testdata/consumer/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), consumer, 0600); err != nil {
		t.Fatal(err)
	}
	data := fmt.Sprintf("module example.com/consumer\n\ngo %s\n\nrequire github.com/wotjr1649/go-treesitter v0.0.1\n", goVersion)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", "-mod=mod", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=", "GOPROXY="+consumerProxy(t, module),
		"GONOPROXY=none", "GOPRIVATE=", "GOSUMDB=off", "GOTOOLCHAIN=local", "GOMODCACHE="+filepath.Join(t.TempDir(), "modcache"))
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("external consumer: %v\n%s\n%s", err, out, diagnostics.Bytes())
	}
	if string(out) != "external consumer: fresh, edit, errors, cancellation, release OK\n" {
		t.Fatalf("unexpected consumer receipt: %q", out)
	}
	t.Logf("%s", out)
	graph := exec.CommandContext(ctx, "go", "list", "-m", "-json", "all")
	graph.Dir, graph.Env = dir, cmd.Env
	out, err = graph.Output()
	if err != nil {
		t.Fatalf("consumer module graph: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(out))
	count := 0
	for {
		var resolved struct {
			Path, Version string
			Main          bool
			Replace       *json.RawMessage
		}
		err := decoder.Decode(&resolved)
		if err == io.EOF {
			break
		}
		if err != nil || resolved.Replace != nil {
			t.Fatalf("consumer graph has invalid data or a replace: %v", err)
		}
		if !resolved.Main && (resolved.Path != "github.com/wotjr1649/go-treesitter" || resolved.Version != "v0.0.1") {
			t.Fatalf("unexpected external dependency: %+v", resolved)
		}
		count++
	}
	if count != 2 {
		t.Fatalf("expected only consumer and v0.0.1 module, got %d modules", count)
	}
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

// A local module proxy exercises the same versioned ZIP consumption as a remote
// proxy, without publishing a release or borrowing the source tree via replace.
func consumerProxy(t *testing.T, module []byte) string {
	t.Helper()
	const modulePath = "github.com/wotjr1649/go-treesitter"
	proxy := t.TempDir()
	versions := filepath.Join(proxy, filepath.FromSlash(modulePath), "@v")
	if err := os.MkdirAll(versions, 0700); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, name := range []string{"go.mod", "parser.go", "LICENSE", "THIRD_PARTY_NOTICES.md", "LICENSES", "syntax", "internal"} {
		err := filepath.WalkDir(name, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			if !entry.Type().IsRegular() {
				return fmt.Errorf("nonregular module file: %s", path)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			file, err := writer.Create(modulePath + "@v0.0.1/" + filepath.ToSlash(path))
			if err != nil {
				return err
			}
			_, err = file.Write(content)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	for extension, data := range map[string][]byte{
		"mod": module, "zip": archive.Bytes(),
		"info": []byte("{\"Version\":\"v0.0.1\",\"Time\":\"2026-09-23T00:00:00Z\"}\n"),
	} {
		if err := os.WriteFile(filepath.Join(versions, "v0.0.1."+extension), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.ToSlash(proxy)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}
