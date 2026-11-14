#ifndef TSG_PRIVATE_WCTYPE_H
#define TSG_PRIVATE_WCTYPE_H
#include <stdint.h>

#define wint_t tsg_wint_t
typedef uint32_t wint_t;
typedef void *locale_t;
#define TSG_STRING_(s) #s
#define TSG_STRING(s) TSG_STRING_(s)
#define TSG_SYMBOL(s) TSG_STRING(__USER_LABEL_PREFIX__) s
/* Preserve musl's definitions (including #undef iswdigit), but never export host libc symbols. */
int iswalnum(wint_t) __asm__(TSG_SYMBOL("tsg_iswalnum"));
int iswalpha(wint_t) __asm__(TSG_SYMBOL("tsg_iswalpha"));
int iswdigit(wint_t) __asm__(TSG_SYMBOL("tsg_iswdigit"));
int iswspace(wint_t) __asm__(TSG_SYMBOL("tsg_iswspace"));
wint_t towupper(wint_t) __asm__(TSG_SYMBOL("tsg_towupper"));
wint_t towlower(wint_t) __asm__(TSG_SYMBOL("tsg_towlower"));
#define __iswalnum_l tsg_iswalnum_l
#define __iswalpha_l tsg_iswalpha_l
#define __iswdigit_l tsg_iswdigit_l
#define __iswspace_l tsg_iswspace_l
#define __towupper_l tsg_towupper_l
#define __towlower_l tsg_towlower_l
#define weak_alias(a, b)
#endif
