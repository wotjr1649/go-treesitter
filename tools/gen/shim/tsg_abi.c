// tsg_abi.c - the C side of engine ABI v3 (docs/design/engine-abi.md). This repository's own code; upstream files are
// compiled unmodified next to it. One wasm instance = one TSParser; trees are opaque TSTree* handles (wasm32 offsets).
// Build: clang --target=wasm32-wasip1 -mexec-model=reactor -DTSG_LANG_FN=<tree_sitter_xxx> (see tools/gen).
// Any change to an export signature or semantics, the chunk layout or the meta block bumps TSG_ABI_VERSION and
// internal/engine.ABIVersion.
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include "tree_sitter/api.h"
#include "language.h"
#include "subtree.h"

#define TSG_ABI_VERSION 3u

#ifndef TSG_LANG_FN
#error "define TSG_LANG_FN"
#endif
const TSLanguage *TSG_LANG_FN(void);

__attribute__((import_module("tsg"), import_name("should_stop"))) int32_t tsg_host_should_stop(void);

static TSParser *g_p;
// 0 = no fields, UINT16_MAX = compare, otherwise minimum structural child count for direct fields.
static uint16_t *x_field_plans;
static TSNode *x_direct_fields;

static void xfields_init(void) {
  if (x_field_plans) return;
  const TSLanguage *lang = TSG_LANG_FN();
  x_field_plans = calloc(lang->production_id_count, sizeof(uint16_t));
  x_direct_fields = calloc(lang->field_count + 1, sizeof(TSNode));
  if (!x_field_plans || !x_direct_fields) abort();
  for (uint32_t i = 0; i < lang->production_id_count; i++) {
    const TSFieldMapEntry *field, *end;
    ts_language_field_map(lang, i, &field, &end);
    uint16_t plan = 0;
    for (const TSFieldMapEntry *f = field; f != end; f++) {
      if (f->inherited) { plan = UINT16_MAX; break; }
      for (const TSFieldMapEntry *other = field; other != f; other++) {
        if ((other->child_index == f->child_index && other->field_id != f->field_id) ||
            (other->field_id == f->field_id && other->child_index > f->child_index)) {
          plan = UINT16_MAX; break;
        }
      }
      if (plan == UINT16_MAX) break;
      if (plan <= f->child_index) plan = (uint16_t)f->child_index + 1;
    }
    x_field_plans[i] = plan;
  }
}

// export chunk buffer (section "chunked preorder export"); allocated and its header written by tsg_init
#define KC 16384u
#define XHDR 16u
static uint8_t *x_buf;

static int xbuf_alloc(void) {
  if (x_buf) return 1;
  x_buf = (uint8_t *)malloc(XHDR + 33u * KC);
  if (!x_buf) return 0;
  memcpy(x_buf, "TSGX", 4);
  uint16_t *h16 = (uint16_t *)(x_buf + 4);
  h16[0] = 2; h16[1] = 0;
  uint32_t *h32 = (uint32_t *)(x_buf + 8);
  h32[0] = KC; h32[1] = 0;
  return 1;
}

uint32_t tsg_abi_version(void) { return TSG_ABI_VERSION; }

int32_t tsg_init(void) {
  if (!xbuf_alloc()) return -1;
  if (g_p) return 0;
  g_p = ts_parser_new();
  if (!g_p) return -1;
  if (!ts_parser_set_language(g_p, TSG_LANG_FN())) {
    ts_parser_delete(g_p);
    g_p = NULL;
    return -1;
  }
  xfields_init();
  return 0;
}

// ---- parse ----
typedef struct { const char *s; uint32_t n; } Src;

static const char *read_src(void *payload, uint32_t byte, TSPoint pos, uint32_t *bytes_read) {
  (void)pos;
  Src *b = (Src *)payload;
  if (byte >= b->n) { *bytes_read = 0; return ""; }
  *bytes_read = b->n - byte;
  return b->s + byte;
}

