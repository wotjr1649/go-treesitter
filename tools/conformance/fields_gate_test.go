package conformance

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFieldLookupGateConfiguration(t *testing.T) {
	linked := Linked()
	if len(linked) == 0 {
		t.Skip("configuration probe needs one linked grammar")
	}
	lang := linked[0]
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "corpus"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "corpus", "manifest.csv"), []byte("id,lang,source_ref,bytes,sha256\n"), 0644); err != nil {
		t.Fatal(err)
	}
	name := lang
	if name == "go" {
		name = "golang"
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(root, name), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "edits"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"manifest.csv", "results-native.csv"} {
		if err := os.WriteFile(filepath.Join(root, "edits", file), []byte("id,lang\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	missing := t.TempDir()
	d := filepath.Join(missing, "edits", lang)
	if err := os.MkdirAll(d, 0755); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"old", "new"} {
		if err := os.WriteFile(filepath.Join(d, "probe."+suffix), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	for file, data := range map[string]string{
		"manifest.csv": "id,lang,base_ref,start,old_end,new_end,old_sha256,new_sha256\n" +
			lang + "/probe," + lang + ",probe,0,0,0," + sha(nil) + "," + sha(nil) + "\n",
		"results-native.csv": "id,edit_exit,edit_sha256,fresh_exit,fresh_sha256,edit_ms\n",
	} {
		if err := os.WriteFile(filepath.Join(missing, "edits", file), []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, test, data, native, langs, message string }{
		{"data", "TestFieldLookup", "", root, lang, "neither TSG_DATA_DIR"},
		{"native", "TestFieldLookup", root, "", lang, "no native executable"},
		{"rows", "TestFieldLookup", root, root, lang, "no corpus rows"},
		{"linked", "TestFieldLookup", root, root, "not-a-linked-grammar", "not linked"},
		{"edited-data", "TestFieldLookupEdits", "", root, lang, "neither TSG_DATA_DIR"},
		{"edited-rows", "TestFieldLookupEdits", root, root, lang, "no edit rows"},
		{"edited-native", "TestFieldLookupEdits", missing, "", lang, "no native edit-fields result or executable"},
		{"edited-linked", "TestFieldLookupEdits", root, root, "not-a-linked-grammar", "not linked"},
	} {
		for _, strict := range []string{"", "1"} {
			t.Run(tc.name+"/strict="+strict, func(t *testing.T) {
				cmd := exec.Command(exe, "-test.run=^"+tc.test+"$", "-test.v")
				cmd.Env = append(os.Environ(), "TSG_DATA_DIR="+tc.data, "TSG_S15_DIR=", "TSG_NATIVE_EXE_DIR="+tc.native,
					"TSG_S15_LANGS="+tc.langs, "TSG_REQUIRE_FIELD_LOOKUP="+strict, "TSG_CONF_OUT=")
				out, err := cmd.CombinedOutput()
				if (err != nil) != (strict == "1") || !strings.Contains(string(out), tc.message) {
					t.Fatalf("strict=%q: %v\n%s", strict, err, out)
				}
				if strict == "" && !strings.Contains(string(out), "--- SKIP:") {
					t.Fatalf("local run did not skip:\n%s", out)
				}
			})
		}
	}
}
