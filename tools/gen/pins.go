package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Pins is tools/gen/pins.json: the machine-readable source of truth for everything a generation consumes.
type Pins struct {
	Schema           int                   `json:"schema"`
	ABI              int                   `json:"abi"`
	Runtime          RuntimePin            `json:"runtime"`
	Tools            ToolPins              `json:"tools"`
	MemoryGrowSHA256 string                `json:"memoryGrowSHA256"` // sha256 of the original wasm2go memory_grow function text
	ShimSHA256       string                `json:"shimSHA256"`       // shim/tsg_abi.c
	Grammars         map[string]GrammarPin `json:"grammars"`
}

type RuntimePin struct {
	Release       string   `json:"release"`
	Repo          string   `json:"repo"`
	Commit        string   `json:"commit"`
	ExtractDir    string   `json:"extractDir"` // under -src
	Archive       string   `json:"archive"`    // file name under -dl
	ArchiveSHA256 string   `json:"archiveSHA256"`
	Dirs          []string `json:"dirs"` // hashed trees, relative to ExtractDir
	TreeSHA256    string   `json:"treeSHA256"`
}

type ToolPins struct {
	WasiSDK       string `json:"wasi-sdk"`
	Clang         string `json:"clang"`         // first line of `clang --version`
	Wasm2go       string `json:"wasm2go"`       // module version recorded in the binary
	Wasm2goSHA256 string `json:"wasm2goSHA256"` // the pinned binary (host specific)
	Go            string `json:"go"`            // Go version wasm2go was built with
}

type GrammarPin struct {
	Repo          string            `json:"repo"`
	Commit        string            `json:"commit"` // full commit
	ExtractDir    string            `json:"extractDir"`
	Archive       string            `json:"archive"`
	ArchiveSHA256 string            `json:"archiveSHA256"`
	GrammarDir    string            `json:"grammarDir"` // directory with parser.c, relative to ExtractDir
	Dirs          []string          `json:"dirs"`       // hashed trees, relative to ExtractDir
	TreeSHA256    string            `json:"treeSHA256"`
	Files         map[string]string `json:"files"` // parser.c / scanner.c -> sha256
	LicenseSHA256 string            `json:"licenseSHA256"`
	CSymbol       string            `json:"cSymbol"` // tree_sitter_xxx
}

func loadPins(path string) (*Pins, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Pins
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &p, nil
}

func sumBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func sumFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// treeSHA256 hashes every regular file under the given directories of root: sha256 over the sorted lines
// "<slash path relative to root>\x00<file sha256>\n". Directory names are inputs, so moving a file changes the hash.
func treeSHA256(root string, dirs []string) (string, error) {
	var lines []string
	for _, d := range dirs {
		base := filepath.Join(root, filepath.FromSlash(d))
		err := filepath.WalkDir(base, func(p string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return err
			}
			if !e.Type().IsRegular() {
				return fmt.Errorf("%s: not a regular file", p)
			}
			s, err := sumFile(p)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			lines = append(lines, filepath.ToSlash(rel)+"\x00"+s+"\n")
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	if len(lines) == 0 {
		return "", fmt.Errorf("no files under %v", dirs)
	}
	sort.Strings(lines)
	return sumBytes([]byte(strings.Join(lines, ""))), nil
}

func expect(what, got, want string) error {
	if got != want {
		return fmt.Errorf("pin mismatch: %s = %s, pinned %s", what, got, want)
	}
	return nil
}
