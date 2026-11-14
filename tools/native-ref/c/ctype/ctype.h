#include <wctype.h>
int isdigit(int) __asm__(TSG_SYMBOL("tsg_isdigit"));
#define __isdigit_l tsg_isdigit_l
