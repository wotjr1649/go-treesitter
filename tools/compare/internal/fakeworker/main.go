// Command fakeworker stands in for an adapter (and for the native reference when called as "sexp"/"edit-sexp") so the
// orchestrator's tests exercise the real process protocol without any candidate library. Test support only.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wotjr1649/go-treesitter/tools/compare/harness"
)

type doc struct{ n int }

type sess struct {
	harness.Base
	name string
}

func (s sess) Identity() map[string]string { return map[string]string{"fake": s.name} }
func (s sess) Parse(src []byte) (harness.Tree, error) {
	if s.name == "bonsai" {
		return nil, harness.ErrUnsupported
	}
	return &doc{len(src)}, nil
}
func (s sess) ParseSnapshot(src []byte) (harness.Tree, error) { return &doc{len(src)}, nil }
func (s sess) Edit(harness.Tree, harness.Edit) error {
	if s.name == "bonsai" || s.name == "malivvan" {
		return harness.ErrUnsupported
	}
	return nil
}
func (s sess) Reparse(src []byte, _ harness.Tree) (harness.Tree, error) { return &doc{len(src)}, nil }
func (s sess) Sexp(t harness.Tree) (string, bool, error) {
	n := t.(*doc).n
	if s.name == "smacker" { // a deliberately different candidate
		return fmt.Sprintf("(source_file (extra) (len_%d))", n), false, nil
	}
	return fmt.Sprintf("(source_file (len_%d))", n), false, nil
}

func main() {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	if len(os.Args) > 2 && (os.Args[1] == "sexp" || os.Args[1] == "edit-sexp") {
		file := os.Args[2]
		if os.Args[1] == "edit-sexp" {
			file = os.Args[3]
		}
		b, err := os.ReadFile(file)
		if err != nil {
			os.Exit(3)
		}
		fmt.Printf("#compare-sexp/1 has_error=0\n(source_file (len_%d))", len(b))
		return
	}
	harness.Main(name, func(lang string) (harness.Session, error) {
		if lang == "json" && (name == "smacker" || name == "bonsai") {
			return nil, harness.ErrUnsupported
		}
		return sess{name: name}, nil
	})
}
