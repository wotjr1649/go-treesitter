package conformance

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	treesitter "github.com/wotjr1649/go-treesitter"
	proc "github.com/wotjr1649/go-treesitter/tools/bench/proc"
)

// utf8NativeExe finds the native reference dumper: TSG_NATIVE_EXE_DIR (the native-ref -out directory, as every other native
// test) or else the local session-15 tree's native/exe2/<lang>.exe; "" when neither has it.
func utf8NativeExe(root, lang string) string {
	if exe, ok := nativeExe(lang); ok {
		return exe
	}
	exe := filepath.Join(root, "native", "exe2", lang+".exe")
	if _, err := os.Stat(exe); err == nil {
		return exe
	}
	return ""
}

// nativeDump runs the native reference dumper `<exe> dump <file>`.
func nativeDump(t *testing.T, exe string, src []byte) []byte {
	t.Helper()
	f := filepath.Join(t.TempDir(), "in.bin")
	if err := os.WriteFile(f, src, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := proc.Command(exe, "dump", f).Output()
	if err != nil {
		t.Fatalf("native dump of a %d-byte input: %v", len(src), err)
	}
	return out
}

// TestInvalidUTF8VsNative: the corpus holds no invalid UTF-8, so this derives inputs with every kind of invalid or
// unusual byte sequence from a corpus file. The tree must equal the native C tree byte for byte (C decodes invalid
// bytes into error nodes; a leading BOM is skipped), HasInvalidUTF8 is compared with utf8.Valid and WithStrictUTF8
// must reject exactly the inputs Go calls invalid.
func TestInvalidUTF8VsNative(t *testing.T) {
	root := dataRoot(t)
	cases := []struct {
		name string
		ins  string
	}{
		{"ff", "\xff"}, {"truncated-3byte", "\xe2\x82"}, {"overlong-c0", "\xc0\x80"}, {"surrogate", "\xed\xa0\x80"},
		{"above-10ffff", "\xf4\x90\x80\x80"}, {"lone-continuation", "\x80"}, {"bom", "\xef\xbb\xbf"}, {"nul", "\x00"},
		{"noncharacter-valid", "\xef\xbf\xbe"},
	}
	for _, lang := range langs(t) {
		t.Run(lang, func(t *testing.T) {
			exe := utf8NativeExe(root, lang)
			if exe == "" {
				skipConformance(t, "no native executable for "+lang+" (TSG_NATIVE_EXE_DIR or <data>/native/exe2)")
			}
			base, _ := smallReference(t, root, lang)
			p := newParser(t, lang)
			strict := newParser(t, lang, treesitter.WithStrictUTF8())
			mid := 0
			for i, c := range base {
				if c == '\n' {
					mid = i + 1
					break
				}
			}
			var run, mism, flagDiff int
			for _, c := range cases {
				for _, where := range []string{"end", "start", "after-first-line"} {
					var src []byte
					switch where {
					case "end":
						src = append(append([]byte{}, base...), c.ins...)
					case "start":
						src = append([]byte(c.ins), base...)
					default:
						src = append(append(append([]byte{}, base[:mid]...), c.ins...), base[mid:]...)
					}
					run++
					id := c.name + "/" + where
					tree, err := p.Parse(bg, src, nil)
					if err != nil {
						mism++
						t.Errorf("%s: Parse: %v", id, err)
						continue
					}
					snap, err := tree.Snapshot(bg)
					if err != nil {
						mism++
						t.Errorf("%s: Snapshot: %v", id, err)
						tree.Close()
						continue
					}
					got, want := Dump(snap), nativeDump(t, exe, src)
					if string(got) != string(want) {
						mism++
						t.Errorf("%s: dump differs from native C: %s", id, firstDiff(got, want))
						preserve("utf8/"+lang+"/"+c.name+"-"+where+".go.dump", got)
					}
					valid := utf8.Valid(src)
					if tree.HasInvalidUTF8() == valid {
						flagDiff++
						t.Errorf("%s: HasInvalidUTF8=%v but utf8.Valid=%v", id, tree.HasInvalidUTF8(), valid)
					}
					tree.Close()
					_, serr := strict.Parse(bg, src, nil)
					if valid != (serr == nil) || (serr != nil && !errors.Is(serr, treesitter.ErrInvalidInput)) {
						mism++
						t.Errorf("%s: strict mode result %v for utf8.Valid=%v", id, serr, valid)
					}
				}
			}
			summary(t, map[string]any{"lang": lang, "inputs": run, "mismatches": mism, "flagDisagreements": flagDiff})
		})
	}
}

func TestUTF8NativeExe(t *testing.T) {
	root, exeDir := t.TempDir(), t.TempDir()
	touch := func(p string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TSG_NATIVE_EXE_DIR", "")
	if got := utf8NativeExe(root, "json"); got != "" {
		t.Fatalf("nothing available: got %q", got)
	}
	legacy := filepath.Join(root, "native", "exe2", "json.exe")
	touch(legacy)
	if got := utf8NativeExe(root, "json"); got != legacy {
		t.Fatalf("legacy layout: got %q want %q", got, legacy)
	}
	t.Setenv("TSG_NATIVE_EXE_DIR", exeDir)
	want, _ := nativeExe("json") // the path, whether or not the file exists yet
	touch(want)
	if got := utf8NativeExe(root, "json"); got != want {
		t.Fatalf("TSG_NATIVE_EXE_DIR wins: got %q want %q", got, want)
	}
}

func TestRequiredName(t *testing.T) {
	for name, want := range map[string]bool{
		"TestSnapshotPointsNative": true, "TestSnapshotPointsNative/json": true, "TestInvalidUTF8VsNative/go": true,
		"TestQuerySmoke/c": true, "TestFieldLookupEdits/python": true, "TestQuerySmokeX": false, "TestCorpus/json": false,
	} {
		if requiredName(name) != want {
			t.Errorf("requiredName(%q) != %v", name, want)
		}
	}
}
