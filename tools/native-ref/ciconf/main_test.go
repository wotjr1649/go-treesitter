package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSelectedLanguages(t *testing.T) {
	got, err := selectedLanguages("cpp, javascript,cpp,rust")
	if err != nil || !reflect.DeepEqual(got, []string{"cpp", "javascript", "rust"}) {
		t.Fatalf("selectedLanguages = %v, %v", got, err)
	}
	if _, err := selectedLanguages("cpp,typo"); err == nil {
		t.Fatal("unknown language accepted")
	}
	all, err := selectedLanguages("")
	if err != nil || len(all) != 25 {
		t.Fatalf("all languages = %v, %v", all, err)
	}
}

func TestInputsOnlyDoesNotRunNative(t *testing.T) {
	root := t.TempDir()
	pins := filepath.Join(root, "pins.json")
	if err := os.WriteFile(pins, []byte("{\r\n\"grammars\":{\"json\":{\"extractDir\":\"json\",\"grammarDir\":\"src\"}}}\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	corpus := filepath.Join(root, "corpus")
	input := []byte("[1]\n")
	raw := append([]byte("===\nsmall\n===\n"), input...)
	raw = append(raw, []byte("\n---\n(document)\n")...)
	if err := writeFile(filepath.Join(corpus, "json", "test", "corpus", "sample.txt"), raw); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "out")
	// There is deliberately no native executable: only the supervised nightly lane may parse these seeds.
	if err := run(pins, corpus, filepath.Join(root, "missing-exe"), out, 1, 1, "json", true); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(out, "corpus", "json", "sample-001.in"))
	if err != nil || string(got) != string(input) {
		t.Fatalf("seed bytes = %q, %v", got, err)
	}
	for _, name := range []string{"native", "edits", "ci.json"} {
		if _, err := os.Stat(filepath.Join(out, name)); !os.IsNotExist(err) {
			t.Fatalf("unexpected native evidence %s: %v", name, err)
		}
	}
	meta, err := os.ReadFile(filepath.Join(out, "inputs.json"))
	if err != nil {
		t.Fatal(err)
	}
	var identity struct {
		PinsSHA256 string `json:"pinsSHA256"`
	}
	if err := json.Unmarshal(meta, &identity); err != nil {
		t.Fatal(err)
	}
	if want := sha([]byte("{\n\"grammars\":{\"json\":{\"extractDir\":\"json\",\"grammarDir\":\"src\"}}}\n")); identity.PinsSHA256 != want {
		t.Fatalf("pins hash = %s, LF-canonical hash = %s", identity.PinsSHA256, want)
	}
}
