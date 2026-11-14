#ifndef TSG_SHIM_WCTYPE_H
#define TSG_SHIM_WCTYPE_H
#include_next <wctype.h>
#include <stdint.h>
int tsg_iswalnum(uint32_t);
int tsg_iswalpha(uint32_t);
int tsg_iswdigit(uint32_t);
int tsg_iswspace(uint32_t);
uint32_t tsg_towupper(uint32_t);
uint32_t tsg_towlower(uint32_t);
#undef iswalnum
#undef iswalpha
#undef iswdigit
#undef iswspace
#undef towupper
#undef towlower
#define iswalnum tsg_iswalnum
#define iswalpha tsg_iswalpha
#define iswdigit tsg_iswdigit
#define iswspace tsg_iswspace
#define towupper tsg_towupper
#define towlower tsg_towlower
#endif