static bool progress(TSParseState *st) { (void)st; return tsg_host_should_stop() != 0; }

void tsg_parser_reset(void) { if (g_p) ts_parser_reset(g_p); }

TSTree *tsg_parse(const char *src, uint32_t len, TSTree *old) {
  if (!g_p) return NULL;
  Src b = {src, len};
  TSInput in = {&b, read_src, TSInputEncodingUTF8, NULL};
  TSParseOptions opt = {NULL, progress};
  TSTree *t = ts_parser_parse_with_options(g_p, old, in, opt);
  if (!t) ts_parser_reset(g_p);  // a stopped parse keeps resumable state; the host never resumes
  return t;
}

uint32_t tsg_root_end(TSTree *t) { return ts_node_end_byte(ts_tree_root_node(t)); }
uint32_t tsg_root_has_error(TSTree *t) { return ts_node_has_error(ts_tree_root_node(t)) ? 1u : 0u; }
uint32_t tsg_descendant_count(TSTree *t) { return ts_node_descendant_count(ts_tree_root_node(t)); }
static void qcursor_drop_tree(TSTree *t);
void tsg_tree_delete(TSTree *t) { qcursor_drop_tree(t); ts_tree_delete(t); }

void tsg_tree_edit(TSTree *t, uint32_t start_byte, uint32_t old_end_byte, uint32_t new_end_byte, uint32_t start_row,
                   uint32_t start_col, uint32_t old_end_row, uint32_t old_end_col, uint32_t new_end_row,
                   uint32_t new_end_col) {
  TSInputEdit e = {
    .start_byte = start_byte, .old_end_byte = old_end_byte, .new_end_byte = new_end_byte,
    .start_point = {start_row, start_col}, .old_end_point = {old_end_row, old_end_col},
    .new_end_point = {new_end_row, new_end_col},
  };
  ts_tree_edit(t, &e);
}

// never NULL (count = 0 when nothing changed; an allocation failure aborts = trap). block: u32 count, then count x {u32 start_byte, end_byte, start_row, start_col, end_row, end_col}; free with free().
uint8_t *tsg_changed_ranges(TSTree *old, TSTree *neu) {
  uint32_t n = 0;
  TSRange *r = ts_tree_get_changed_ranges(old, neu, &n);
  uint32_t *blk = (uint32_t *)malloc(4u + 24u * n);
  if (!blk) abort();
  blk[0] = n;
  for (uint32_t i = 0; i < n; i++) {
    uint32_t *o = blk + 1 + 6u * i;
    o[0] = r[i].start_byte; o[1] = r[i].end_byte;
    o[2] = r[i].start_point.row; o[3] = r[i].start_point.column;
    o[4] = r[i].end_point.row; o[5] = r[i].end_point.column;
  }
  free(r);
  return (uint8_t *)blk;
}

// Shared by snapshot field exceptions and query captures; build lazily on the first result needing an index.
typedef struct { const void *id; uint32_t index; } NodeIndexEntry;
typedef struct { NodeIndexEntry *entries; uint32_t capacity; } NodeIndex;

