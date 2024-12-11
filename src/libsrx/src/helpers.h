/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef CONFD_HELPERS_H_
#define CONFD_HELPERS_H_

#include <stdarg.h>

int vasprintf(char **strp, const char *fmt, va_list ap);

char *unquote(char *buf);
char *fgetkey(const char *file, const char *key);

#endif /* CONFD_HELPERS_H_ */
