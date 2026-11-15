#ifndef TSG_SHIM_WCTYPE_H
#define TSG_SHIM_WCTYPE_H
#include_next <wctype.h>
#include <stdint.h>
int tsg_iswalnum(uint32_t);
int tsg_iswalpha(uint32_t);
int tsg_iswblank(uint32_t);
int tsg_iswcntrl(uint32_t);
int tsg_iswdigit(uint32_t);
int tsg_iswgraph(uint32_t);
int tsg_iswlower(uint32_t);
int tsg_iswprint(uint32_t);
int tsg_iswpunct(uint32_t);
int tsg_iswspace(uint32_t);
int tsg_iswupper(uint32_t);
int tsg_iswxdigit(uint32_t);
uint32_t tsg_towupper(uint32_t);
uint32_t tsg_towlower(uint32_t);
#undef iswalnum
#undef iswalpha
#undef iswblank
#undef iswcntrl
#undef iswdigit
#undef iswgraph
#undef iswlower
#undef iswprint
#undef iswpunct
#undef iswspace
#undef iswupper
#undef iswxdigit
#undef towupper
#undef towlower
#define iswalnum tsg_iswalnum
#define iswalpha tsg_iswalpha
#define iswblank tsg_iswblank
#define iswcntrl tsg_iswcntrl
#define iswdigit tsg_iswdigit
#define iswgraph tsg_iswgraph
#define iswlower tsg_iswlower
#define iswprint tsg_iswprint
#define iswpunct tsg_iswpunct
#define iswspace tsg_iswspace
#define iswupper tsg_iswupper
#define iswxdigit tsg_iswxdigit
#define towupper tsg_towupper
#define towlower tsg_towlower
#endif
