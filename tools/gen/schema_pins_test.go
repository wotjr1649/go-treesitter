package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wotjr1649/go-treesitter/tools/gen/schema"
)

// Opt-in because source archives are deliberately not downloaded by unit tests.
func TestPinnedSchemas(t *testing.T) {
	dl := os.Getenv("TSG_SCHEMA_ARCHIVES")
	if dl == "" {
		t.Skip("set TSG_SCHEMA_ARCHIVES to the SHA-pinned grammar archive directory")
	}
	pinPath := os.Getenv("TSG_SCHEMA_PINS")
	if pinPath == "" {
		pinPath = "pins.json"
	}
	pinHash, err := sumFile(pinPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pins=%s pinsSHA256=%s", pinPath, pinHash)
	pins, err := loadPins(pinPath)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(pins.Grammars))
	for name := range pins.Grammars {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			g := pins.Grammars[name]
			archive := filepath.Join(dl, g.Archive)
			hash, err := sumFile(archive)
			if err != nil {
				t.Fatal(err)
			}
			if hash != g.ArchiveSHA256 {
				t.Fatalf("archive SHA256 got %s, want %s", hash, g.ArchiveSHA256)
			}
			data, err := schemaFromArchive(archive, path.Join(g.GrammarDir, "node-types.json"))
			if err != nil {
				t.Fatal(err)
			}
			r := schema.Check(data)
			t.Logf("commit=%s archiveSHA256=%s schemaSHA256=%s bytes=%d assessment=%s findings=%+v",
				g.Commit, hash, sumBytes(data), len(data), r.Assessment, r.Findings)
			if r.Assessment != "PASS" {
				t.Fatalf("strict schema check %s", r.Assessment)
			}
		})
	}
}

func schemaFromArchive(name, member string) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	tr := tar.NewReader(z)
	var data []byte
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		_, relative, ok := strings.Cut(h.Name, "/")
		if !ok || relative != member {
			continue
		}
		if data != nil || !h.FileInfo().Mode().IsRegular() || h.Size > maxSchemaBytes {
			return nil, fmt.Errorf("invalid or duplicate schema member %q", h.Name)
		}
		data, err = io.ReadAll(io.LimitReader(tr, maxSchemaBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > maxSchemaBytes {
			return nil, fmt.Errorf("schema member too large: %q", h.Name)
		}
	}
	if data == nil {
		return nil, fmt.Errorf("missing schema member %q in %q", member, name)
	}
	return data, nil
}
