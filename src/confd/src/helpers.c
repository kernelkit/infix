/* SPDX-License-Identifier: BSD-3-Clause */

#include <errno.h>
#include <stdarg.h>
#include "core.h"

FILE *popenf(const char *type, const char *cmdf, ...)
{
	va_list ap;
	char *cmd;
	FILE *fp;
	int len;

	va_start(ap, cmdf);
	len = vasprintf(&cmd, cmdf, ap);
	va_end(ap);

	if (len < 0) {
		errno = ENOMEM;
		return NULL;
	}

	fp = popen(cmd, type);
	free(cmd);
	return fp;
}

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

int vreadllf(long long *value, const char *fmt, va_list ap)
{
	char line[0x100];
	FILE *fp;

	fp = open_file("r", fmt, ap);
	if (!fp)
		return -1;

	if (!fgets(line, sizeof(line), fp)) {
		fclose(fp);
		return -1;
	}

	fclose(fp);

	errno = 0;
	*value = strtoll(line, NULL, 0);

	return errno ? -1 : 0;
}

int readllf(long long *value, const char *fmt, ...)
{
	va_list ap;
	int rc;

	va_start(ap, fmt);
	rc = vreadllf(value, fmt, ap);
	va_end(ap);

	return rc;
}

int readdf(int *value, const char *fmt, ...)
{
	long long tmp;
	va_list ap;
	int rc;

	va_start(ap, fmt);
	rc = vreadllf(&tmp, fmt, ap);
	va_end(ap);

	if (rc)
		return rc;

	if (tmp < INT_MIN || tmp > INT_MAX)
		return -1;

	*value = tmp;
	return 0;
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

