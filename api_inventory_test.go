package treesitter_test

import (
	"fmt"
	"go/importer"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestAPIInventory(t *testing.T) {
	const inventoryPath = "docs/design/api-stability.md"
	const root = "github.com/wotjr1649/go-treesitter"
	const match = root + "/experimental/match"

	markdown, err := os.ReadFile(inventoryPath)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]map[string]bool{root: {}, match: {}}
	var section map[string]bool
	for line := range strings.SplitSeq(string(markdown), "\n") {
		if title, ok := strings.CutPrefix(line, "### "); ok {
			section = want[strings.TrimSpace(title)]
		} else if strings.HasPrefix(line, "## ") {
			section = nil
		}
		row, isTableRow := strings.CutPrefix(strings.TrimSpace(line), "|")
		if section == nil || !isTableRow {
			continue
		}
		cell, _, ok := strings.Cut(row, "|")
		cell = strings.TrimSpace(cell)
		if cell == "Identifier" || cell == "---" {
			continue
		}
		name := strings.Trim(cell, "`")
		if !ok || name == "" || section[name] {
			t.Fatalf("%s: invalid or duplicate identifier row %q", inventoryPath, line)
		}
		section[name] = true
	}

	cmd := exec.CommandContext(t.Context(), "go", "list", "-export", "-f", "{{.ImportPath}} {{.Export}}", ".", "./experimental/match")
	output, err := cmd.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list -export: %v\n%s", err, exit.Stderr)
		}
		t.Fatalf("go list -export: %v", err)
	}
	exports := make(map[string]string)
	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		path, file, _ := strings.Cut(line, " ")
		exports[path] = strings.TrimSpace(file)
	}
	imp := importer.ForCompiler(token.NewFileSet(), "gc", func(path string) (io.ReadCloser, error) {
		if exports[path] == "" {
			return nil, fmt.Errorf("go list returned no export data for %s", path)
		}
		return os.Open(exports[path])
	})

	for _, path := range []string{root, match} {
		t.Run(path, func(t *testing.T) {
			pkg, err := imp.Import(path)
			if err != nil {
				t.Fatal(err)
			}
			got := apiIdentifiers(pkg)
			var missing, extra []string
			for name := range got {
				if !want[path][name] {
					missing = append(missing, name)
				}
			}
			for name := range want[path] {
				if !got[name] {
					extra = append(extra, name)
				}
			}
			slices.Sort(missing)
			slices.Sort(extra)
			if len(missing)+len(extra) > 0 {
				t.Errorf("%s: missing from inventory: %v; extra in inventory: %v", inventoryPath, missing, extra)
			}
		})
	}
}

func apiIdentifiers(pkg *types.Package) map[string]bool {
	ids := make(map[string]bool)
	apiTypes := make(map[string]types.Type)
	for _, name := range pkg.Scope().Names() {
		obj := pkg.Scope().Lookup(name)
		if !obj.Exported() {
			continue
		}
		ids[name] = true
		switch obj := obj.(type) {
		case *types.TypeName:
			apiTypes[name] = obj.Type()
		case *types.Var:
			// A variable can expose members of an otherwise private type, such as *sentinel.
			typ := types.Unalias(obj.Type())
			if ptr, ok := typ.(*types.Pointer); ok {
				typ = types.Unalias(ptr.Elem())
			}
			if named, ok := typ.(*types.Named); ok && named.Obj().Pkg() == pkg && !named.Obj().Exported() {
				apiTypes[named.Obj().Name()] = named
			}
		}
	}
	for name, typ := range apiTypes {
		if fields, ok := typ.Underlying().(*types.Struct); ok {
			for i := range fields.NumFields() {
				if field := fields.Field(i); field.Exported() {
					ids[name+"."+field.Name()] = true
				}
			}
		}
		for _, receiver := range []types.Type{typ, types.NewPointer(typ)} {
			for method := range types.NewMethodSet(receiver).Methods() {
				if method.Obj().Exported() {
					ids[name+"."+method.Obj().Name()] = true
				}
			}
		}
	}
	return ids
}
