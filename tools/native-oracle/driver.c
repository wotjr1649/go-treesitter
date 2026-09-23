/* One executable per grammar; stdin is bounded UTF-8, stdout is an ordered tree. */
#include <stdio.h>
#include <stdlib.h>
#include <errno.h>
#include <string.h>
#include "tree_sitter/api.h"
#ifdef _WIN32
#include <fcntl.h>
#include <io.h>
#endif

#ifndef ORACLE_LANGUAGE
#error ORACLE_LANGUAGE must name the grammar entry point
#endif
const TSLanguage *ORACLE_LANGUAGE(void);

static void quoted(const char *s) {
    putchar('"');
    for (const unsigned char *p = (const unsigned char *)s; *p; p++) {
        if (*p == '"' || *p == '\\') printf("\\%c", *p);
        else if (*p < 32) printf("\\u%04x", *p);
        else putchar(*p);
    }
    putchar('"');
}

static int snapshot(TSNode root) {
    TSTreeCursor cursor = ts_tree_cursor_new(root);
    unsigned parents[4096], depth = 0, count = 0;
    for (;;) {
        if (count == 1000000) { ts_tree_cursor_delete(&cursor); return 0; }
        TSNode n = ts_tree_cursor_current_node(&cursor);
        const char *field = ts_tree_cursor_current_field_name(&cursor);
        TSPoint a = ts_node_start_point(n), b = ts_node_end_point(n);
        if (count) putchar(',');
        fputs("{\"Type\":", stdout); quoted(ts_node_type(n));
        fputs(",\"Field\":", stdout); quoted(field ? field : "");
        printf(",\"Parent\":%d,\"Named\":%s,\"Extra\":%s,\"Missing\":%s,\"Error\":%s,"
               "\"StartByte\":%u,\"EndByte\":%u,\"StartPoint\":{\"Row\":%u,\"Column\":%u},"
               "\"EndPoint\":{\"Row\":%u,\"Column\":%u}}",
               depth ? (int)parents[depth - 1] : -1,
               ts_node_is_named(n) ? "true" : "false", ts_node_is_extra(n) ? "true" : "false",
               ts_node_is_missing(n) ? "true" : "false", ts_node_is_error(n) ? "true" : "false",
               ts_node_start_byte(n), ts_node_end_byte(n), a.row, a.column, b.row, b.column);
        unsigned index = count++;
        if (ts_tree_cursor_goto_first_child(&cursor)) {
            if (depth == 4096) { ts_tree_cursor_delete(&cursor); return 0; }
            parents[depth++] = index;
            continue;
        }
        while (!ts_tree_cursor_goto_next_sibling(&cursor)) {
            if (!depth) { ts_tree_cursor_delete(&cursor); return 1; }
            ts_tree_cursor_goto_parent(&cursor);
            depth--;
        }
    }
}

static int offset(const char *text, uint32_t *value) {
    if (!*text) return 0;
    for (const char *p = text; *p; p++) if (*p < '0' || *p > '9') return 0;
    errno = 0;
    char *end;
    unsigned long n = strtoul(text, &end, 10);
    if (errno || *end || n > 4 * 1024 * 1024) return 0;
    *value = (uint32_t)n;
    return 1;
}

static TSPoint point(const char *source, uint32_t end) {
    TSPoint result = {0, 0};
    for (uint32_t i = 0; i < end; i++) {
        if (source[i] == '\n') { result.row++; result.column = 0; }
        else result.column++;
    }
    return result;
}

static int boundary(const char *source, uint32_t length, uint32_t at) {
    return at == length || ((unsigned char)source[at] & 0xc0) != 0x80;
}

int main(int argc, char **argv) {
    /* Optional edit: stdin is old bytes followed by final bytes. */
    uint32_t old_length = 0, start = 0, old_end = 0, new_end = 0;
    if (argc != 1 && (argc != 5 || !offset(argv[1], &old_length) ||
        !offset(argv[2], &start) || !offset(argv[3], &old_end) ||
        !offset(argv[4], &new_end))) return 9;
#ifdef _WIN32
    if (_setmode(_fileno(stdin), _O_BINARY) == -1) return 1;
#endif
    const size_t limit = 4 * 1024 * 1024;
    char *source = malloc(limit + 1);
    if (!source) return 2;
    size_t length = fread(source, 1, limit + 1, stdin);
    if (ferror(stdin) || length > limit) { free(source); return 3; }
    char *final_source = source;
    if (argc == 5) {
        if (old_length > length) { free(source); return 9; }
        length -= old_length;
        final_source += old_length;
        if (start > old_end || old_end > old_length || start > new_end || new_end > length ||
            old_length - old_end != length - new_end ||
            memcmp(source, final_source, start) ||
            memcmp(source + old_end, final_source + new_end, length - new_end) ||
            !boundary(source, old_length, start) || !boundary(source, old_length, old_end) ||
            !boundary(final_source, (uint32_t)length, start) ||
            !boundary(final_source, (uint32_t)length, new_end)) { free(source); return 9; }
    }
    TSParser *parser = ts_parser_new();
    if (!parser) { free(source); return 4; }
    const TSLanguage *language = ORACLE_LANGUAGE();
    if (!ts_parser_set_language(parser, language)) {
        ts_parser_delete(parser); free(source); return 5;
    }
    ts_parser_set_timeout_micros(parser, 10000000);
    TSTree *old = NULL;
    if (argc == 5) {
        old = ts_parser_parse_string(parser, NULL, source, old_length);
        if (!old) { ts_parser_delete(parser); free(source); return 6; }
        TSInputEdit edit = {start, old_end, new_end, point(source, start),
                           point(source, old_end), point(final_source, new_end)};
        ts_tree_edit(old, &edit);
    }
    TSTree *tree = ts_parser_parse_string(parser, old, final_source, (uint32_t)length);
    if (old) ts_tree_delete(old);
    free(source);
    if (!tree) { ts_parser_delete(parser); return 6; }
    TSNode root = ts_tree_root_node(tree);
    if (ts_node_is_null(root)) { ts_tree_delete(tree); ts_parser_delete(parser); return 7; }
    printf("{\"abi\":%u,\"input_bytes\":%zu,\"has_error\":%s,\"start\":%u,\"end\":%u,\"nodes\":[",
           ts_language_abi_version(language), length, ts_node_has_error(root) ? "true" : "false",
           ts_node_start_byte(root), ts_node_end_byte(root));
    int ok = snapshot(root);
    puts("]}");
    ts_tree_delete(tree);
    ts_parser_delete(parser);
    return ok && !ferror(stdout) ? 0 : 8;
}
