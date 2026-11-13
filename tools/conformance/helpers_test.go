package conformance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	treesitter "github.com/wotjr1649/go-treesitter"
)

var bg = context.Background()

// dataRoot returns TSG_S15_DIR or skips the test.
func dataRoot(t *testing.T) string {
	t.Helper()
	d := os.Getenv("TSG_S15_DIR")
	if d == "" {
		t.Skip("TSG_S15_DIR is not set: the conformance lane needs the local session-15 data tree " +
			"(corpus, edits, large inputs and native dumps); skipping")
	}
	return d
}

// langs returns the linked languages selected by TSG_S15_LANGS (comma list; default all linked), skipping when none.
func langs(t *testing.T) []string {
	t.Helper()
	all := Linked()
	if v := os.Getenv("TSG_S15_LANGS"); v != "" {
		var sel []string
		for _, l := range strings.Split(v, ",") {
			if slices.Contains(all, l) {
				sel = append(sel, l)
			} else {
				t.Logf("language %q is not linked into this binary (build tag no_%s?): not tested", l, l)
			}
		}
		all = sel
	}
	if len(all) == 0 {
		t.Skip("no grammar module linked for the selected languages")
	}
	return all
}

func newParser(t *testing.T, lang string, opts ...treesitter.Option) *treesitter.Parser {
	t.Helper()
	p, err := treesitter.NewParser(languages[lang](), opts...)
	if err != nil {
		t.Fatalf("NewParser(%s): %v", lang, err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func readCSV(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return rows[1:]
}

func sha(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// firstDiff describes the first differing line of two dumps.
func firstDiff(got, want []byte) string {
	g, w := bytes.Split(got, []byte{'\n'}), bytes.Split(want, []byte{'\n'})
	for i := 0; i < max(len(g), len(w)); i++ {
		var a, b []byte
		if i < len(g) {
			a = g[i]
		}
		if i < len(w) {
			b = w[i]
		}
		if !bytes.Equal(a, b) {
			return fmt.Sprintf("line %d: got %q want %q (got %d lines, want %d)", i, a, b, len(g), len(w))
		}
	}
	return "identical"
}

// outDir is where failure dumps and the summary log are preserved (TSG_CONF_OUT, optional).
func outDir() string { return os.Getenv("TSG_CONF_OUT") }

var summaryMu sync.Mutex

// summary logs one JSON line and appends it to $TSG_CONF_OUT/summary.jsonl.
func summary(t *testing.T, kv map[string]any) {
	t.Helper()
	kv["test"] = t.Name()
	kv["time"] = time.Now().Format(time.RFC3339)
	b, _ := json.Marshal(kv)
	t.Logf("SUMMARY %s", b)
	if d := outDir(); d != "" {
		summaryMu.Lock()
		defer summaryMu.Unlock()
		os.MkdirAll(d, 0o755)
		f, err := os.OpenFile(filepath.Join(d, "summary.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			f.Write(append(b, '\n'))
			f.Close()
		}
	}
}

// preserve stores a failing artifact under $TSG_CONF_OUT/fail (no-op without TSG_CONF_OUT).
func preserve(name string, data []byte) {
	if d := outDir(); d != "" {
		p := filepath.Join(d, "fail", name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, data, 0o644)
	}
}

func atou(t *testing.T, s string) uint32 {
	t.Helper()
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	return uint32(n)
}