static void node_index_drop(NodeIndex *map) {
  free(map->entries); map->entries = NULL; map->capacity = 0;
}
static NodeIndexEntry *node_index_slot(NodeIndex *map, const void *id) {
  uint32_t hash = (uint32_t)(uintptr_t)id;
  hash ^= hash >> 16; hash *= 0x7feb352du; hash ^= hash >> 15;
  uint32_t slot = hash & (map->capacity - 1);
  while (map->entries[slot].id && map->entries[slot].id != id) slot = (slot + 1) & (map->capacity - 1);
  return &map->entries[slot];
}
static bool node_index_build(NodeIndex *map, TSNode root) {
  uint32_t count = ts_node_descendant_count(root);
  uint64_t capacity = 1;
  while (capacity < 2ull * count) capacity *= 2;
  if (capacity > UINT32_MAX / sizeof(NodeIndexEntry)) abort();
  map->capacity = (uint32_t)capacity;
  map->entries = calloc(map->capacity, sizeof(NodeIndexEntry));
  if (!map->entries) abort();
  TSTreeCursor cur = ts_tree_cursor_new(root);
  uint32_t visited = 0;
  do {
    if ((visited & 255u) == 0 && tsg_host_should_stop()) {
      ts_tree_cursor_delete(&cur); node_index_drop(map); return false;
    }
    TSNode node = ts_tree_cursor_current_node(&cur);
    NodeIndexEntry *entry = node_index_slot(map, node.id);
    if (entry->id) abort();
    *entry = (NodeIndexEntry){node.id, ts_tree_cursor_current_descendant_index(&cur)};
    visited++;
    if (ts_tree_cursor_goto_first_child(&cur)) continue;
    while (!ts_tree_cursor_goto_next_sibling(&cur)) {
      if (!ts_tree_cursor_goto_parent(&cur)) goto done;
    }
  } while (true);
done:
  ts_tree_cursor_delete(&cur);
  if (visited != count) abort();
  return true;
}
static uint32_t node_index_get(NodeIndex *map, TSNode node) {
  NodeIndexEntry *entry = node_index_slot(map, node.id);
  return entry->id ? entry->index : UINT32_MAX;
}

// ---- chunked preorder export (snapshot block format 2) ----
// Header 16 bytes: "TSGX", u16 format=2, u16 0, u32 KC, u32 field-exception block pointer (0 = none). Columns follow at buf+16 (KC = chunk capacity, multiple of 4):
// symbol u16 @0, field u16 @2KC, flags u8 @4KC, startByte @5KC, endByte @9KC, startRow @13KC, startCol @17KC, endRow @21KC,
// endCol @25KC (u32 each), parent i32 @29KC (global preorder index, -1 = root). flags: 1 named, 2 extra, 4 missing, 8 error,
// 16 has_error. symbol 65535 = ERROR. firstChild/nextSibling are not exported; the host derives them from parent.
static TSTreeCursor x_cur;
static int x_live, x_done, x_err;
static int32_t *x_prev;
static uint32_t x_cap, x_i, x_d, x_n;
static TSTree *x_tree;
static NodeIndex x_index;
// u32 count followed by {node index, field id, target index}; UINT32_MAX means an explicit null result.
static uint32_t *x_fields;
static uint32_t x_field_count, x_field_capacity;
static void xfields_end(void) {
  free(x_fields); x_fields = NULL; x_field_count = x_field_capacity = 0;
  if (x_buf) ((uint32_t *)(x_buf + 12))[0] = 0;
}
static void xend(void) {
  if (x_live) { ts_tree_cursor_delete(&x_cur); x_live = 0; }
  free(x_prev); x_prev = NULL;
  node_index_drop(&x_index); x_tree = NULL;
}

void tsg_export_end(void) { xend(); xfields_end(); x_done = 1; }
uint8_t *tsg_export_buf(void) { return x_buf; }
uint32_t tsg_export_err(void) { return (uint32_t)x_err; }

uint32_t tsg_export_begin(TSTree *t) {
  xend(); xfields_end();
  x_done = 0; x_err = 0;
  if (!t) return 0;
  if (!xbuf_alloc()) return 0;
  x_cap = 256;
  x_prev = (int32_t *)malloc(sizeof(int32_t) * x_cap);
  if (!x_prev) abort();
  x_tree = t;
  x_n = ts_node_descendant_count(ts_tree_root_node(t));
  x_cur = ts_tree_cursor_new(ts_tree_root_node(t));
  x_live = 1; x_i = 0; x_d = 0;
  return x_n;
}

