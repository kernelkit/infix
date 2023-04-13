#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>

int vasprintf(char **strp, const char *fmt, va_list ap)
{
    va_list apdup;
    char *str;
    int len;

    va_copy(apdup, ap);
    len = vsnprintf(0, 0, fmt, apdup);
    va_end(apdup);

    if (len < 0)
	    return -1;

    len++;
    str = malloc(len);
    if (!str)
        return -1;

    *strp = str;
    return vsnprintf(str, len, fmt, ap);
}
