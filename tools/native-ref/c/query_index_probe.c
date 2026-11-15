// Include the product shim unchanged so this test needs no grammar-module regeneration.
#include <stdio.h>
#include TSG_INDEX_SHIM

int32_t tsg_host_should_stop(void) { return 0; }

int main(void) {
  if (tsg_init() != 0) return 1;
  TSTree *first = tsg_parse("[1,2]", 5, NULL);
  TSTree *other = tsg_parse("[3]", 3, NULL);
  if (!first || !other) return 2;
  TSGQCursor *a = tsg_qcursor_new(), *b = tsg_qcursor_new(), *c = tsg_qcursor_new();
  a->tree = b->tree = first;
  c->tree = other;
  if (!node_index_build(&a->index, ts_tree_root_node(a->tree)) ||
      !node_index_build(&b->index, ts_tree_root_node(b->tree)) ||
      !node_index_build(&c->index, ts_tree_root_node(c->tree))) return 3;
  NodeIndexEntry *other_nodes = c->index.entries;
  uint32_t other_capacity = c->index.capacity;
  tsg_tree_delete(first);
  // Directly observe both matching cursors; allocator address reuse cannot affect this assertion.
  if (a->tree || a->index.entries || a->index.capacity || b->tree || b->index.entries || b->index.capacity) {
    puts("cached query index survived tree deletion");
    return 4;
  }
  if (c->tree != other || c->index.entries != other_nodes || c->index.capacity != other_capacity) return 5;
  tsg_tree_delete(other);
  if (c->tree || c->index.entries || c->index.capacity) return 6;
  tsg_qcursor_delete(a);
  tsg_qcursor_delete(b);
  tsg_qcursor_delete(c);
  if (q_cursors) return 7;
  ts_parser_delete(g_p);
  free(x_buf);
  return 0;
}