static bool xfields_may_differ(Subtree subtree) {
  uint32_t production = ts_subtree_production_id(subtree);
  uint16_t plan = x_field_plans[production];
  if (!plan) return false;
  if (plan == UINT16_MAX) return true;
  const TSSymbol *aliases = ts_language_alias_sequence(TSG_LANG_FN(), production);
  const Subtree *children = ts_subtree_children(subtree);
  uint32_t structural = 0;
  // v0.27.2 node.c selects the first mapped non-extra child; tree_cursor.c selects its sole direct field.
  // Skip only with no inheritance/collisions, all children visible (including aliases), and every mapped index
  // present. Keep ALL hidden children: even non-inherited fields can meet extras or inner fields there.
  for (uint32_t i = 0, n = ts_subtree_child_count(subtree); i < n; i++) {
    bool extra = ts_subtree_extra(children[i]);
    TSSymbol alias = !extra && aliases ? aliases[structural] : 0;
    if (!ts_subtree_visible(children[i]) && !alias) return true;
    if (!extra) structural++;
  }
  return structural < plan;
}

// Cursor fields describe direct visible children; C may instead follow a field through a visible alias.
static bool xfields_node(TSNode node) {
  Subtree subtree = *(const Subtree *)node.id;
  if (ts_subtree_child_count(subtree) == 0 || ts_node_is_error(node)) return true;
  if (!xfields_may_differ(subtree)) return true;
  const TSFieldMapEntry *field, *end;
  ts_language_field_map(TSG_LANG_FN(), ts_subtree_production_id(subtree), &field, &end);
  for (const TSFieldMapEntry *f = field; f != end; f++) x_direct_fields[f->field_id] = (TSNode){0};
  TSTreeCursor children = ts_tree_cursor_new(node);
  if (ts_tree_cursor_goto_first_child(&children)) {
    uint32_t visited = 0;
    do {
      if ((visited++ & 255u) == 0 && tsg_host_should_stop()) {
        ts_tree_cursor_delete(&children); return false;
      }
      TSFieldId id = ts_tree_cursor_current_field_id(&children);
      if (id && !x_direct_fields[id].id) x_direct_fields[id] = ts_tree_cursor_current_node(&children);
    } while (ts_tree_cursor_goto_next_sibling(&children));
  }
  ts_tree_cursor_delete(&children);
  for (TSFieldId previous = 0; field != end; field++) {
    TSFieldId id = field->field_id;
    if (id == previous) continue;
    previous = id;
    TSNode target = ts_node_child_by_field_id(node, id);
    TSNode direct = x_direct_fields[id];
    if (ts_node_eq(target, direct)) continue;
    uint32_t target_index = UINT32_MAX;
    if (target.id) {
      if (!x_index.entries && !node_index_build(&x_index, ts_tree_root_node(x_tree))) return false;
      target_index = node_index_get(&x_index, target);
      if (target_index == UINT32_MAX) abort();
    }
    if (x_field_count == x_field_capacity) {
      uint64_t capacity = x_field_capacity ? 2ull * x_field_capacity : 16;
      if (4ull + 12ull * capacity > UINT32_MAX) abort();
      uint32_t *fields = realloc(x_fields, (uint32_t)(4ull + 12ull * capacity));
      if (!fields) abort();
      x_fields = fields; x_field_capacity = (uint32_t)capacity;
    }
    uint32_t *entry = x_fields + 1 + 3u * x_field_count++;
    entry[0] = x_i; entry[1] = id; entry[2] = target_index;
  }
  return true;
}

