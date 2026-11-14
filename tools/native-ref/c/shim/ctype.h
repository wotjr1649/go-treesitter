#ifndef TSG_SHIM_CTYPE_H
#define TSG_SHIM_CTYPE_H
#include_next <ctype.h>
int tsg_isdigit(int);
#undef isdigit
#define isdigit tsg_isdigit
#endif
