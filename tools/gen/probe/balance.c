// TEST ONLY: include the pinned upstream runtime and product shim without changing either file.
#include "lib.c"

static int32_t probe_before_reset = -1;
static uint32_t probe_resets;

static void probe_reset(TSParser *p) {
  probe_before_reset = p->canceled_balancing ? 1 : 0;
  probe_resets++;
  ts_parser_reset(p);
}
#define ts_parser_reset probe_reset
#include "tsg_abi.c"
#undef ts_parser_reset

int32_t tsg_probe_canceled_balancing(void) { return g_p ? (g_p->canceled_balancing ? 1 : 0) : -1; }
int32_t tsg_probe_flag_before_last_reset(void) { return probe_before_reset; }
uint32_t tsg_probe_reset_count(void) { return probe_resets; }
void tsg_probe_clear(void) { probe_before_reset = -1; }

// Negative control: the next parse must expose stale balancing state when this reset is omitted.
TSTree *tsg_probe_parse_noreset(const char *src, uint32_t len) {
  if (!g_p) return NULL;
  Src b = {src, len};
  TSInput in = {&b, read_src, TSInputEncodingUTF8, NULL};
  TSParseOptions opt = {NULL, progress};
  return ts_parser_parse_with_options(g_p, NULL, in, opt);
}
