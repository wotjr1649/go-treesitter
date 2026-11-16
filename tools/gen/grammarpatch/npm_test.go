package grammarpatch

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLockfileGrammarInput(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		t.Run(map[bool]string{false: "verified", true: "corrupt"}[corrupt], func(t *testing.T) {
			root := t.TempDir()
			cache := t.TempDir()
			const body = "module.exports = 'grammar';\n"
			var b bytes.Buffer
			z := gzip.NewWriter(&b)
			tw := tar.NewWriter(z)
			if err := tw.WriteHeader(&tar.Header{Name: "package/grammar.js", Size: int64(len(body)), Mode: 0644}); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := z.Close(); err != nil {
				t.Fatal(err)
			}
			raw := b.Bytes()
			sum := sha512.Sum512(raw)
			integrity := "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
			lock, _ := json.Marshal(map[string]any{"lockfileVersion": 3, "packages": map[string]any{"node_modules/tiny": map[string]any{"version": "1.2.3", "resolved": "https://registry.npmjs.org/tiny/-/tiny-1.2.3.tgz", "integrity": integrity, "hasInstallScript": true}}})
			if err := os.WriteFile(filepath.Join(root, "package-lock.json"), lock, 0644); err != nil {
				t.Fatal(err)
			}
			if corrupt {
				raw = append(append([]byte(nil), raw...), 0)
			}
			if err := os.WriteFile(filepath.Join(cache, hash([]byte(integrity))+".tgz"), raw, 0644); err != nil {
				t.Fatal(err)
			}
			inputs, err := prepareNPM(Config{Regenerate: true, GrammarDir: "typescript/src", NPMDependencies: []string{"tiny"}, NPMCache: cache}, root)
			if corrupt {
				if err == nil {
					t.Fatal("corrupt cached tarball accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(root, "typescript/node_modules/tiny/grammar.js"))
			if err != nil || string(got) != body {
				t.Fatalf("cwd grammar dependency missing: %s %v", got, err)
			}
			if len(inputs) != 1 || inputs[0].Version != "1.2.3" || inputs[0].LockfileSHA256 != hash(lock) || inputs[0].ScriptsExecuted {
				t.Fatalf("incomplete provenance: %+v", inputs)
			}
		})
	}
}

func npmFixture(t *testing.T, resolved string, packages bool) (Config, string) {
	t.Helper()
	root := t.TempDir()
	entries := map[string]any{}
	if packages {
		entries["node_modules/tiny"] = map[string]any{"version": "1.2.3", "resolved": resolved, "integrity": "sha512-unused"}
	}
	lock, err := json.Marshal(map[string]any{"lockfileVersion": 3, "packages": entries})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package-lock.json"), lock, 0644); err != nil {
		t.Fatal(err)
	}
	return Config{Regenerate: true, GrammarDir: "src", NPMDependencies: []string{"tiny"}, NPMCache: t.TempDir()}, root
}

func TestNPMRejectsUnpinnedInputs(t *testing.T) {
	for _, tc := range []struct {
		name, resolved, want string
		packages             bool
	}{
		{"host", "https://evil.invalid/tiny.tgz", "not pinned", true},
		{"scheme", "http://registry.npmjs.org/tiny.tgz", "not pinned", true},
		{"port", "https://registry.npmjs.org:443/tiny.tgz", "not pinned", true},
		{"userinfo", "https://user@registry.npmjs.org/tiny.tgz", "not pinned", true},
		{"missing-lock-entry", "", "not pinned", false},
		{"implicit-fetch", "https://registry.npmjs.org/tiny.tgz", "enable fetching", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, root := npmFixture(t, tc.resolved, tc.packages)
			if _, err := prepareNPM(c, root); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

type npmTransport func(*http.Request) (*http.Response, error)

func (f npmTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNPMRejectsRedirect(t *testing.T) {
	old := http.DefaultTransport
	defer func() { http.DefaultTransport = old }()
	requests := 0
	http.DefaultTransport = npmTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://evil.invalid/tiny.tgz"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})
	c, root := npmFixture(t, "https://registry.npmjs.org/tiny.tgz", true)
	c.FetchNPM = true
	if _, err := prepareNPM(c, root); err == nil || !strings.Contains(err.Error(), "redirect rejected") {
		t.Fatalf("redirect accepted: %v", err)
	}
	if requests != 1 {
		t.Fatalf("followed redirect: %d requests", requests)
	}
	files, err := os.ReadDir(c.NPMCache)
	if err != nil || len(files) != 0 {
		t.Fatalf("rejected response cached: %v %v", files, err)
	}
}

func TestExtractRejectsMaliciousTar(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		typeflag   byte
		duplicate  bool
	}{
		{"parent", "package/../evil", tar.TypeReg, false},
		{"backslash", `package/..\evil`, tar.TypeReg, false},
		{"drive", "package/C:/evil", tar.TypeReg, false},
		{"symlink", "package/evil", tar.TypeSymlink, false},
		{"hardlink", "package/evil", tar.TypeLink, false},
		{"duplicate", "package/evil", tar.TypeReg, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			z := gzip.NewWriter(&b)
			w := tar.NewWriter(z)
			h := &tar.Header{Name: tc.path, Typeflag: tc.typeflag, Mode: 0644}
			if tc.typeflag == tar.TypeSymlink || tc.typeflag == tar.TypeLink {
				h.Linkname = "../../evil"
			}
			if err := w.WriteHeader(h); err != nil {
				t.Fatal(err)
			}
			if tc.duplicate {
				if err := w.WriteHeader(h); err != nil {
					t.Fatal(err)
				}
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			if err := z.Close(); err != nil {
				t.Fatal(err)
			}
			archive := filepath.Join(t.TempDir(), "bad.tgz")
			if err := os.WriteFile(archive, b.Bytes(), 0644); err != nil {
				t.Fatal(err)
			}
			if err := extract(archive, t.TempDir()); err == nil {
				t.Fatal("malicious tar accepted")
			}
		})
	}
	if local("evil\x00suffix") {
		t.Fatal("NUL path accepted")
	}
}

func TestExtractRejectsNULTarPath(t *testing.T) {
	var raw bytes.Buffer
	w := tar.NewWriter(&raw)
	// A long name forces a PAX path record; NUL in that record is invalid tar metadata.
	name := "package/" + strings.Repeat("a", 120) + "?suffix"
	if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0644}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data := bytes.Replace(raw.Bytes(), []byte("?suffix"), []byte("\x00suffix"), 1)
	if bytes.Equal(data, raw.Bytes()) {
		t.Fatal("NUL fixture was not injected")
	}
	var compressed bytes.Buffer
	z := gzip.NewWriter(&compressed)
	if _, err := z.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "nul.tgz")
	if err := os.WriteFile(archive, compressed.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	if err := extract(archive, t.TempDir()); err == nil {
		t.Fatal("NUL tar path accepted")
	}
}