uint32_t tsg_export_next(void) {
  x_field_count = 0;
  ((uint32_t *)(x_buf + 12))[0] = 0;
  if (!x_live || x_done) return 0;
  uint8_t *c = x_buf + XHDR;
  uint16_t *symc = (uint16_t *)c, *fldc = (uint16_t *)(c + 2u * KC);
  uint8_t *flg = c + 4u * KC;
  uint32_t *sb = (uint32_t *)(c + 5u * KC), *eb = (uint32_t *)(c + 9u * KC);
  uint32_t *sr = (uint32_t *)(c + 13u * KC), *sc = (uint32_t *)(c + 17u * KC);
  uint32_t *er = (uint32_t *)(c + 21u * KC), *ec = (uint32_t *)(c + 25u * KC);
  int32_t *par = (int32_t *)(c + 29u * KC);
  uint32_t k = 0;
  while (k < KC) {
    if ((x_i & 255u) == 0 && tsg_host_should_stop()) { x_err = 1; x_done = 1; break; }
    if (x_i >= x_n) { x_err = 1; x_done = 1; break; }  // traversal longer than the descendant count
    if (x_d + 1 >= x_cap) {
      if (x_cap > UINT32_MAX / (2u * sizeof(int32_t))) abort();
      x_cap *= 2;
      int32_t *np = (int32_t *)realloc(x_prev, sizeof(int32_t) * x_cap);
      if (!np) abort();
      x_prev = np;
    }
    TSNode nd = ts_tree_cursor_current_node(&x_cur);
    if (!xfields_node(nd)) { x_err = 1; x_done = 1; break; }
    TSPoint a = ts_node_start_point(nd), b = ts_node_end_point(nd);
    symc[k] = ts_node_symbol(nd);
    fldc[k] = ts_tree_cursor_current_field_id(&x_cur);
    flg[k] = (uint8_t)((ts_node_is_named(nd) ? 1 : 0) | (ts_node_is_extra(nd) ? 2 : 0) | (ts_node_is_missing(nd) ? 4 : 0) |
                       (ts_node_is_error(nd) ? 8 : 0) | (ts_node_has_error(nd) ? 16 : 0));
    sb[k] = ts_node_start_byte(nd); eb[k] = ts_node_end_byte(nd);
    sr[k] = a.row; sc[k] = a.column; er[k] = b.row; ec[k] = b.column;
    par[k] = x_d == 0 ? -1 : x_prev[x_d - 1];
    x_prev[x_d] = (int32_t)x_i;
    x_i++; k++;
    if (ts_tree_cursor_goto_first_child(&x_cur)) { x_d++; continue; }
    for (;;) {
      if (ts_tree_cursor_goto_next_sibling(&x_cur)) break;
      if (!ts_tree_cursor_goto_parent(&x_cur)) { x_done = 1; goto out; }
      x_d--;
    }
  }
out:
  if (x_field_count) {
    x_fields[0] = x_field_count;
    ((uint32_t *)(x_buf + 12))[0] = (uint32_t)(uintptr_t)x_fields;
  }
  if (x_done) {
    if (x_i != x_n) x_err = 1;
    xend();
  }
  return k;
}

// ---- language meta block (format 1), read once by the generator ----
// "TSGM", u32 format=1, u32 language_abi, u32 nSym, u32 nField (ids 0..field_count, id 0 = ""), u32 poolBytes;
// symOff u32[nSym+1], fldOff u32[nField+1], symFlags u8[nSym] (1 named, 2 visible, 4 supertype), then the name pool.
uint8_t *tsg_lang_meta(void) {
  const TSLanguage *lang = TSG_LANG_FN();
  uint32_t nSym = ts_language_symbol_count(lang), nField = ts_language_field_count(lang) + 1;
  uint32_t pool = 0;
  for (uint32_t i = 0; i < nSym; i++) { const char *s = ts_language_symbol_name(lang, (TSSymbol)i); pool += s ? (uint32_t)strlen(s) : 0; }
  for (uint32_t i = 1; i < nField; i++) { const char *s = ts_language_field_name_for_id(lang, (TSFieldId)i); pool += s ? (uint32_t)strlen(s) : 0; }
  uint32_t total = 24 + 4 * (nSym + 1) + 4 * (nField + 1) + nSym + pool;
  uint8_t *blk = (uint8_t *)malloc(total);
  if (!blk) return NULL;
  memcpy(blk, "TSGM", 4);
  uint32_t *h = (uint32_t *)(blk + 4);
  h[0] = 1; h[1] = ts_language_abi_version(lang); h[2] = nSym; h[3] = nField; h[4] = pool;
  uint32_t *symOff = (uint32_t *)(blk + 24), *fldOff = symOff + nSym + 1;
  uint8_t *flags = (uint8_t *)(fldOff + nField + 1);
  char *poolp = (char *)(flags + nSym);
  uint32_t pp = 0;
  for (uint32_t k = 0; k < nSym; k++) {
    symOff[k] = pp;
    const char *s = ts_language_symbol_name(lang, (TSSymbol)k);
    uint32_t l = s ? (uint32_t)strlen(s) : 0;
    memcpy(poolp + pp, s ? s : "", l); pp += l;
    TSSymbolType ty = ts_language_symbol_type(lang, (TSSymbol)k);
    flags[k] = ty == TSSymbolTypeRegular ? 3 : ty == TSSymbolTypeAnonymous ? 2 : ty == TSSymbolTypeSupertype ? 7 : 0;
  }
  symOff[nSym] = pp;
  fldOff[0] = pp;
  for (uint32_t k = 1; k < nField; k++) {
    fldOff[k] = pp;
    const char *s = ts_language_field_name_for_id(lang, (TSFieldId)k);
    uint32_t l = s ? (uint32_t)strlen(s) : 0;
    memcpy(poolp + pp, s ? s : "", l); pp += l;
  }
  fldOff[nField] = pp;
  return blk;
}

