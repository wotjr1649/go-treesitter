package gpl_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/go-treesitter/internal/provenance"
)

func TestGPLProvenance(t *testing.T) {
	data, err := os.ReadFile("provenance.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		RuntimeManifestSHA256 string `json:"runtime_manifest_sha256"`
		Files                 map[string]string
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	pins, err := provenance.Read()
	if err != nil || pins.Runtime.ManifestSHA256 != manifest.RuntimeManifestSHA256 {
		t.Fatalf("optional module/runtime identity mismatch: %v", err)
	}
	seen := 0
	err = filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if path == "provenance.json" || path == "go.sum" {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("nonregular optional module entry: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) != manifest.Files[filepath.ToSlash(path)] {
			return fmt.Errorf("optional module file changed or unrecorded: %s", path)
		}
		seen++
		return nil
	})
	if err != nil || seen != len(manifest.Files) {
		t.Fatalf("optional module inventory: %v count=%d want=%d", err, seen, len(manifest.Files))
	}
	module, err := os.ReadFile("go.mod")
	if err != nil || strings.Contains(string(module), "replace ") {
		t.Fatalf("optional module contains a replace or unreadable manifest: %v", err)
	}
}
