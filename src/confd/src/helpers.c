/* SPDX-License-Identifier: BSD-3-Clause */

#include <stdarg.h>
#include "core.h"

static FILE *open_file(const char *mode, const char *fmt, va_list ap)
{
	va_list apc;
	char *file;
	int len;

	va_copy(apc, ap);
	len = vsnprintf(NULL, 0, fmt, apc);
	va_end(apc);

	file = alloca(len + 1);
	if (!file) {
		errno = ENOMEM;
		return NULL;
	}

	va_copy(apc, ap);
	vsnprintf(file, len + 1, fmt, apc);
	va_end(apc);

	return fopen(file, mode);
}

/*
 * Write interger value to a file composed from fmt and optional args.
 */
int writedf(int value, const char *fmt, ...)
{
	va_list ap;
	FILE *fp;

	va_start(ap, fmt);
	fp = open_file("r+", fmt, ap);
	va_end(ap);
	if (!fp)
		return -1;

	fprintf(fp, "%d\n", value);
	return fclose(fp);
}

/*
 * Write str to a file composed from fmt and optional args.
 */
int writesf(const char *str, const char *fmt, ...)
{
	va_list ap;
	FILE *fp;

	va_start(ap, fmt);
	fp = open_file("r+", fmt, ap);
	va_end(ap);
	if (!fp)
		return -1;

	fprintf(fp, "%s\n", str);
	return fclose(fp);
}

