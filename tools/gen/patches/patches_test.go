package patches

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestApply(t *testing.T) {
	const diff = "--- a/src/parser.c\n+++ b/src/parser.c\n@@ -1,2 +1,2 @@\n a\n-b\n+c\n"
	for _, tc := range []string{"valid", "patch hash", "before hash", "after hash", "context", "other file", "path", "report"} {
		t.Run(tc, func(t *testing.T) {
			root := t.TempDir()
			// A build copy sits beneath the checkout; Git must not prepend that checkout's path.
			if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
				t.Fatalf("git init: %s %v", out, err)
			}
			if out, err := exec.Command("git", "-C", root, "config", "core.autocrlf", "true").CombinedOutput(); err != nil {
				t.Fatalf("git config: %s %v", out, err)
			}
			src := filepath.Join(root, "source")
			patchRoot := filepath.Join(root, "generator")
			if err := os.MkdirAll(filepath.Join(src, "src"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(patchRoot, "patches", "test"), 0755); err != nil {
				t.Fatal(err)
			}
			before := []byte("a\nb\n")
			patch := []byte(diff)
			p := Pin{File: "patches/test/0001-fix.patch", Source: "src/parser.c", SHA256: sum(patch), BeforeSHA256: sum(before), AfterSHA256: sum([]byte("a\nc\n")), UpstreamReport: "https://example.com/issues/1"}
			switch tc {
			case "patch hash":
				p.SHA256 = strings.Repeat("0", 64)
			case "before hash":
				p.BeforeSHA256 = strings.Repeat("0", 64)
			case "after hash":
				p.AfterSHA256 = strings.Repeat("0", 64)
			case "context":
				patch = []byte(strings.ReplaceAll(diff, " a\n", " z\n"))
				p.SHA256 = sum(patch)
			case "other file":
				patch = []byte(strings.ReplaceAll(diff, "src/parser.c", "src/other.c"))
				p.SHA256 = sum(patch)
			case "path":
				p.Source = "../escape.c"
			case "report":
				p.UpstreamReport = ""
			}
			if err := os.WriteFile(filepath.Join(src, "src", "parser.c"), before, 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(patchRoot, filepath.FromSlash(p.File)), patch, 0644); err != nil {
				t.Fatal(err)
			}
			err := Apply(patchRoot, src, []Pin{p})
			if tc != "valid" {
				if err == nil {
					t.Fatal("invalid patch accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(src, "src", "parser.c"))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "a\nc\n" {
				t.Fatalf("got %q", got)
			}
			if err := Apply(patchRoot, src, []Pin{p}); err == nil {
				t.Fatal("already applied patch accepted")
			}
		})
	}
}

func TestGeneratedPatchKeepsReferenceHashes(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "patches"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	diff := []byte("--- a/src/parser.c\n+++ b/src/parser.c\n@@ -1 +1 @@\n-old\n+new\n")
	if err := os.WriteFile(filepath.Join(root, "patches/fix.patch"), diff, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src/parser.c"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	p := Pin{File: "patches/fix.patch", Source: "src/parser.c", SHA256: sum(diff), BeforeSHA256: "reference-before", AfterSHA256: "reference-after", GeneratedBeforeSHA256: sum([]byte("old\n")), GeneratedAfterSHA256: sum([]byte("new\n")), UpstreamReport: "https://example.com/1"}
	pins := []Pin{p}
	if err := ApplyGenerated(root, root, pins); err != nil {
		t.Fatal(err)
	}
	if pins[0] != p {
		t.Fatal("reference identities mutated")
	}
	p.GeneratedAfterSHA256 = ""
	if err := ApplyGenerated(root, root, []Pin{p}); err == nil {
		t.Fatal("partial generated identity accepted")
	}
}
