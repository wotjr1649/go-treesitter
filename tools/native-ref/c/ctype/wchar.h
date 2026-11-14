#ifndef TSG_PRIVATE_WCHAR_H
#define TSG_PRIVATE_WCHAR_H
#include <wctype.h>
#define wchar_t tsg_wchar_t
typedef uint32_t wchar_t;
/* musl iswspace must search 32-bit code points even on Windows. */
static inline const wchar_t *wcschr(const wchar_t *s, wchar_t c) {
    for (; *s && *s != c; ++s) {}
    return *s == c ? s : 0;
}
#endif
