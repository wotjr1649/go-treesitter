package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// TEST ONLY (-field-full-compare): the shim's xfields_may_differ pre-check skips the C comparison for productions it
// judges safe (tsg_abi.c; it encodes the 0.27.2 node.c/tree_cursor.c algorithms). This variant removes that skip so
// every node with children is compared with ts_node_child_by_field_id. tools/gen/field-precheck-soundness.sh builds the
// variant next to the product module and compares the two modules' field results: any difference means the pre-check
// skipped a place where C differs. The product shim stays byte-identical (shimSHA256 and every module identity
// unchanged); only a copy in the work directory is edited, and exactly one edit is allowed.
const fullCompareShimName = "tsg_abi_full_compare.c"

const fullCompareGuard = "  if (!xfields_may_differ(subtree)) return true;\n"

func fullCompareShim(o opts) (string, error) {
	src, err := os.ReadFile(filepath.Join(o.root, "shim", "tsg_abi.c"))
	if err != nil {
		return "", err
	}
	if bytes.Count(src, []byte(fullCompareGuard)) != 1 {
		return "", fmt.Errorf("-field-full-compare: tsg_abi.c must contain the pre-check call exactly once")
	}
	out := bytes.Replace(src, []byte(fullCompareGuard), []byte("  /* TSG_FIELDS_FULL_COMPARE: pre-check removed */\n"), 1)
	p := filepath.Join(o.work, fullCompareShimName)
	return p, os.WriteFile(p, out, 0o644)
}

func fieldFullCompareFiles(o opts) map[string][]byte {
	repo, _ := filepath.Abs(o.repo)
	mod := fmt.Sprintf("module %s%s\n\ngo 1.26.0\n\nrequire github.com/wotjr1649/go-treesitter v0.0.0\n\nreplace github.com/wotjr1649/go-treesitter => %q\n",
		modulePrefix, o.lang, filepath.ToSlash(repo))
	return map[string][]byte{
		"go.mod":    []byte(mod),
		"README.md": []byte("# TEST ONLY: field pre-check soundness variant\n\nGenerated with tools/gen -field-full-compare. Never publish or use as a product grammar.\nSame module path as the product module so tools/conformance can be built against it with go test -modfile.\n"),
	}
}
