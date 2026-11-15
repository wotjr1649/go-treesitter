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
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, data, native, langs, message string }{
		{"data", "", root, lang, "neither TSG_DATA_DIR"},
		{"native", root, "", lang, "no native executable"},
		{"rows", root, root, lang, "no corpus rows"},
		{"linked", root, root, "not-a-linked-grammar", "not linked"},
	} {
		for _, strict := range []string{"", "1"} {
			t.Run(tc.name+"/strict="+strict, func(t *testing.T) {
				cmd := exec.Command(exe, "-test.run=^TestFieldLookup$", "-test.v")
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
