// Command structliterals fails when an exported struct type whose fields are all exported changes its field list (names, order or type text) between a baseline directory and the head directory. Such a type admits
// unkeyed literals in other packages, which apidiff does not report as incompatible when a field is added or reordered.
//
// (a baseline struct with an unexported field cannot be written as an unkeyed literal elsewhere).
//
// Usage: structliterals <baseline dir> <head dir>
//
// shortcut: field types are compared as source text, not resolved; a type change that keeps its text is left to apidiff.
package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: structliterals <baseline dir> <head dir>")
		os.Exit(2)
	}
	base, err := structs(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	head, err := structs(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	problems := compare(base, head)
	for _, p := range problems {
		fmt.Println(p)
	}
	if len(problems) > 0 {
		os.Exit(1)
	}
}

// structs maps each exported struct type of the package in dir to its field list ("Name Type" per field, in order).
func structs(dir string) (map[string][]string, error) {
	ctx := build.Default
	ctx.CgoEnabled = false
	pkg, err := ctx.ImportDir(dir, 0)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	out := make(map[string][]string)
	for _, name := range pkg.GoFiles {
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			return nil, err
		}
		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts := spec.(*ast.TypeSpec)
				if st, ok := ts.Type.(*ast.StructType); ok && ts.Name.IsExported() && !ts.Assign.IsValid() {
					out[ts.Name.Name] = fieldList(st)
				}
			}
		}
	}
	return out, nil
}

func fieldList(st *ast.StructType) []string {
	var fields []string
	for _, f := range st.Fields.List {
		typ := types.ExprString(f.Type)
		if len(f.Names) == 0 {
			fields = append(fields, embeddedName(f.Type)+" "+typ)
		}
		for _, n := range f.Names {
			fields = append(fields, n.Name+" "+typ)
		}
	}
	return fields
}

// embeddedName is the field name of an embedded type expression (T, *T, pkg.T, T[A]).
func embeddedName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.StarExpr:
		return embeddedName(x.X)
	case *ast.SelectorExpr:
		return x.Sel.Name
	case *ast.IndexExpr:
		return embeddedName(x.X)
	case *ast.IndexListExpr:
		return embeddedName(x.X)
	case *ast.Ident:
		return x.Name
	}
	return types.ExprString(e)
}

// open reports whether every field is exported, so a package other than the defining one can write an unkeyed literal.
func open(fields []string) bool {
	return !slices.ContainsFunc(fields, func(f string) bool { return !token.IsExported(f) })
}

func compare(base, head map[string][]string) []string {
	var problems []string
	for _, name := range slices.Sorted(maps.Keys(base)) {
		got, ok := head[name]
		// A removed or retyped type is apidiff's finding.
		if ok && open(base[name]) && !slices.Equal(base[name], got) {
			problems = append(problems, fmt.Sprintf("%s: fields changed, unkeyed literals break\n\tbaseline: %s\n\thead:     %s", name, strings.Join(base[name], "; "), strings.Join(got, "; ")))
		}
	}
	return problems
}
