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
int iswblank(wint_t) __asm__(TSG_SYMBOL("tsg_iswblank"));
int iswcntrl(wint_t) __asm__(TSG_SYMBOL("tsg_iswcntrl"));
int iswdigit(wint_t) __asm__(TSG_SYMBOL("tsg_iswdigit"));
int iswgraph(wint_t) __asm__(TSG_SYMBOL("tsg_iswgraph"));
int iswlower(wint_t) __asm__(TSG_SYMBOL("tsg_iswlower"));
int iswprint(wint_t) __asm__(TSG_SYMBOL("tsg_iswprint"));
int iswpunct(wint_t) __asm__(TSG_SYMBOL("tsg_iswpunct"));
int iswspace(wint_t) __asm__(TSG_SYMBOL("tsg_iswspace"));
int iswupper(wint_t) __asm__(TSG_SYMBOL("tsg_iswupper"));
int iswxdigit(wint_t) __asm__(TSG_SYMBOL("tsg_iswxdigit"));
wint_t towupper(wint_t) __asm__(TSG_SYMBOL("tsg_towupper"));
wint_t towlower(wint_t) __asm__(TSG_SYMBOL("tsg_towlower"));
#define __iswalnum_l tsg_iswalnum_l
#define __iswalpha_l tsg_iswalpha_l
#define __iswblank_l tsg_iswblank_l
#define __iswcntrl_l tsg_iswcntrl_l
#define __iswdigit_l tsg_iswdigit_l
#define __iswgraph_l tsg_iswgraph_l
#define __iswlower_l tsg_iswlower_l
#define __iswprint_l tsg_iswprint_l
#define __iswpunct_l tsg_iswpunct_l
#define __iswspace_l tsg_iswspace_l
#define __iswupper_l tsg_iswupper_l
#define __iswxdigit_l tsg_iswxdigit_l
#define __towupper_l tsg_towupper_l
#define __towlower_l tsg_towlower_l
#define weak_alias(a, b)
#endif
