package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The same tree and constant as goldenTree in tools/gen/pinbump: pinbump re-implements treeSHA256 over in-memory files,
// and this keeps the two in step.
func TestTreeSHA256MatchesPinbump(t *testing.T) {
	root := t.TempDir()
	for p, b := range map[string]string{"src/a.c": "A", "src/b/c.h": "B", "other/x": "X"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(b), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := treeSHA256(root, []string{"src"})
	const want = "70a1830b710d98abb6170dd3710480caffbe9ecd908b22ab968471a59a33a4ad"
	if err != nil || got != want {
		t.Fatalf("treeSHA256 = %s, %v; want %s", got, err, want)
	}
}
