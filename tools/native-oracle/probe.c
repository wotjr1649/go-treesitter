#include <stdio.h>
#include <string.h>
#include "tree_sitter/api.h"

const TSLanguage *tree_sitter_tsx(void);

static void quoted(const char *s) {
    putchar('"');
    for (; *s; s++) {
        if (*s == '"' || *s == '\\') putchar('\\');
        if (*s == '\n') fputs("\\n", stdout);
        else if (*s == '\r') fputs("\\r", stdout);
        else if (*s == '\t') fputs("\\t", stdout);
        else putchar(*s);
    }
    putchar('"');
}

static void node(TSNode n, int parent, const char *field, int *count) {
    int index = (*count)++;
    if (index) putchar(',');
    fputs("{\"Type\":", stdout); quoted(ts_node_type(n));
    fputs(",\"Field\":", stdout); quoted(field ? field : "");
    TSPoint a = ts_node_start_point(n), b = ts_node_end_point(n);
    printf(",\"Parent\":%d,\"Named\":%s,\"Extra\":%s,\"Missing\":%s,\"Error\":%s,"
           "\"StartByte\":%u,\"EndByte\":%u,\"StartPoint\":{\"Row\":%u,\"Column\":%u},"
           "\"EndPoint\":{\"Row\":%u,\"Column\":%u}}",
           parent, ts_node_is_named(n) ? "true" : "false",
           ts_node_is_extra(n) ? "true" : "false", ts_node_is_missing(n) ? "true" : "false",
           ts_node_is_error(n) ? "true" : "false", ts_node_start_byte(n), ts_node_end_byte(n),
           a.row, a.column, b.row, b.column);
    for (uint32_t i = 0; i < ts_node_child_count(n); i++)
        node(ts_node_child(n, i), index, ts_node_field_name_for_child(n, i), count);
}

int main(void) {
    const char *sources[] = {
        "const a = <p>Org & Team</p>;", "const b = <p>AT&T</p>;",
        "const c = <p>&</p>;", "const d = <p>a = b</p>;",
        "const e = <code>k=v</code>;", "const f = <p>x &amp; y</p>;",
        "const g = <p>plain</p>;"
    };
    TSParser *parser = ts_parser_new();
    if (!parser || !ts_parser_set_language(parser, tree_sitter_tsx())) return 2;
    for (unsigned i = 0; i < sizeof(sources) / sizeof(sources[0]); i++) {
        TSTree *tree = ts_parser_parse_string(parser, NULL, sources[i], (uint32_t)strlen(sources[i]));
        if (!tree) { ts_parser_delete(parser); return 3; }
        TSNode root = ts_tree_root_node(tree);
        if (ts_node_is_null(root)) { ts_tree_delete(tree); ts_parser_delete(parser); return 4; }
        printf("{\"fixture\":\"F%u\",\"language\":\"tsx\",\"abi\":%u,\"source\":", i + 1, ts_language_abi_version(tree_sitter_tsx()));
        quoted(sources[i]);
        printf(",\"has_error\":%s,\"start\":%u,\"end\":%u,\"sexpr\":",
               ts_node_has_error(root) ? "true" : "false", ts_node_start_byte(root), ts_node_end_byte(root));
        char *sexpr = ts_node_string(root);
        if (!sexpr) { ts_tree_delete(tree); ts_parser_delete(parser); return 5; }
        quoted(sexpr); free(sexpr);
        fputs(",\"nodes\":[", stdout);
        int count = 0; node(root, -1, "", &count);
        puts("]}");
        ts_tree_delete(tree);
    }
    ts_parser_delete(parser);
    return 0;
}
