#!/usr/bin/env bash
# build-ref.sh ADAPTER LANG: native reference of a cgo candidate, from ITS OWN vendored runtime and grammar C with its own
# cgo flags (no -fsigned-char: Go's cgo uses the platform default). Runs as the adapter's user; output /work/ADAPTER/ref.
set -uo pipefail
a=$1; lang=$2; fn=tree_sitter_$lang
modl() { (cd "/src/tools/compare/adapters/$a" && go list -m -f '{{.Dir}}' "$1"); }
drv=/src/tools/compare/native/sexp_ref.c
case $a in
official)
  case $lang in c) g=tree-sitter-c;; go) g=tree-sitter-go;; javascript) g=tree-sitter-javascript;; json) g=tree-sitter-json;; python) g=tree-sitter-python;; esac
  off=$(modl github.com/tree-sitter/go-tree-sitter) && gd=$(modl "github.com/tree-sitter/$g") || exit 1
  srcs=("$off/src/lib.c" "$gd/src/parser.c"); [ -f "$gd/src/scanner.c" ] && srcs+=("$gd/src/scanner.c")
  gcc -std=c11 -O2 -fPIC -D_POSIX_C_SOURCE=200112L -D_DEFAULT_SOURCE -I"$off/include" -I"$off/src" -I"$gd/src" -DTSG_LANG_FN=$fn "${srcs[@]}" "$drv" -o "/work/$a/ref" ;;
smacker)
  case $lang in c) sub=c;; go) sub=golang;; javascript) sub=javascript;; python) sub=python;; *) echo "no $lang package"; exit 2;; esac
  sm=$(modl github.com/smacker/go-tree-sitter) || exit 1
  srcs=(); for f in "$sm"/*.c; do [ "$(basename "$f")" = bindings.c ] || srcs+=("$f"); done
  srcs+=("$sm/$sub/parser.c"); [ -f "$sm/$sub/scanner.c" ] && srcs+=("$sm/$sub/scanner.c")
  gcc -O2 -fPIC -I"$sm" -I"$sm/$sub" -DTSG_API_H='"api.h"' -DTSG_LANG_FN=$fn "${srcs[@]}" "$drv" -o "/work/$a/ref" ;;
*) exit 2 ;;
esac
