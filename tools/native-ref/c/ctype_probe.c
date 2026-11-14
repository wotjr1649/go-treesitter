#include <assert.h>
#include <ctype.h>
#include <wctype.h>

int main(void) {
    assert(iswalnum(0x00e9) == 1);
    assert(iswalpha(0x00c9) == 1);
    assert(towupper(0x00e9) == 0x00c9);
    assert(towlower(0x00c9) == 0x00e9);
    assert(iswspace(0x00a0) == 0);
    assert(iswdigit(0x0660) == 0);
    assert(isdigit(0x0660) == 0);
    assert(towupper(0x0131) == 'I');
    assert(iswalnum(0x1f600) == 0);
    assert(iswspace(0x1f600) == 0);
    assert(towupper(0x1f600) == 0x1f600);
    assert(towlower(0x1f600) == 0x1f600);
    assert(iswalpha(0x10400) == 1);
    assert(iswspace(0x12000) == 0);
    assert(iswspace(0x2000) != 0);
    assert(iswspace(0) == 0);
    assert(iswspace('\n') != 0);
    assert(iswdigit('9') == 1);
    assert(isdigit('9') == 1);
    assert(isdigit(-1) == 0);
    assert(towupper('z') == 'Z');
    return 0;
}
