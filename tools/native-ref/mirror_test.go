package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWasiLibcMirrorFallback(t *testing.T) {
	const archiveName = "wasi-libc.tar.gz"
	const ctypeDir = "libc-top-half/musl/src/ctype"
	var archive bytes.Buffer
	zw := gzip.NewWriter(&archive)
	tw := tar.NewWriter(zw)
	body := "synthetic ctype source"
	if err := tw.WriteHeader(&tar.Header{Name: "root/" + ctypeDir + "/iswalpha.c", Mode: 0644, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	for _, err := range []error{tw.Close(), zw.Close()} {
		if err != nil {
			t.Fatal(err)
		}
	}
	hash := sha256.Sum256(archive.Bytes())
	sum := hex.EncodeToString(hash[:])
	for _, mode := range []string{"unavailable", "wrong-upstream-hash", "wrong-mirror-hash", "upstream-ok", "corrupt-cache"} {
		t.Run(mode, func(t *testing.T) {
			var calls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, r.URL.Path)
				if r.Header.Get("Authorization") != "" {
					t.Error("credential sent to non-GitHub server")
				}
				switch r.URL.Path {
				case "/upstream":
					if mode == "unavailable" || mode == "wrong-mirror-hash" {
						http.Error(w, "unavailable", http.StatusServiceUnavailable)
						return
					}
					if mode == "wrong-upstream-hash" {
						fmt.Fprint(w, "corrupt")
						return
					}
				case "/mirror/" + archiveName:
					if mode == "wrong-mirror-hash" {
						fmt.Fprint(w, "corrupt")
						return
					}
				default:
					t.Errorf("unexpected URL %s", r.URL.Path)
				}
				w.Write(archive.Bytes())
			}))
			defer server.Close()
			t.Setenv("GITHUB_TOKEN", "test-placeholder")
			dl, work := t.TempDir(), t.TempDir()
			if mode == "corrupt-cache" { // a poisoned restored cache file is replaced by a verified download
				if err := os.WriteFile(filepath.Join(dl, archiveName), []byte("poisoned"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			raw, err := json.Marshal(pins{
				Mirrors: []string{server.URL + "/mirror"},
				WasiLibc: source{
					Repo: "https://github.com/WebAssembly/wasi-libc", Commit: strings.Repeat("a", 40),
					Archive: archiveName, ArchiveURL: server.URL + "/upstream", ArchiveSHA256: sum,
					ExtractDir: "wasi-libc", Dirs: []string{ctypeDir},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			pinsPath := filepath.Join(t.TempDir(), "pins.json")
			if err := os.WriteFile(pinsPath, raw, 0600); err != nil {
				t.Fatal(err)
			}
			p, err := readPins(pinsPath)
			if err != nil {
				t.Fatal(err)
			}
			extracted := filepath.Join(work, p.WasiLibc.ExtractDir, ctypeDir, "iswalpha.c")
			err = prepare(p.WasiLibc, dl, work, true, map[string]bool{})
			if mode == "wrong-mirror-hash" {
				if err == nil {
					t.Fatal("accepted corrupt mirror")
				}
				if _, err := os.Stat(filepath.Join(dl, archiveName)); !os.IsNotExist(err) {
					t.Fatal("corrupt download retained")
				}
				if _, err := os.Stat(extracted); !os.IsNotExist(err) {
					t.Fatal("corrupt archive extracted")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(extracted)
				if err != nil || string(got) != body {
					t.Fatalf("extracted %q: %v", got, err)
				}
			}
			want := "/upstream,/mirror/" + archiveName
			if mode == "upstream-ok" || mode == "corrupt-cache" {
				want = "/upstream"
			}
			if strings.Join(calls, ",") != want {
				t.Fatalf("calls %v, want %s", calls, want)
			}
		})
	}
}
