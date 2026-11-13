/* native_ref.c - the native C reference driver (this repository's own code; upstream sources are never edited).
 *
 * Linked with the unmodified tree-sitter runtime (lib/src/lib.c) and ONE grammar; TSG_LANG_FN names its entry function.
 *   <exe> dump <file>                                  canonical tree dump of a fresh parse (stdout, binary)
 *   <exe> edit <old> <new> <start> <old_end> <new_end> dump of the new tree reparsed incrementally from the edited old tree
 *   <exe> changed-ranges <old> <new> <start> <old_end> <new_end>
 *                                                      ts_tree_get_changed_ranges(old, new), one range per line:
 *                                                      "start_byte end_byte start_row start_col end_row end_col"
 * Points are byte columns with '\n' rows, the same as treesitter.EditFor. Exit: 0 ok, 2 usage, 3 cannot read input,
 * 4 parse failed.
 *
 * Dump: header "#tsg-dump/1 root_has_error=<0|1> nodes=<n>\n", then one line per node in TSTreeCursor preorder, fields
 * separated by '\t': depth field type named extra missing is_error has_error start_byte end_byte
 * start_row:start_col end_row:end_col ("-" when the cursor reports no field name).
 */
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#ifdef _WIN32
#include <fcntl.h>
#include <io.h>
#endif
#include "tree_sitter/api.h"

#ifndef TSG_LANG_FN
#error "define TSG_LANG_FN, e.g. -DTSG_LANG_FN=tree_sitter_json"
#endif
const TSLanguage *TSG_LANG_FN(void);

typedef struct { char *p; uint32_t len, cap; } Buf;

static void put(Buf *b, const char *s, uint32_t n) {
  if (b->len + n + 1 > b->cap) {
    uint32_t c = b->cap ? b->cap : 4096;
    while (b->len + n + 1 > c) c *= 2;
    b->p = realloc(b->p, c);
    if (!b->p) abort();
    b->cap = c;
  }
  memcpy(b->p + b->len, s, n);
  b->len += n;
  b->p[b->len] = 0;
}
static void str(Buf *b, const char *s) { put(b, s, (uint32_t)strlen(s)); }
static void u32(Buf *b, uint32_t v) { char t[16]; int n = snprintf(t, sizeof t, "%u", v); put(b, t, (uint32_t)n); }

static Buf dump_tree(TSTree *tree) {
  Buf body = {0}, out = {0};
  uint32_t count = 0, depth = 0;
  TSTreeCursor cur = ts_tree_cursor_new(ts_tree_root_node(tree));
  for (;;) {
    TSNode n = ts_tree_cursor_current_node(&cur);
    const char *field = ts_tree_cursor_current_field_name(&cur);
    TSPoint sp = ts_node_start_point(n), ep = ts_node_end_point(n);
    u32(&body, depth); str(&body, "\t");
    str(&body, field ? field : "-"); str(&body, "\t");
    str(&body, ts_node_type(n)); str(&body, "\t");
    str(&body, ts_node_is_named(n) ? "1" : "0"); str(&body, "\t");
    str(&body, ts_node_is_extra(n) ? "1" : "0"); str(&body, "\t");
    str(&body, ts_node_is_missing(n) ? "1" : "0"); str(&body, "\t");
    str(&body, ts_node_is_error(n) ? "1" : "0"); str(&body, "\t");
    str(&body, ts_node_has_error(n) ? "1" : "0"); str(&body, "\t");
    u32(&body, ts_node_start_byte(n)); str(&body, "\t");
    u32(&body, ts_node_end_byte(n)); str(&body, "\t");
    u32(&body, sp.row); str(&body, ":"); u32(&body, sp.column); str(&body, "\t");
    u32(&body, ep.row); str(&body, ":"); u32(&body, ep.column); str(&body, "\n");
    count++;
    if (ts_tree_cursor_goto_first_child(&cur)) { depth++; continue; }
    for (;;) {
      if (ts_tree_cursor_goto_next_sibling(&cur)) break;
      if (!ts_tree_cursor_goto_parent(&cur)) goto done;
      depth--;
    }
  }
done:;
  int root_err = ts_node_has_error(ts_tree_root_node(tree));
  ts_tree_cursor_delete(&cur);
  str(&out, "#tsg-dump/1 root_has_error=");
  str(&out, root_err ? "1" : "0");
  str(&out, " nodes=");
  u32(&out, count);
  str(&out, "\n");
  if (body.len) put(&out, body.p, body.len);
  free(body.p);
  return out;
}

