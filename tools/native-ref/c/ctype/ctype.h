#include <wctype.h>
int isblank(int) __asm__(TSG_SYMBOL("tsg_isblank"));
int isdigit(int) __asm__(TSG_SYMBOL("tsg_isdigit"));
#define __isblank_l tsg_isblank_l
#define __isdigit_l tsg_isdigit_l