// ---- queries (ABI 2) ----
static uint32_t q_error_offset;
static TSQueryError q_error_type;
TSQuery *tsg_query_new(const char *src, uint32_t len) {
  q_error_offset = 0; q_error_type = TSQueryErrorNone;
  return ts_query_new(TSG_LANG_FN(), src, len, &q_error_offset, &q_error_type);
}
uint32_t tsg_query_error_offset(void) { return q_error_offset; }
uint32_t tsg_query_error_type(void) { return (uint32_t)q_error_type; }
void tsg_query_delete(TSQuery *q) { ts_query_delete(q); }

// Header: TSGQ, u32 format, total bytes, patterns, captures, strings. Length-prefixed UTF-8 strings
// (capture names then literals), then per-pattern start byte, step count, {type,value} predicate steps.
uint8_t *tsg_query_meta(TSQuery *q) {
  uint32_t np = ts_query_pattern_count(q), nc = ts_query_capture_count(q), ns = ts_query_string_count(q);
  uint64_t size = 24;
  for (uint32_t i = 0, n; i < nc; i++) { ts_query_capture_name_for_id(q, i, &n); size += 4ull + n; }
  for (uint32_t i = 0, n; i < ns; i++) { ts_query_string_value_for_id(q, i, &n); size += 4ull + n; }
  for (uint32_t i = 0, n; i < np; i++) { ts_query_predicates_for_pattern(q, i, &n); size += 8ull + 8ull * n; }
  if (size > UINT32_MAX) abort();
  uint8_t *b = malloc((uint32_t)size);
  if (!b) abort();
  memcpy(b, "TSGQ", 4);
  uint32_t h[] = {1, (uint32_t)size, np, nc, ns};
  memcpy(b + 4, h, sizeof(h));
  uint32_t pos = 24;
  for (uint32_t kind = 0; kind < 2; kind++) {
    uint32_t count = kind ? ns : nc;
    for (uint32_t i = 0; i < count; i++) {
      uint32_t n;
      const char *s = kind ? ts_query_string_value_for_id(q, i, &n) : ts_query_capture_name_for_id(q, i, &n);
      memcpy(b + pos, &n, 4); pos += 4;
      memcpy(b + pos, s, n); pos += n;
    }
  }
  for (uint32_t i = 0; i < np; i++) {
    uint32_t n, start = ts_query_start_byte_for_pattern(q, i);
    const TSQueryPredicateStep *steps = ts_query_predicates_for_pattern(q, i, &n);
    memcpy(b + pos, &start, 4); memcpy(b + pos + 4, &n, 4); pos += 8;
    for (uint32_t j = 0; j < n; j++) {
      uint32_t pair[] = {(uint32_t)steps[j].type, steps[j].value_id};
      memcpy(b + pos, pair, 8); pos += 8;
    }
  }
  return b;
}

