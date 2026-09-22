package provenance

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// VerifyRuntime binds the complete checked-in runtime to its approved manifest.
// It is used by validation before interpreting any runtime/oracle comparison.
func VerifyRuntime(root string) error {
	pins, err := Read()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(root, "internal/provenance/runtime.json"))
	if err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != pins.Runtime.ManifestSHA256 {
		return fmt.Errorf("runtime manifest identity mismatch")
	}
	var manifest struct {
		Schema         int
		Baseline       Baseline
		InternalModule string `json:"internal_module"`
		SourceSHA256   string `json:"source_sha256"`
		ImporterSHA256 string `json:"importer_sha256"`
		Patches        []struct{ Path, SHA256 string }
		Files          map[string]struct{ SHA256 string }
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	if manifest.Schema != 1 || manifest.Baseline != pins.Baseline ||
		manifest.InternalModule != pins.Runtime.Module || len(manifest.Files) == 0 {
		return fmt.Errorf("runtime origin identity mismatch")
	}
	check := func(name, expected string) error {
		if !filepath.IsLocal(name) {
			return fmt.Errorf("nonlocal runtime provenance path: %s", name)
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) != expected {
			return fmt.Errorf("runtime file identity mismatch: %s", name)
		}
		return nil
	}
	for _, file := range append(manifest.Patches, struct{ Path, SHA256 string }{"tools/runtime-bundle/source.json", manifest.SourceSHA256},
		struct{ Path, SHA256 string }{"tools/runtime-bundle/import.py", manifest.ImporterSHA256}) {
		if err := check(file.Path, file.SHA256); err != nil {
			return err
		}
	}
	base := filepath.Join(root, "internal/runtime")
	seen := 0
	err = filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("nonregular runtime entry: %s", path)
		}
		relative, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		file, exists := manifest.Files[name]
		if !exists {
			return fmt.Errorf("unrecorded runtime file: %s", name)
		}
		seen++
		return check("internal/runtime/"+name, file.SHA256)
	})
	if err == nil && seen != len(manifest.Files) {
		err = fmt.Errorf("runtime file inventory incomplete")
	}
	return err
}
