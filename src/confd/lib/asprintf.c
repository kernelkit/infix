#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>

#ifndef HAVE_VASPRINTF
int vasprintf(char **strp, const char *fmt, va_list ap);
#endif

int asprintf(char **strp, const char *fmt, ...)
{
    va_list ap;
    int len;

    va_start(ap, fmt);
    len = vasprintf(strp, fmt, ap);
    va_end(ap);
    return len;
}