typedef struct TSGQCursor {
  TSQueryCursor *cursor;
  TSTree *tree;
  TSQueryCursorOptions options;
  uint8_t *buf;
  uint32_t bytes;
  NodeIndex index;
  bool halted;
  struct TSGQCursor *next;
} TSGQCursor;

static TSGQCursor *q_cursors;
static void qcursor_drop_index(TSGQCursor *c) {
  node_index_drop(&c->index);
}
static void qcursor_drop_tree(TSTree *t) {
  for (TSGQCursor *c = q_cursors; c; c = c->next) {
    if (c->tree == t) { qcursor_drop_index(c); c->tree = NULL; }
  }
}
TSGQCursor *tsg_qcursor_new(void) {
  TSGQCursor *c = calloc(1, sizeof(TSGQCursor));
  if (!c) abort();
  c->cursor = ts_query_cursor_new();
  if (!c->cursor) abort();
  c->next = q_cursors; q_cursors = c;
  return c;
}
void tsg_qcursor_delete(TSGQCursor *c) {
  if (!c) return;
  TSGQCursor **link = &q_cursors;
  while (*link && *link != c) link = &(*link)->next;
  if (*link) *link = c->next;
  ts_query_cursor_delete(c->cursor); node_index_drop(&c->index); free(c->buf); free(c);
}
static bool qprogress(TSQueryCursorState *state) {
  TSGQCursor *c = state->payload;
  if (tsg_host_should_stop()) { c->halted = true; return true; }
  return false;
}
int32_t tsg_qcursor_exec(TSGQCursor *c, TSQuery *q, TSTree *t, uint32_t start, uint32_t end, uint32_t limit) {
  if (c->tree != t) qcursor_drop_index(c);
  c->tree = t; c->halted = false;
  c->options = (TSQueryCursorOptions){c, qprogress};
  ts_query_cursor_set_match_limit(c->cursor, limit ? limit : UINT32_MAX);
  if (!ts_query_cursor_set_byte_range(c->cursor, start, end)) return -1;
  ts_query_cursor_exec_with_options(c->cursor, q, ts_tree_root_node(t), &c->options);
  return 0;
}
uint32_t tsg_qcursor_next(TSGQCursor *c) {
  TSQueryMatch m;
  if (!ts_query_cursor_next_match(c->cursor, &m)) return 0;
  if (!c->index.entries && !node_index_build(&c->index, ts_tree_root_node(c->tree))) {
    c->halted = true; return 0;
  }
  uint32_t bytes = 24u + 20u * m.capture_count;
  if (bytes > c->bytes) {
    uint8_t *b = realloc(c->buf, bytes);
    if (!b) abort();
    c->buf = b; c->bytes = bytes;
  }
  memcpy(c->buf, "TSGM", 4);
  uint32_t h[] = {1, 1, bytes - 16, m.pattern_index, m.capture_count};
  memcpy(c->buf + 4, h, sizeof(h));
  for (uint32_t i = 0; i < m.capture_count; i++) {
    TSNode n = m.captures[i].node;
    uint32_t cap[] = {m.captures[i].index, node_index_get(&c->index, n), ts_node_symbol(n), ts_node_start_byte(n), ts_node_end_byte(n)};
    memcpy(c->buf + 24 + 20u * i, cap, sizeof(cap));
  }
  return 1;
}
uint32_t tsg_qcursor_halted(TSGQCursor *c) { return c->halted; }
uint32_t tsg_qcursor_exceeded(TSGQCursor *c) { return ts_query_cursor_did_exceed_match_limit(c->cursor); }
uint8_t *tsg_qcursor_buf(TSGQCursor *c) { return c->buf; }
