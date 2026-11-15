#include <stdio.h>
#include TSG_FIELD_PLAN_SHIM

static const TSFieldMapEntry entries[] = {
  {1, 1, false}, {1, 3, false},
  {1, 3, false}, {1, 1, false},
  {1, 1, false}, {2, 1, false},
  {1, 1, true},
  {1, 3, false}, {2, 1, false},
  {1, 1, false}, {1, 1, false},
};
static const TSMapSlice slices[] = {
  {0, 0}, {0, 2}, {2, 2}, {4, 2}, {6, 1}, {7, 2}, {9, 2},
};
static const TSLanguage language = {
  .abi_version = 15,
  .production_id_count = 7,
  .field_count = 2,
  .field_map_slices = slices,
  .field_map_entries = entries,
};
const TSLanguage *tsg_test_language(void) { return &language; }
int32_t tsg_host_should_stop(void) { return 0; }

int main(void) {
  static const uint16_t expected[] = {0, 4, UINT16_MAX, UINT16_MAX, UINT16_MAX, 4, 2};
  static const char *names[] = {
    "empty", "ascending repeated field", "descending same-field", "collision", "inherited",
    "descending distinct fields", "equal repeated field",
  };
  xfields_init();
  for (unsigned i = 0; i < sizeof(expected) / sizeof(expected[0]); i++) {
    if (x_field_plans[i] != expected[i]) {
      fprintf(stderr, "field plan mismatch: %s: got %u, expected %u\n", names[i], x_field_plans[i], expected[i]);
      return 1;
    }
  }
  free(x_field_plans);
  free(x_direct_fields);
  return 0;
}
