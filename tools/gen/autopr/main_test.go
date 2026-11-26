package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const base = `{"schema":3,"tools":{"x":"1"},"grammars":{"c":{"repo":"https://github.com/o/c","commit":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","archive":"c.tgz"},"json":{"repo":"https://github.com/o/j","commit":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}}`

func prop(mut func(string) string) []byte { return []byte(mut(base)) }

var good = candidate{"grammar", "c", "v1.2.3", strings.Repeat("c", 40)}

func moved(s string) string {
	return strings.Replace(s, strings.Repeat("a", 40), strings.Repeat("c", 40), 1)
}

func TestCheckPins(t *testing.T) {
	bad := func(c candidate, f func(string) string) error {
		_, err := checkPins([]byte(base), prop(f), c)
		return err
	}
	if err := bad(good, moved); err != nil {
		t.Fatalf("good proposal refused: %v", err)
	}
	for name, tc := range map[string]struct {
		c candidate
		f func(string) string
	}{
		"other grammar moved": {good, func(s string) string {
			return moved(strings.Replace(s, strings.Repeat("b", 40), strings.Repeat("d", 40), 1))
		}},
		"tool pin moved": {good, func(s string) string { return moved(strings.Replace(s, `"x":"1"`, `"x":"2"`, 1)) }},
		"field added": {good, func(s string) string {
			return moved(strings.Replace(s, `"archive":"c.tgz"`, `"archive":"c.tgz","archiveURL":"https://github.com/evil/x/a.tgz"`, 1))
		}},
		"repo switched":        {good, func(s string) string { return moved(strings.Replace(s, "o/c", "evil/c", 1)) }},
		"commit not candidate": {good, func(s string) string { return s }},
		"unknown grammar":      {candidate{"grammar", "zig", "v1.0.0", good.Commit}, moved},
		"added grammar":        {good, func(s string) string { return moved(strings.Replace(s, `"json":`, `"zig":{},"json":`, 1)) }},
		"bad tag":              {candidate{"grammar", "c", "v1.2.3\nx=1", good.Commit}, moved},
		"bad kind":             {candidate{"tool", "c", "v1.2.3", good.Commit}, moved},
		"foreign host": {good, func(s string) string {
			return moved(strings.Replace(s, `"archive":"c.tgz"`, `"archive":"c.tgz","archiveURL":"https://evil.example/x"`, 1))
		}},
	} {
		if err := bad(tc.c, tc.f); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestCheckFiles(t *testing.T) {
	read := func(string) ([]byte, error) { return []byte("module m\n"), nil }
	ok := file{Path: "grammars/c/engine_gen.go", Regular: true, Size: 10}
	for name, tc := range map[string]struct {
		f    []file
		fail bool
	}{
		"module file":    {[]file{ok, {Path: "grammars/c/go.mod", Regular: true, Data: []byte("module m\n")}}, false},
		"other module":   {[]file{{Path: "grammars/json/x.go", Regular: true}}, true},
		"workflow":       {[]file{{Path: ".github/workflows/x.yml", Regular: true}}, true},
		"traversal":      {[]file{{Path: "grammars/c/../../x", Regular: true}}, true},
		"symlink":        {[]file{{Path: "grammars/c/l", Regular: false}}, true},
		"go.mod changed": {[]file{{Path: "grammars/c/go.mod", Regular: true, Data: []byte("module m\nrequire evil v1\n")}}, true},
		"too big":        {[]file{{Path: "grammars/c/a", Regular: true, Size: maxBytes + 1}}, true},
		"backslash":      {[]file{{Path: "grammars/c/a\\b", Regular: true}}, true},
		"prefix sibling": {[]file{{Path: "grammars/cpp/a", Regular: true}}, true},
	} {
		if err := checkFiles(tc.f, []string{"c"}, read); (err != nil) != tc.fail {
			t.Errorf("%s: err=%v, want failure=%v", name, err, tc.fail)
		}
	}
}

func TestRunApply(t *testing.T) {
	root, prop, reg := t.TempDir(), t.TempDir(), t.TempDir()
	must := func(p, s string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must(filepath.Join(root, "tools/gen/pins.json"), base)
	must(filepath.Join(prop, "pins.json"), moved(base))
	must(filepath.Join(prop, "candidate.json"), `{"Kind":"grammar","Name":"c","Tag":"v1.2.3","Commit":"`+good.Commit+`"}`)
	must(filepath.Join(reg, "grammars/c/engine_gen.go"), "package c\n")
	if _, _, err := run(root, prop, reg, true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "grammars/c/engine_gen.go")); string(b) != "package c\n" {
		t.Fatal("regenerated file not applied")
	}
	must(filepath.Join(reg, "grammars/json/x.go"), "package j\n")
	if _, _, err := run(root, prop, reg, true); err == nil {
		t.Fatal("a file of another module was accepted")
	}
}

// Grammars that share one repository commit (typescript and tsx) are one pin: they move together or not at all.
func TestCheckPinsGroup(t *testing.T) {
	a, b := strings.Repeat("a", 40), strings.Repeat("c", 40)
	main := `{"grammars":{"ts":{"repo":"https://github.com/o/t","commit":"` + a + `"},"tsx":{"repo":"https://github.com/o/t","commit":"` + a + `"},"j":{"repo":"https://github.com/o/j","commit":"` + a + `"}}}`
	both := strings.ReplaceAll(main, a, b)
	c := candidate{"grammar", "ts", "v1.0.0", b}
	// j is another repository: it must not move even though its commit text is the same.
	jKept := strings.Replace(both, "https://github.com/o/j\",\"commit\":\""+b, "https://github.com/o/j\",\"commit\":\""+a, 1)
	names, err := checkPins([]byte(main), []byte(jKept), c)
	if err != nil || strings.Join(names, " ") != "ts tsx" {
		t.Fatalf("group move: names %v, err %v", names, err)
	}
	if _, err := checkPins([]byte(main), []byte(both), c); err == nil {
		t.Fatal("moving an unrelated repository's entry was accepted")
	}
	onlyTS := strings.Replace(main, `"commit":"`+a, `"commit":"`+b, 1)
	if _, err := checkPins([]byte(main), []byte(onlyTS), c); err == nil {
		t.Fatal("moving ts without tsx was accepted")
	}
	if err := checkFiles([]file{{Path: "grammars/tsx/x.go", Regular: true}, {Path: "grammars/ts/x.go", Regular: true}}, names, func(string) ([]byte, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
}
