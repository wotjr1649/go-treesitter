package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchSelectedLanguageAndLicense(t *testing.T) {
	source, license := []byte("real input\n"), []byte("permissive license\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/source":
			w.Write(source)
		case "/license":
			w.Write(license)
		default:
			t.Errorf("unexpected fetch: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	es := []entry{
		{Lang: "excluded", Kind: "url", URL: srv.URL + "/excluded"},
		{Lang: "html", File: "page.html", Kind: "url", URL: srv.URL + "/source", SHA256: sum(source), LicenseURL: srv.URL + "/license", LicenseSHA256: sum(license)},
	}
	manifest := filepath.Join(dir, "large.json")
	writeManifest := func() {
		t.Helper()
		b, err := json.Marshal(es)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifest, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeManifest()
	out := filepath.Join(dir, "out")
	if err := fetch(manifest, "", out, false, []string{"html"}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string][]byte{"page.html": source, "page.html.LICENSE": license} {
		got, err := os.ReadFile(filepath.Join(out, "html", name))
		if err != nil || string(got) != string(want) {
			t.Fatalf("%s: got %q, err %v", name, got, err)
		}
	}
	es[1].LicenseSHA256 = sum([]byte("wrong"))
	writeManifest()
	if err := fetch(manifest, "", filepath.Join(dir, "bad"), false, []string{"html"}); err == nil {
		t.Fatal("accepted a license hash mismatch")
	}
}