static char *slurp(const char *path, uint32_t *n) {
  FILE *f = fopen(path, "rb");
  if (!f) return NULL;
  size_t cap = 1 << 16, len = 0;
  char *buf = malloc(cap);
  if (!buf) return NULL;
  for (;;) {
    len += fread(buf + len, 1, cap - len, f);
    if (len < cap) break;
    cap *= 2;
    buf = realloc(buf, cap);
    if (!buf) return NULL;
  }
  fclose(f);
  *n = (uint32_t)len;
  return buf;
}

static TSPoint point_at(const char *s, uint32_t off) {
  TSPoint p = {0, 0};
  for (uint32_t i = 0; i < off; i++) {
    if (s[i] == '\n') { p.row++; p.column = 0; } else { p.column++; }
  }
  return p;
}

static TSParser *new_parser(void) {
  TSParser *p = ts_parser_new();
  if (p && !ts_parser_set_language(p, TSG_LANG_FN())) { ts_parser_delete(p); p = NULL; }
  return p;
}

static int emit(Buf b) {
  if (fwrite(b.p, 1, b.len, stdout) != b.len) return 5;
  fflush(stdout);
  return 0;
}

int main(int argc, char **argv) {
#ifdef _WIN32
  _setmode(_fileno(stdout), _O_BINARY);
#endif
  const char *cmd = argc > 1 ? argv[1] : "";
  int edit = !strcmp(cmd, "edit"), cr = !strcmp(cmd, "changed-ranges");
  if (!((argc == 3 && !strcmp(cmd, "dump")) || (argc == 7 && (edit || cr)))) {
    fputs("usage: <exe> dump <file> | edit|changed-ranges <old> <new> <start> <old_end> <new_end>\n", stderr);
    return 2;
  }
  uint32_t n1, n2 = 0;
  char *a = slurp(argv[2], &n1), *b = NULL;
  if (argc == 7) b = slurp(argv[3], &n2);
  if (!a || (argc == 7 && !b)) { fputs("open failed\n", stderr); return 3; }
  TSParser *p = new_parser();
  if (!p) { fputs("set language failed\n", stderr); return 4; }
  TSTree *t0 = ts_parser_parse_string(p, NULL, a, n1);
  if (!t0) { fputs("parse failed\n", stderr); return 4; }
  if (argc == 3) {
    Buf out = dump_tree(t0);
    ts_tree_delete(t0);
    ts_parser_delete(p);
    return emit(out);
  }
  uint32_t start = (uint32_t)strtoul(argv[4], 0, 10), oe = (uint32_t)strtoul(argv[5], 0, 10),
           ne = (uint32_t)strtoul(argv[6], 0, 10);
  if (start > oe || oe > n1 || start > ne || ne > n2) { fputs("edit out of range\n", stderr); return 2; }
  TSInputEdit e = {.start_byte = start, .old_end_byte = oe, .new_end_byte = ne, .start_point = point_at(a, start),
                   .old_end_point = point_at(a, oe), .new_end_point = point_at(b, ne)};
  ts_tree_edit(t0, &e);
  TSTree *t1 = ts_parser_parse_string(p, t0, b, n2);
  if (!t1) { fputs("reparse failed\n", stderr); return 4; }
  Buf out = {0};
  if (edit) {
    out = dump_tree(t1);
  } else {
    uint32_t count = 0;
    TSRange *r = ts_tree_get_changed_ranges(t0, t1, &count);
    for (uint32_t i = 0; i < count; i++) {
      u32(&out, r[i].start_byte); str(&out, " "); u32(&out, r[i].end_byte); str(&out, " ");
      u32(&out, r[i].start_point.row); str(&out, " "); u32(&out, r[i].start_point.column); str(&out, " ");
      u32(&out, r[i].end_point.row); str(&out, " "); u32(&out, r[i].end_point.column); str(&out, "\n");
    }
    free(r);
    if (!out.p) put(&out, "", 0); /* zero ranges: empty output */
  }
  ts_tree_delete(t1);
  ts_tree_delete(t0);
  ts_parser_delete(p);
  return emit(out);
}
