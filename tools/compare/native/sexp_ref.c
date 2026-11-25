/* sexp_ref.c - the comparison's native reference driver (this repository's own code; upstream sources are never edited).
 *
 * Linked with one tree-sitter runtime and ONE grammar (TSG_LANG_FN names its entry function):
 *   <exe> sexp FILE                          fresh parse of FILE
 *   <exe> edit-sexp OLD NEW START OE NE      NEW reparsed incrementally from the OLD tree edited by (START, OE, NE)
 * Output (stdout, binary): "#compare-sexp/1 has_error=<0|1>\n" followed by ts_node_string of the root, no trailing newline.
 * Points are byte columns with '\n' rows, as treesitter.EditFor and harness.MakeEdit compute them.
 * Exit: 0 ok, 2 usage, 3 cannot read input, 4 parse failed.
 *
 * -DTSG_API_H='"api.h"' selects a runtime that vendors a flat layout (smacker); the default is <tree_sitter/api.h>.
 * It is the same driver for the product's native-ref build (-driver), the official binding's vendored runtime and
 * smacker's, so every reference prints ts_node_string the same way. Build flags (signed char, NDEBUG) come from the caller.
 */
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#ifdef TSG_API_H
#include TSG_API_H
#else
#include <tree_sitter/api.h>
#endif
#ifdef _WIN32
#include <fcntl.h>
#include <io.h>
#endif

#ifndef TSG_LANG_FN
#error "define TSG_LANG_FN, e.g. -DTSG_LANG_FN=tree_sitter_json"
#endif
const TSLanguage *TSG_LANG_FN(void);

static char *slurp(const char *path, uint32_t *n) {
  FILE *f = fopen(path, "rb");
  if (!f) return NULL;
  size_t cap = 1 << 16, len = 0;
  char *b = malloc(cap);
  size_t r;
  while (b && (r = fread(b + len, 1, cap - len, f)) > 0) {
    len += r;
    if (len == cap) { cap *= 2; b = realloc(b, cap); }
  }
  fclose(f);
  if (!b) return NULL;
  *n = (uint32_t)len;
  return b;
}

static TSPoint point_at(const char *b, uint32_t off) {
  TSPoint p = {0, 0};
  for (uint32_t i = 0; i < off; i++) {
    if (b[i] == '\n') { p.row++; p.column = 0; } else { p.column++; }
  }
  return p;
}

static int emit(TSTree *t) {
  TSNode root = ts_tree_root_node(t);
  char *s = ts_node_string(root);
  printf("#compare-sexp/1 has_error=%d\n", ts_node_has_error(root) ? 1 : 0);
  fputs(s, stdout);
  free(s);
  return fflush(stdout) == 0 ? 0 : 5;
}

int main(int argc, char **argv) {
#ifdef _WIN32
  _setmode(_fileno(stdout), _O_BINARY);
#endif
  const char *cmd = argc > 1 ? argv[1] : "";
  int is_edit = !strcmp(cmd, "edit-sexp");
  if (!((argc == 3 && !strcmp(cmd, "sexp")) || (argc == 7 && is_edit))) {
    fprintf(stderr, "usage: %s sexp FILE | edit-sexp OLD NEW START OLD_END NEW_END\n", argv[0]);
    return 2;
  }
  uint32_t n1 = 0, n2 = 0;
  char *a = slurp(argv[2], &n1), *b = NULL;
  if (is_edit) b = slurp(argv[3], &n2);
  if (!a || (is_edit && !b)) return 3;
  TSParser *p = ts_parser_new();
  if (!ts_parser_set_language(p, TSG_LANG_FN())) return 4;
  TSTree *t = ts_parser_parse_string(p, NULL, a, n1);
  if (!t) return 4;
  if (is_edit) {
    uint32_t start = (uint32_t)strtoul(argv[4], 0, 10), oe = (uint32_t)strtoul(argv[5], 0, 10), ne = (uint32_t)strtoul(argv[6], 0, 10);
    if (start > oe || oe > n1 || start > ne || ne > n2) return 2;
    TSInputEdit e = {start, oe, ne, point_at(a, start), point_at(a, oe), point_at(b, ne)};
    ts_tree_edit(t, &e);
    TSTree *t2 = ts_parser_parse_string(p, t, b, n2);
    ts_tree_delete(t);
    if (!t2) return 4;
    t = t2;
  }
  int rc = emit(t);
  ts_tree_delete(t);
  ts_parser_delete(p);
  return rc;
}
