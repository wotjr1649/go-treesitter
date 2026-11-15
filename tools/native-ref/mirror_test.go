package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareMirrorFallback(t *testing.T) {
	var archive bytes.Buffer
	zw := gzip.NewWriter(&archive)
	tw := tar.NewWriter(zw)
	body := "synthetic parser"
	if err := tw.WriteHeader(&tar.Header{Name: "root/src/parser.c", Mode: 0644, Size: int64(len(body))}); err != nil {
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
	for _, mode := range []string{"unavailable", "wrong-upstream-hash", "wrong-mirror-hash", "upstream-ok"} {
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
						http.Error(w, "unavailable", 503)
						return
					}
					if mode == "wrong-upstream-hash" {
						fmt.Fprint(w, "corrupt")
						return
					}
				case "/mirror/grammar.tar.gz":
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
			s := source{Repo: "https://github.com/example/grammar", Commit: strings.Repeat("a", 40), Archive: "grammar.tar.gz", ArchiveURL: server.URL + "/upstream", ArchiveSHA256: sum, ExtractDir: "grammar", Dirs: []string{"src"}, Mirrors: []string{server.URL + "/mirror"}}
			err := prepare(s, dl, work, true, map[string]bool{})
			if mode == "wrong-mirror-hash" {
				if err == nil {
					t.Fatal("accepted corrupt mirror")
				}
				if _, err := os.Stat(filepath.Join(dl, s.Archive)); !os.IsNotExist(err) {
					t.Fatal("corrupt download retained")
				}
				if _, err := os.Stat(filepath.Join(work, "grammar", "src", "parser.c")); !os.IsNotExist(err) {
					t.Fatal("corrupt archive extracted")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(filepath.Join(work, "grammar", "src", "parser.c"))
				if err != nil || string(got) != body {
					t.Fatalf("extracted %q: %v", got, err)
				}
			}
			want := "/upstream,/mirror/grammar.tar.gz"
			if mode == "upstream-ok" {
				want = "/upstream"
			}
			if strings.Join(calls, ",") != want {
				t.Fatalf("calls %v, want %s", calls, want)
			}
		})
	}
}
